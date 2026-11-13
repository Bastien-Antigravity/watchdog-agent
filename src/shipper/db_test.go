package shipper

import (
	"os"
	"strings"
	"testing"

	distributed_config "github.com/Bastien-Antigravity/distributed-config"
	dcore "github.com/Bastien-Antigravity/distributed-config/src/core"
	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
)

func makeTestAppConfig(caps map[string]interface{}) *toolbox_config.AppConfig {
	return &toolbox_config.AppConfig{
		Config: &distributed_config.Config{
			Config: &dcore.Config{
				Capabilities: caps,
			},
		},
	}
}

func TestResolveDBConfigWithAppConfig_SuccessAndPriority(t *testing.T) {
	cfg := makeTestAppConfig(map[string]interface{}{
		"timescale_db": map[string]interface{}{
			"ip":       "10.0.0.5",
			"port":     "5433",
			"user":     "custom_user",
			"password": "secret_pass",
			"db_name":  "maindb",
			"ssl_cert": "true",
		},
		"rag_engine": map[string]interface{}{
			"db_name": "obsidiandb",
		},
		"watchdog_agent": map[string]interface{}{
			"db_name": "obsidiandb",
		},
	})

	dbCfg, err := ResolveDBConfigWithAppConfig(cfg)
	if err != nil {
		t.Fatalf("expected valid config to succeed, got error: %v", err)
	}
	if dbCfg.Host != "10.0.0.5" || dbCfg.Port != 5433 || dbCfg.User != "custom_user" || dbCfg.Password != "secret_pass" {
		t.Errorf("unexpected resolved connection params: %+v", dbCfg)
	}
	if dbCfg.DBName != "obsidiandb" {
		t.Errorf("expected base ecosystem db obsidiandb (not maindb), got %s", dbCfg.DBName)
	}
	if dbCfg.SSLMode != "require" {
		t.Errorf("expected sslmode require when ssl_cert=true, got %s", dbCfg.SSLMode)
	}
}

func TestResolveDBConfigWithAppConfig_FallbackToRagEngineDBName(t *testing.T) {
	cfg := makeTestAppConfig(map[string]interface{}{
		"timescale_db": map[string]interface{}{
			"ip":       "127.0.0.1",
			"port":     "5432",
			"user":     "dbuser",
			"password": "pw",
			"db_name":  "maindb",
			"ssl_cert": "false",
		},
		"rag_engine": map[string]interface{}{
			"db_name": "obsidiandb",
		},
	})

	dbCfg, err := ResolveDBConfigWithAppConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dbCfg.DBName != "obsidiandb" || dbCfg.SSLMode != "disable" {
		t.Errorf("unexpected dbCfg: %+v", dbCfg)
	}
}

func TestResolveDBConfigWithAppConfig_FailFastValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  *toolbox_config.AppConfig
	}{
		{name: "missing timescale_db", cfg: makeTestAppConfig(map[string]interface{}{})},
		{name: "invalid port", cfg: makeTestAppConfig(map[string]interface{}{
			"timescale_db": map[string]interface{}{"ip": "127.0.0.1", "port": "not-a-port", "user": "u", "password": "p"},
			"rag_engine":   map[string]interface{}{"db_name": "obsidiandb"},
		})},
		{name: "missing base db_name", cfg: makeTestAppConfig(map[string]interface{}{
			"timescale_db": map[string]interface{}{"ip": "127.0.0.1", "port": "5432", "user": "u", "password": "p", "db_name": "maindb"},
		})},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ResolveDBConfigWithAppConfig(tc.cfg); err == nil {
				t.Fatalf("expected fail-fast error for %s, got nil", tc.name)
			}
		})
	}
}

func TestResolveBrainID_Precedence(t *testing.T) {
	_ = os.Setenv("TARGET_BRAIN_ID", "env-brain")
	defer os.Unsetenv("TARGET_BRAIN_ID")

	// 1. watchdog_agent.brain_id wins over squad_agent & env
	cfg1 := makeTestAppConfig(map[string]interface{}{
		"watchdog_agent": map[string]interface{}{"brain_id": "wd-brain"},
		"squad_agent":    map[string]interface{}{"target_brain_id": "sq-brain"},
	})
	if got := ResolveBrainID(cfg1); got != "wd-brain" {
		t.Errorf("expected wd-brain, got %s", got)
	}

	// 2. squad_agent.target_brain_id wins over env
	cfg2 := makeTestAppConfig(map[string]interface{}{
		"squad_agent": map[string]interface{}{"target_brain_id": "sq-brain"},
	})
	if got := ResolveBrainID(cfg2); got != "sq-brain" {
		t.Errorf("expected sq-brain, got %s", got)
	}

	// 3. Env wins if AppConfig has no brain setting
	cfg3 := makeTestAppConfig(map[string]interface{}{})
	if got := ResolveBrainID(cfg3); got != "env-brain" {
		t.Errorf("expected env-brain, got %s", got)
	}

	// 4. Canonical root brain fallback
	_ = os.Unsetenv("TARGET_BRAIN_ID")
	_ = os.Unsetenv("BRAIN_ID")
	if got := ResolveBrainID(cfg3); got != "obsidian-brain" {
		t.Errorf("expected obsidian-brain, got %s", got)
	}
}

func TestDBConfig_DSN(t *testing.T) {
	cfg := DBConfig{
		Host:     "127.0.0.1",
		Port:     5432,
		User:     "test_user",
		Password: "pass@word!#",
		DBName:   "obsidiandb",
		SSLMode:  "disable",
	}

	dsn := cfg.DSN()
	if !strings.HasPrefix(dsn, "postgres://test_user:") {
		t.Errorf("expected DSN to start with postgres://test_user:, got %s", dsn)
	}
	if !strings.Contains(dsn, "@127.0.0.1:5432/obsidiandb") {
		t.Errorf("expected DSN to contain host/db, got %s", dsn)
	}
	if !strings.Contains(dsn, "sslmode=disable") {
		t.Errorf("expected DSN to contain sslmode=disable, got %s", dsn)
	}
	if !strings.Contains(dsn, "connect_timeout=5") {
		t.Errorf("expected DSN to contain connect_timeout=5, got %s", dsn)
	}
}
