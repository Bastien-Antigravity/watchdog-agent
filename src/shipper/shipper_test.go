package shipper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testDummyDBConfig() DBConfig {
	return DBConfig{
		Host:     "127.0.0.1",
		Port:     59999,
		User:     "dbuser",
		Password: "pw",
		DBName:   "obsidiandb",
		SSLMode:  "disable",
	}
}

func TestShipper_DiscoverFilesAndCheckpoint(t *testing.T) {
	tempDir := t.TempDir()

	serviceLogDir := filepath.Join(tempDir, "test-service", "logs")
	if err := os.MkdirAll(serviceLogDir, 0755); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}

	logFile := filepath.Join(serviceLogDir, "test-service.log")
	initialContent := "2026-11-08T15:20:00.000000000Z    node-01      test-service           INFO       main.go              main                      20     Service starting\n" +
		"2026-11-08T15:20:01.000000000Z    node-01      test-service           ERROR      router.go            HandleReq                 45     Failed to connect to gateway\n"

	if err := os.WriteFile(logFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write initial log: %v", err)
	}

	checkpointPath := filepath.Join(tempDir, ".shipper_offsets.json")
	cfg := Config{
		RootDir:        tempDir,
		CheckpointPath: checkpointPath,
		PollInterval:   100 * time.Millisecond,
		BrainID:        "obsidian-brain",
		DBConfig:       testDummyDBConfig(),
	}

	shipper, err := NewLogShipper(cfg)
	if err != nil {
		t.Fatalf("failed to create shipper: %v", err)
	}

	files, err := shipper.DiscoverFiles()
	if err != nil {
		t.Fatalf("DiscoverFiles failed: %v", err)
	}
	if len(files) != 1 || files[0] != logFile {
		t.Fatalf("expected [%s], got %v", logFile, files)
	}

	cm := shipper.cm
	if offset, _ := cm.GetOffset(logFile); offset != 0 {
		t.Errorf("initial offset should be 0, got %d", offset)
	}

	cm.SetOffset(logFile, int64(len(initialContent)), 12345)
	if err := cm.Save(); err != nil {
		t.Fatalf("cm.Save failed: %v", err)
	}

	cmReloaded, err := NewCheckpointManager(checkpointPath)
	if err != nil {
		t.Fatalf("reload checkpoint failed: %v", err)
	}
	if reloadedOffset, _ := cmReloaded.GetOffset(logFile); reloadedOffset != int64(len(initialContent)) {
		t.Errorf("expected reloaded offset %d, got %d", len(initialContent), reloadedOffset)
	}
}

func TestShipper_TailSingleFile_SkipsIncompleteEOFLine(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "partial.log")

	completeLine := "2026-11-08T15:20:01.000000000Z    node-01      svc-a                  ERROR      main.go              Run                       10     First complete error\n"
	partialLine := "2026-11-08T15:20:02.000000000Z    node-01      svc-a                  ERROR      main.go              Run                       11     Half flushed"

	if err := os.WriteFile(logFile, []byte(completeLine+partialLine), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	shipper, err := NewLogShipper(Config{
		RootDir:        tempDir,
		CheckpointPath: filepath.Join(tempDir, ".shipper_offsets.json"),
		BrainID:        "trading-brain",
		DBConfig:       testDummyDBConfig(),
	})
	if err != nil {
		t.Fatalf("NewLogShipper failed: %v", err)
	}

	batch, nextOff, hitLimit := shipper.tailSingleFile(logFile, 100)
	if hitLimit || nextOff == nil {
		t.Fatalf("unexpected tail result: hitLimit=%v nextOff=%v", hitLimit, nextOff)
	}
	if len(batch) != 1 || batch[0].BrainID != "trading-brain" || batch[0].EventHash == "" {
		t.Fatalf("expected 1 parsed entry with BrainID and EventHash, got %+v", batch)
	}
	if nextOff.offset != int64(len(completeLine)) {
		t.Fatalf("cursor advanced past partial line: expected %d, got %d", len(completeLine), nextOff.offset)
	}

	// Commit cursor then finish writing newline
	shipper.cm.SetOffset(logFile, nextOff.offset, nextOff.inode)
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open append failed: %v", err)
	}
	_, _ = f.WriteString(" error line\n")
	_ = f.Close()

	batch2, nextOff2, _ := shipper.tailSingleFile(logFile, 100)
	if len(batch2) != 1 || batch2[0].Message != "Half flushed error line" {
		t.Fatalf("expected second pass to read completed line intact, got %+v", batch2)
	}
	if nextOff2.offset != int64(len(completeLine)+len(partialLine)+len(" error line\n")) {
		t.Fatalf("unexpected final offset: %d", nextOff2.offset)
	}
	if batch[0].EventHash == batch2[0].EventHash {
		t.Fatal("expected distinct EventHash across distinct lines")
	}
}

