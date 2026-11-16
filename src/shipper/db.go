package shipper

// =============================================================================
// ESSENTIAL PROCESS:
// TimescaleDB / PostgreSQL error sink management for watchdog-agent. Resolves
// validated connection credentials from AppConfig capabilities, verifies schema
// "watchdog_agent".fleet_error_logs, and commits idempotent batch inserts.
//
// DATA FLOW:
// 1. Validates AppConfig capabilities (timescale_db + watchdog_agent/rag_engine db_name).
// 2. Decrypts ENC(...) credentials at cold startup without silent fallback values.
// 3. Opens pooled sql.DB connection and ensures schema/hypertable DDL availability.
// 4. Executes transactional ON CONFLICT DO NOTHING batch inserts keyed by event_hash.
//
// KEY PARAMETERS:
// - DBConfig: Validated PostgreSQL connection parameters resolved from AppConfig.
// - FleetErrorLogsSchemaSQL: Authoritative DDL for watchdog_agent.fleet_error_logs.
// =============================================================================

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	_ "github.com/lib/pq"
)

// -----------------------------------------------------------------------------

// DBClient manages the TimescaleDB connection pool and idempotent batch writes.
type DBClient struct {
	db *sql.DB
}

// DBConfig encapsulates validated PostgreSQL connection parameters.
type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// TimescaleDBCap mirrors the validated capabilities.timescale_db YAML structure.
type TimescaleDBCap struct {
	IP       string      `json:"ip"`
	Port     string      `json:"port"`
	User     string      `json:"user"`
	Password string      `json:"password"`
	DBName   string      `json:"db_name"`
	SSLCert  interface{} `json:"ssl_cert"`
}

// WatchdogErrorDBCap captures optional watchdog_agent error log settings in AppConfig.
type WatchdogErrorDBCap struct {
	DBName  string `json:"db_name"`
	BrainID string `json:"brain_id"`
}

// -----------------------------------------------------------------------------

// ResolveBrainID determines the active brain namespace using the canonical precedence
// documented in Multi-Brain Partitioning Standard: AppConfig -> env -> obsidian-brain.
func ResolveBrainID(appConfig *toolbox_config.AppConfig) string {
	var wdCap WatchdogErrorDBCap
	if err := appConfig.GetCapability("watchdog_agent", &wdCap); err == nil && wdCap.BrainID != "" {
		return wdCap.BrainID
	}
	var saCap struct {
		TargetBrainID string `json:"target_brain_id"`
	}
	if err := appConfig.GetCapability("squad_agent", &saCap); err == nil && saCap.TargetBrainID != "" {
		return saCap.TargetBrainID
	}
	if envBrain := os.Getenv("TARGET_BRAIN_ID"); envBrain != "" {
		return envBrain
	}
	if envBrain := os.Getenv("BRAIN_ID"); envBrain != "" {
		return envBrain
	}
	return "obsidian-brain"
}

// ResolveDBConfigWithAppConfig resolves DB parameters from validated AppConfig
// targeting Option B (base ecosystem 'obsidiandb') and fails fast on missing values.
func ResolveDBConfigWithAppConfig(appConfig *toolbox_config.AppConfig) (DBConfig, error) {
	var tsCap TimescaleDBCap
	if err := appConfig.GetCapability("timescale_db", &tsCap); err != nil {
		return DBConfig{}, fmt.Errorf("failed to resolve timescale_db capability: %w", err)
	}
	port, err := strconv.Atoi(tsCap.Port)
	if err != nil {
		return DBConfig{}, fmt.Errorf("invalid timescale_db port %q: %w", tsCap.Port, err)
	}
	password, err := appConfig.DecryptSecret(tsCap.Password)
	if err != nil {
		return DBConfig{}, fmt.Errorf("failed to decrypt timescale_db password: %w", err)
	}

	sslMode := "disable"
	if tsCap.SSLCert == true || tsCap.SSLCert == "true" {
		sslMode = "require"
	}

	dbName, err := resolveErrorDBName(appConfig)
	if err != nil {
		return DBConfig{}, err
	}

	return DBConfig{
		Host:     tsCap.IP,
		Port:     port,
		User:     tsCap.User,
		Password: password,
		DBName:   dbName,
		SSLMode:  sslMode,
	}, nil
}

func resolveErrorDBName(appConfig *toolbox_config.AppConfig) (string, error) {
	var wdCap WatchdogErrorDBCap
	if err := appConfig.GetCapability("watchdog_agent", &wdCap); err == nil && wdCap.DBName != "" {
		return wdCap.DBName, nil
	}
	var ragCap struct {
		DBName string `json:"db_name"`
	}
	if err := appConfig.GetCapability("rag_engine", &ragCap); err == nil && ragCap.DBName != "" {
		return ragCap.DBName, nil
	}
	return "", fmt.Errorf("missing base database name in capability 'watchdog_agent.db_name' or 'rag_engine.db_name'")
}

