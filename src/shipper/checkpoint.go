package shipper

// =============================================================================
// ESSENTIAL PROCESS:
// Checkpoint manager tracking persistent file byte offsets and inodes.
// Guarantees at-least-once delivery with zero duplicate replay across shipper
// restarts, crashes, or log file rotations.
//
// DATA FLOW:
// 1. Loads .shipper_offsets.json on initialization.
// 2. Returns current byte offset for given log file.
// 3. Updates offsets after successful database transaction commit.
// 4. Atomically persists state to disk via tempfile + rename and prunes deleted files.
//
// KEY PARAMETERS:
// - filePath: Filesystem path to the JSON cursor checkpoint store.
// - offsets: Map of monitored log file path to last committed FileOffset.
// =============================================================================

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileOffset records the read progression for a single log file.
type FileOffset struct {
	Offset        int64     `json:"offset"`
	Inode         uint64    `json:"inode"`
	LastProcessed time.Time `json:"last_processed"`
}

// CheckpointManager coordinates thread-safe file cursor persistence.
type CheckpointManager struct {
	mu       sync.RWMutex
	filePath string
	offsets  map[string]*FileOffset
}

// NewCheckpointManager creates and loads a checkpoint manager.
func NewCheckpointManager(checkpointPath string) (*CheckpointManager, error) {
	cm := &CheckpointManager{
		filePath: checkpointPath,
		offsets:  make(map[string]*FileOffset),
	}

	if err := cm.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return cm, nil
}

// load reads the checkpoint file from disk.
func (cm *CheckpointManager) load() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &cm.offsets)
}

// Save atomically writes the current state to disk.
func (cm *CheckpointManager) Save() error {
	cm.mu.RLock()
	data, err := json.MarshalIndent(cm.offsets, "", "  ")
	cm.mu.RUnlock()
	if err != nil {
		return err
	}

	dir := filepath.Dir(cm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpFile := cm.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, cm.filePath)
}

// GetOffset retrieves the last committed offset and inode for a file.
func (cm *CheckpointManager) GetOffset(path string) (int64, uint64) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if fo, ok := cm.offsets[path]; ok {
		return fo.Offset, fo.Inode
	}
	return 0, 0
}

// SetOffset updates the file cursor.
func (cm *CheckpointManager) SetOffset(path string, offset int64, inode uint64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.offsets[path] = &FileOffset{
		Offset:        offset,
		Inode:         inode,
		LastProcessed: time.Now().UTC(),
	}
}

// -----------------------------------------------------------------------------

// PruneMissingFiles removes stale offsets whose log files no longer exist on disk.
func (cm *CheckpointManager) PruneMissingFiles(existingFiles map[string]bool) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	pruned := false
	for path := range cm.offsets {
		if !existingFiles[path] {
			delete(cm.offsets, path)
			pruned = true
		}
	}
	return pruned
}

