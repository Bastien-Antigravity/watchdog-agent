package shipper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointManager_SaveAndReload(t *testing.T) {
	tempDir := t.TempDir()
	checkpointPath := filepath.Join(tempDir, "subfolder", ".shipper_offsets.json")

	cm, err := NewCheckpointManager(checkpointPath)
	if err != nil {
		t.Fatalf("failed to initialize CheckpointManager: %v", err)
	}

	file1 := "/var/log/fleet/svc1/svc1.log"
	file2 := "/var/log/fleet/svc2/svc2.log"

	cm.SetOffset(file1, 1024, 11111)
	cm.SetOffset(file2, 2048, 22222)

	if err := cm.Save(); err != nil {
		t.Fatalf("failed to save checkpoint: %v", err)
	}

	// Reload from disk
	reloaded, err := NewCheckpointManager(checkpointPath)
	if err != nil {
		t.Fatalf("failed to reload checkpoint: %v", err)
	}

	off1, ino1 := reloaded.GetOffset(file1)
	if off1 != 1024 || ino1 != 11111 {
		t.Errorf("file1: expected (1024, 11111), got (%d, %d)", off1, ino1)
	}

	off2, ino2 := reloaded.GetOffset(file2)
	if off2 != 2048 || ino2 != 22222 {
		t.Errorf("file2: expected (2048, 22222), got (%d, %d)", off2, ino2)
	}

	// Check non-existent file returns 0, 0
	off3, ino3 := reloaded.GetOffset("/nonexistent.log")
	if off3 != 0 || ino3 != 0 {
		t.Errorf("expected (0, 0) for non-existent file, got (%d, %d)", off3, ino3)
	}
}

func TestCheckpointManager_NonExistentFile(t *testing.T) {
	tempDir := t.TempDir()
	checkpointPath := filepath.Join(tempDir, "fresh_offsets.json")

	cm, err := NewCheckpointManager(checkpointPath)
	if err != nil {
		t.Fatalf("expected NewCheckpointManager to succeed on non-existent file, got %v", err)
	}

	offset, inode := cm.GetOffset("any.log")
	if offset != 0 || inode != 0 {
		t.Errorf("expected 0, 0; got %d, %d", offset, inode)
	}
}

func TestCheckpointManager_CorruptedFileRecovery(t *testing.T) {
	tempDir := t.TempDir()
	checkpointPath := filepath.Join(tempDir, "corrupted_offsets.json")

	// Write invalid JSON bytes
	if err := os.WriteFile(checkpointPath, []byte("NOT_VALID_JSON{{{"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := NewCheckpointManager(checkpointPath)
	if err == nil {
		t.Error("expected NewCheckpointManager to return error on corrupted JSON")
	}
}

func TestCheckpointManager_PruneMissingFiles(t *testing.T) {
	tempDir := t.TempDir()
	checkpointPath := filepath.Join(tempDir, ".shipper_offsets.json")

	cm, err := NewCheckpointManager(checkpointPath)
	if err != nil {
		t.Fatalf("failed to init cm: %v", err)
	}

	cm.SetOffset("/var/log/fleet/active.log", 128, 10)
	cm.SetOffset("/var/log/fleet/deleted.log", 256, 20)

	pruned := cm.PruneMissingFiles(map[string]bool{
		"/var/log/fleet/active.log": true,
	})
	if !pruned {
		t.Fatal("expected PruneMissingFiles to report pruning happened")
	}
	if off, _ := cm.GetOffset("/var/log/fleet/deleted.log"); off != 0 {
		t.Errorf("expected deleted.log offset to be pruned, got %d", off)
	}
	if off, _ := cm.GetOffset("/var/log/fleet/active.log"); off != 128 {
		t.Errorf("expected active.log offset to remain 128, got %d", off)
	}
}