// -----------------------------------------------------------------------------

// DSN returns the formatted PostgreSQL connection URI.
func (c DBConfig) DSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:   "/" + c.DBName,
	}
	q := u.Query()
	q.Set("sslmode", c.SSLMode)
	q.Set("connect_timeout", "5")
	u.RawQuery = q.Encode()
	return u.String()
}

// NewDBClient establishes a connection pool to the configured base database.
func NewDBClient(cfg DBConfig) (*DBClient, error) {
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(15 * time.Minute)

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer pingCancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping postgresql (%s:%d/%s): %w", cfg.Host, cfg.Port, cfg.DBName, err)
	}

	client := &DBClient{db: db}
	schemaCtx, schemaCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer schemaCancel()
	if err := client.EnsureSchema(schemaCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to verify/create watchdog_agent schema: %w", err)
	}

	return client, nil
}

// -----------------------------------------------------------------------------

// FleetErrorLogsSchemaSQL defines the canonical Option B DDL inside obsidiandb schema 'watchdog_agent'.
const FleetErrorLogsSchemaSQL = `CREATE SCHEMA IF NOT EXISTS watchdog_agent;

CREATE TABLE IF NOT EXISTS watchdog_agent.fleet_error_logs (
	timestamp     TIMESTAMPTZ NOT NULL,
	brain_id      VARCHAR(64) NOT NULL,
	hostname      VARCHAR(64) NOT NULL,
	service_name  VARCHAR(64) NOT NULL,
	logger_name   VARCHAR(64) NOT NULL,
	level         SMALLINT NOT NULL,
	level_str     VARCHAR(16) NOT NULL,
	filename      VARCHAR(256),
	function_name VARCHAR(128),
	line_number   VARCHAR(16),
	message       TEXT NOT NULL,
	stack_trace   TEXT,
	event_hash    CHAR(64) NOT NULL,
	created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE (timestamp, event_hash)
);

DO $$ BEGIN
	IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
		PERFORM create_hypertable('watchdog_agent.fleet_error_logs', 'timestamp', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
		PERFORM add_retention_policy('watchdog_agent.fleet_error_logs', INTERVAL '30 days', if_not_exists => TRUE);
	END IF;
	IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'db_browser_reader') THEN
		GRANT USAGE ON SCHEMA watchdog_agent TO db_browser_reader;
		GRANT SELECT ON watchdog_agent.fleet_error_logs TO db_browser_reader;
	END IF;
EXCEPTION WHEN OTHERS THEN
	NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_fleet_error_brain_svc_time ON watchdog_agent.fleet_error_logs (brain_id, service_name, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_error_level_time ON watchdog_agent.fleet_error_logs (level, timestamp DESC);`

// EnsureSchema creates schema 'watchdog_agent', table 'fleet_error_logs', hypertable, and retention if missing.
func (c *DBClient) EnsureSchema(ctx context.Context) error {
	_, err := c.db.ExecContext(ctx, FleetErrorLogsSchemaSQL)
	return err
}

// InsertBatch idempotently inserts parsed error logs inside an atomic bounded transaction.
func (c *DBClient) InsertBatch(ctx context.Context, entries []*ParsedErrorLog) error {
	if len(entries) == 0 {
		return nil
	}

	txCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	tx, err := c.db.BeginTx(txCtx, nil)
	if err != nil {
		return fmt.Errorf("begin tx failed: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(txCtx, `
		INSERT INTO watchdog_agent.fleet_error_logs (
			timestamp, brain_id, hostname, service_name, logger_name, level,
			level_str, filename, function_name, line_number, message, stack_trace, event_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (timestamp, event_hash) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("prepare stmt failed: %w", err)
	}
	defer stmt.Close()

	for _, e := range entries {
		_, err := stmt.ExecContext(
			txCtx,
			e.Timestamp,
			e.BrainID,
			e.Hostname,
			e.ServiceName,
			e.LoggerName,
			e.Level,
			e.LevelStr,
			e.Filename,
			e.FunctionName,
			e.LineNumber,
			e.Message,
			e.StackTrace,
			e.EventHash,
		)
		if err != nil {
			return fmt.Errorf("exec stmt failed for %s:%s: %w", e.ServiceName, e.Message, err)
		}
	}

	return tx.Commit()
}

// Close closes the underlying database pool.
func (c *DBClient) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}
