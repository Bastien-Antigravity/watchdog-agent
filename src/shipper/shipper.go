package shipper

// =============================================================================
// ESSENTIAL PROCESS:
// Autonomous fleet log tailer and shipper. Continuously monitors local log files
// across native host paths or container volume mounts, parses operational failures,
// guarantees idempotent at-least-once insertion into obsidiandb, and dispatches
// throttled incident events to Squad Agent over NATS.
//
// DATA FLOW:
// 1. Discovers active microservice log files (*.log) on native host or Docker volumes.
// 2. Compares file sizes and OS inodes against checkpointed state (.shipper_offsets.json).
// 3. Tails complete newline-delimited lines up to MaxBatchSize per pass.
// 4. Persists parsed errors atomically into watchdog_agent.fleet_error_logs.
// 5. Commits byte offsets + prunes stale files strictly after transaction commit.
// 6. Emits deduplicated IncidentEvent alerts to Squad Agent via Notifier.
//
// KEY PARAMETERS:
// - MaxBatchSize: Maximum error entries per transaction pass (default: 500).
// - BrainID: Multi-tenant brain identifier stamped onto every persisted log row.
// =============================================================================

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	toolbox_conn "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/conn_manager"
	toolbox_connectivity "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/connectivity"
	toolbox_logger "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/logger"
)

// -----------------------------------------------------------------------------

const defaultMaxBatchSize = 500

// Config configures the LogShipper engine.
type Config struct {
	RootDir        string
	ScanDir        string
	CheckpointPath string
	PollInterval   time.Duration
	MaxBatchSize   int
	BrainID        string
	DBConfig       DBConfig
	Notifier       IncidentPublisher
	Logger         toolbox_logger.Logger
}

// LogShipper coordinates multi-file tailing, checkpointing, and database syncing.
type LogShipper struct {
	cfg            Config
	cm             *CheckpointManager
	db             *DBClient
	throttler      *IncidentThrottler
	reconnStrategy *toolbox_conn.NetworkManager
	logger         toolbox_logger.Logger
	running        atomic.Bool
	mu             sync.Mutex
	reconnNextTry  time.Time
	reconnAttempt  int
}

// -----------------------------------------------------------------------------

// NewLogShipper initializes the shipper with persistent cursor state.
func NewLogShipper(cfg Config) (*LogShipper, error) {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.MaxBatchSize <= 0 {
		cfg.MaxBatchSize = defaultMaxBatchSize
	}
	if cfg.BrainID == "" {
		cfg.BrainID = "obsidian-brain"
	}
	if cfg.ScanDir == "" && (toolbox_connectivity.NewResolver().IsDockerEnv() || DirExists("/var/log/fleet")) {
		cfg.ScanDir = "/var/log/fleet"
	}
	if cfg.CheckpointPath == "" {
		cfg.CheckpointPath = filepath.Join(cfg.RootDir, ".shipper_offsets.json")
	}

	cm, err := NewCheckpointManager(cfg.CheckpointPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init checkpoint manager: %w", err)
	}

	shipper := &LogShipper{
		cfg:            cfg,
		cm:             cm,
		throttler:      NewIncidentThrottler(60 * time.Second),
		reconnStrategy: toolbox_conn.NewCriticalStrategy(cfg.Logger),
		logger:         cfg.Logger,
	}

	// Non-fatal initial DB/schema check: offline DB or pending schema freezes cursors until recovery.
	if dbClient, err := NewDBClient(cfg.DBConfig); err == nil {
		shipper.db = dbClient
		shipper.logger.Info("Database schema verified: watchdog_agent.fleet_error_logs ready")
	} else {
		shipper.logger.Warning("Initial shipper DB/schema check deferred (%v); file offsets remain frozen until recovery", err)
	}

	return shipper, nil
}

func getFileInode(fi os.FileInfo) uint64 {
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return 0
}

// DirExists reports whether path exists and is a directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ScanDir returns the resolved external log scan directory (empty if scanning workspace RootDir).
func (s *LogShipper) ScanDir() string {
	return s.cfg.ScanDir
}

// DiscoverFiles finds all *.log files in Docker volumes or native workspace trees.
func (s *LogShipper) DiscoverFiles() ([]string, error) {
	var candidates []string

	if s.cfg.ScanDir != "" && DirExists(s.cfg.ScanDir) {
		err := filepath.Walk(s.cfg.ScanDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && strings.HasSuffix(info.Name(), ".log") {
				candidates = append(candidates, path)
			}
			return nil
		})
		return candidates, err
	}

	if s.cfg.RootDir != "" {
		err := filepath.Walk(s.cfg.RootDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				name := info.Name()
				if name == ".git" || name == "target" || name == "node_modules" || name == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(info.Name(), ".log") {
				normalized := filepath.ToSlash(path)
				if strings.Contains(normalized, "/logs/") || strings.Contains(normalized, "/bin/logs/") {
					candidates = append(candidates, path)
				}
			}
			return nil
		})
		return candidates, err
	}

	return candidates, nil
}

// -----------------------------------------------------------------------------

type pendingOffset struct {
	offset int64
	inode  uint64
}

func (s *LogShipper) ensureConnected() error {
	if s.db != nil {
		return nil
	}
	if !s.reconnNextTry.IsZero() && time.Now().Before(s.reconnNextTry) {
		return fmt.Errorf("database connection unavailable (retrying after %s)", time.Until(s.reconnNextTry).Round(time.Millisecond))
	}
	dbClient, err := NewDBClient(s.cfg.DBConfig)
	if err != nil {
		delay := s.reconnStrategy.GetNextDelay(s.reconnAttempt)
		s.reconnAttempt++
		s.reconnNextTry = time.Now().Add(delay)
		return fmt.Errorf("database connection unavailable (retrying in %s): %w", delay, err)
	}
	s.db = dbClient
	s.reconnAttempt = 0
	s.reconnNextTry = time.Time{}
	s.logger.Info("Database schema verified: watchdog_agent.fleet_error_logs ready")
	return nil
}

