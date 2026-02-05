package logs2db

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Location codes for state operations
const (
	LOC_STATE_LOAD  = "SHD_L2D_030"
	LOC_STATE_SAVE  = "SHD_L2D_031"
	LOC_STATE_RESET = "SHD_L2D_032"
)

// FileState tracks the loading progress for a single log file.
type FileState struct {
	LastLine     int       `json:"last_line"`
	LastLoadedAt time.Time `json:"last_loaded_at"`
}

// StateData is the root structure of the state file.
type StateData struct {
	Version int                   `json:"version"`
	Files   map[string]*FileState `json:"files"`
}

// StateManager handles reading and writing the state file.
type StateManager struct {
	filePath string
	data     *StateData
	mu       sync.Mutex
}

// NewStateManager creates a new state manager for the given file path.
func NewStateManager(filePath string) *StateManager {
	return &StateManager{
		filePath: filePath,
		data: &StateData{
			Version: 1,
			Files:   make(map[string]*FileState),
		},
	}
}

// Load reads the state file from disk. If it doesn't exist, initializes empty state.
func (sm *StateManager) Load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	data, err := os.ReadFile(sm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No state file yet, start fresh
			sm.data = &StateData{
				Version: 1,
				Files:   make(map[string]*FileState),
			}
			return nil
		}
		return fmt.Errorf("failed to read state file: %w (%s)", err, LOC_STATE_LOAD)
	}

	var state StateData
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to parse state file: %w (%s)", err, LOC_STATE_LOAD)
	}

	if state.Files == nil {
		state.Files = make(map[string]*FileState)
	}

	sm.data = &state
	return nil
}

// Save atomically writes the state file (write to temp file, then rename).
func (sm *StateManager) Save() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	return sm.saveLocked()
}

func (sm *StateManager) saveLocked() error {
	data, err := json.MarshalIndent(sm.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w (%s)", err, LOC_STATE_SAVE)
	}

	// Write to temp file first, then rename for atomicity
	dir := filepath.Dir(sm.filePath)
	tmpFile, err := os.CreateTemp(dir, ".log2db_state_*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp state file: %w (%s)", err, LOC_STATE_SAVE)
	}

	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp state file: %w (%s)", err, LOC_STATE_SAVE)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp state file: %w (%s)", err, LOC_STATE_SAVE)
	}

	if err := os.Rename(tmpPath, sm.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename state file: %w (%s)", err, LOC_STATE_SAVE)
	}

	return nil
}

// GetLastLine returns the last loaded line number for a given file.
// Returns 0 if the file has not been tracked yet.
func (sm *StateManager) GetLastLine(filename string) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if fs, ok := sm.data.Files[filename]; ok {
		return fs.LastLine
	}
	return 0
}

// SetLastLine updates the last loaded line for a file and saves the state.
func (sm *StateManager) SetLastLine(filename string, line int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.data.Files[filename] = &FileState{
		LastLine:     line,
		LastLoadedAt: time.Now(),
	}

	return sm.saveLocked()
}

// Reset clears all state (for reload).
func (sm *StateManager) Reset() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.data = &StateData{
		Version: 1,
		Files:   make(map[string]*FileState),
	}

	return sm.saveLocked()
}

// GetTrackedFiles returns the list of filenames that have been loaded.
func (sm *StateManager) GetTrackedFiles() []string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	files := make([]string, 0, len(sm.data.Files))
	for f := range sm.data.Files {
		files = append(files, f)
	}
	return files
}

// RemoveFile removes a file from the tracked state.
func (sm *StateManager) RemoveFile(filename string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.data.Files, filename)
	return sm.saveLocked()
}