func TestShipper_TailSingleFile_BoundedBatchChunking(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "burst.log")

	line1 := "2026-11-08T15:20:01Z node-1 svc ERROR a.go F 1 Error one\n"
	line2 := "2026-11-08T15:20:02Z node-1 svc ERROR a.go F 2 Error two\n"
	line3 := "2026-11-08T15:20:03Z node-1 svc ERROR a.go F 3 Error three\n"

	if err := os.WriteFile(logFile, []byte(line1+line2+line3), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	shipper, err := NewLogShipper(Config{
		RootDir:        tempDir,
		CheckpointPath: filepath.Join(tempDir, ".shipper_offsets.json"),
		MaxBatchSize:   2,
		DBConfig:       testDummyDBConfig(),
	})
	if err != nil {
		t.Fatalf("NewLogShipper failed: %v", err)
	}

	batch1, off1, hit1 := shipper.tailSingleFile(logFile, 2)
	if !hit1 || len(batch1) != 2 {
		t.Fatalf("expected hitLimit=true with 2 errors, got hit=%v len=%d", hit1, len(batch1))
	}
	if off1.offset != int64(len(line1)+len(line2)) {
		t.Fatalf("expected cursor paused cleanly after line 2 (%d), got %d", len(line1)+len(line2), off1.offset)
	}

	shipper.cm.SetOffset(logFile, off1.offset, off1.inode)
	batch2, off2, hit2 := shipper.tailSingleFile(logFile, 2)
	if hit2 || len(batch2) != 1 || batch2[0].Message != "Error three" {
		t.Fatalf("expected final line without hitLimit, got hit=%v batch=%+v", hit2, batch2)
	}
	if off2.offset != int64(len(line1)+len(line2)+len(line3)) {
		t.Fatalf("unexpected final offset %d", off2.offset)
	}
}

func TestShipper_RotationDetection(t *testing.T) {
	tempDir := t.TempDir()
	checkpointPath := filepath.Join(tempDir, ".shipper_offsets.json")
	cm, err := NewCheckpointManager(checkpointPath)
	if err != nil {
		t.Fatalf("failed to create cm: %v", err)
	}

	testFile := filepath.Join(tempDir, "service.log")
	newErr := "2026-11-08T15:20:01Z node-1 svc ERROR a.go F 1 Post rotation error\n"
	if err := os.WriteFile(testFile, []byte(newErr), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	fi, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}

	initialInode := getFileInode(fi)
	cm.SetOffset(testFile, 5000, initialInode) // Simulate offset 5000 > currentSize (truncation)

	shipper, err := NewLogShipper(Config{
		RootDir:        tempDir,
		CheckpointPath: checkpointPath,
		DBConfig:       testDummyDBConfig(),
	})
	if err != nil {
		t.Fatalf("NewLogShipper failed: %v", err)
	}
	shipper.cm = cm

	batch, nextOff, _ := shipper.tailSingleFile(testFile, 10)
	if len(batch) != 1 || nextOff.offset != int64(len(newErr)) {
		t.Fatalf("expected truncated file to reset cursor to 0 and read 1 error, got batch=%+v nextOff=%+v", batch, nextOff)
	}
}

func TestShipper_OfflineDB_OffsetsFreeze(t *testing.T) {
	tempDir := t.TempDir()

	serviceLogDir := filepath.Join(tempDir, "service-b", "logs")
	if err := os.MkdirAll(serviceLogDir, 0755); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}

	logFile := filepath.Join(serviceLogDir, "service-b.log")
	errorContent := "2026-11-08T15:20:01.000000000Z    node-01      service-b              ERROR      main.go              Run                       10     Uncaught panic\n"
	if err := os.WriteFile(logFile, []byte(errorContent), 0644); err != nil {
		t.Fatalf("failed to write log: %v", err)
	}

	checkpointPath := filepath.Join(tempDir, ".shipper_offsets.json")
	cfg := Config{
		RootDir:        tempDir,
		CheckpointPath: checkpointPath,
		PollInterval:   100 * time.Millisecond,
		DBConfig:       testDummyDBConfig(),
	}

	shipper, err := NewLogShipper(cfg)
	if err != nil {
		t.Fatalf("failed to create shipper: %v", err)
	}
	shipper.db = nil

	if _, err = shipper.ScanAndShip(t.Context()); err == nil {
		t.Fatal("expected ScanAndShip to fail when DB is unreachable, but got nil error")
	}

	if offset, _ := shipper.cm.GetOffset(logFile); offset != 0 {
		t.Fatalf("expected offset to remain frozen at 0 on DB error, got %d", offset)
	}
}

func TestShipper_SkipsForwardedEntriesInMainLog(t *testing.T) {
	tempDir := t.TempDir()
	mainLogDir := filepath.Join(tempDir, "log-server", "logs")
	if err := os.MkdirAll(mainLogDir, 0o755); err != nil {
		t.Fatalf("failed to mkdir main log dir: %v", err)
	}

	mainLog := filepath.Join(mainLogDir, "_main.log")
	content := strings.Join([]string{
		"2026-11-11T10:00:00.000000000Z    host-01      config_server          ERROR      loader.go            Load                      42     forwarded config failure",
		"2026-11-11T10:00:01.000000000Z    host-01      log_server             ERROR      writer.rs            flush                     88     own disk write failure",
	}, "\n") + "\n"
	if err := os.WriteFile(mainLog, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write _main.log: %v", err)
	}

	shipper, err := NewLogShipper(Config{
		RootDir:        tempDir,
		CheckpointPath: filepath.Join(tempDir, ".shipper_offsets.json"),
		DBConfig:       testDummyDBConfig(),
	})
	if err != nil {
		t.Fatalf("failed to create shipper: %v", err)
	}

	batch, nextOff, hitLimit := shipper.tailSingleFile(mainLog, 50)
	if hitLimit || nextOff == nil {
		t.Fatalf("expected non-empty offset update without limit hit")
	}
	if len(batch) != 1 || batch[0].ServiceName != "log_server" {
		t.Fatalf("expected only log_server entry from _main.log, got %+v", batch)
	}
}