func isAggregatedMainLog(filePath string) bool {
	base := filepath.Base(filePath)
	return base == "_main.log" || strings.HasPrefix(base, "_main.")
}

func isOwnLogServerEntry(entry *ParsedErrorLog) bool {
	return entry.ServiceName == "log_server" || entry.ServiceName == "log-server" ||
		entry.LoggerName == "log_server" || entry.LoggerName == "log-server"
}

// tailSingleFile reads complete newline-terminated lines from lastOffset up to maxRemaining errors.
func (s *LogShipper) tailSingleFile(filePath string, maxRemaining int) ([]*ParsedErrorLog, *pendingOffset, bool) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, nil, false
	}

	currentSize := fi.Size()
	currentInode := getFileInode(fi)
	lastOffset, lastInode := s.cm.GetOffset(filePath)

	if currentSize < lastOffset || (currentInode != 0 && lastInode != 0 && currentInode != lastInode) {
		lastOffset = 0
	}
	if currentSize <= lastOffset {
		return nil, nil, false
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, false
	}
	defer f.Close()

	if _, err := f.Seek(lastOffset, io.SeekStart); err != nil {
		return nil, nil, false
	}

	reader := bufio.NewReader(f)
	readOffset := lastOffset
	isMainAggLog := isAggregatedMainLog(filePath)
	var batch []*ParsedErrorLog
	hitLimit := false

	for {
		line, readErr := reader.ReadString('\n')
		// Skip incomplete trailing line lacking '\n' so mid-write flushes are not dropped.
		if !strings.HasSuffix(line, "\n") {
			break
		}

		lineOffset := readOffset
		readOffset += int64(len(line))

		if parsed, ok := ParseLogLine(line); ok {
			// Aggregated _main.log duplicates forwarded client streams already tailed per service; retain only log-server's own errors.
			if isMainAggLog && !isOwnLogServerEntry(parsed) {
				if readErr != nil {
					break
				}
				continue
			}
			parsed.BrainID = s.cfg.BrainID
			parsed.EventHash = ComputeEventHash(s.cfg.BrainID, filePath, lineOffset, parsed)
			batch = append(batch, parsed)
			if len(batch) >= maxRemaining {
				hitLimit = true
				break
			}
		}

		if readErr != nil {
			break
		}
	}

	if readOffset == lastOffset {
		return nil, nil, hitLimit
	}
	return batch, &pendingOffset{offset: readOffset, inode: currentInode}, hitLimit
}

// ScanAndShip executes one bounded synchronization pass across monitored log files.
func (s *LogShipper) ScanAndShip(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureConnected(); err != nil {
		return 0, err
	}

	files, err := s.DiscoverFiles()
	if err != nil {
		return 0, err
	}

	existingSet := make(map[string]bool, len(files))
	for _, f := range files {
		existingSet[f] = true
	}

	var totalBatch []*ParsedErrorLog
	fileNewOffsets := make(map[string]*pendingOffset)
	hitLimit := false

	for _, filePath := range files {
		remaining := s.cfg.MaxBatchSize - len(totalBatch)
		if remaining <= 0 {
			hitLimit = true
			break
		}
		entries, nextOff, fileHitLimit := s.tailSingleFile(filePath, remaining)
		if len(entries) > 0 {
			totalBatch = append(totalBatch, entries...)
		}
		if nextOff != nil {
			fileNewOffsets[filePath] = nextOff
		}
		if fileHitLimit {
			hitLimit = true
			break
		}
	}

	if len(totalBatch) > 0 {
		if err := s.db.InsertBatch(ctx, totalBatch); err != nil {
			_ = s.db.Close()
			s.db = nil
			return 0, fmt.Errorf("failed to flush error batch to db: %w", err)
		}
	}

	changed := len(fileNewOffsets) > 0
	for path, state := range fileNewOffsets {
		s.cm.SetOffset(path, state.offset, state.inode)
	}
	if !hitLimit && s.cm.PruneMissingFiles(existingSet) {
		changed = true
	}
	if changed {
		_ = s.cm.Save()
	}

	// Best-effort non-blocking Squad Agent notification after durable DB commit.
	if len(totalBatch) > 0 && s.cfg.Notifier != nil && s.throttler != nil {
		for _, errEntry := range totalBatch {
			if should, count := s.throttler.ShouldDispatch(errEntry); should {
				inc := BuildIncidentEvent(errEntry, count)
				_ = s.cfg.Notifier.PublishIncident(ctx, inc)
			}
		}
	}

	return len(totalBatch), nil
}

// Start launches the continuous log shipper background loop.
func (s *LogShipper) Start(ctx context.Context) {
	if !s.running.CompareAndSwap(false, true) {
		return
	}

	go func() {
		ticker := time.NewTicker(s.cfg.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.running.Store(false)
				if s.db != nil {
					_ = s.db.Close()
				}
				return
			case <-ticker.C:
				for {
					shipped, err := s.ScanAndShip(ctx)
					if err != nil || shipped < s.cfg.MaxBatchSize {
						break
					}
					select {
					case <-ctx.Done():
						s.running.Store(false)
						if s.db != nil {
							_ = s.db.Close()
						}
						return
					default:
					}
				}
			}
		}
	}()
}
