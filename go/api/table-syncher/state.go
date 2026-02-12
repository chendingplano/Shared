package tablesyncher

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
	LOC_STATE_LOAD  = "SHD_SYN_030"
	LOC_STATE_SAVE  = "SHD_SYN_031"
	LOC_STATE_RESET = "SHD_SYN_032"
)

// TableState tracks the synchronization progress for a single table.
type TableState struct {
	LastLSN      string    `json:"last_lsn"`      // Last processed LSN
	LastSyncedAt time.Time `json:"last_synced_at"`
	RecordCount  int64     `json:"record_count"` // Total records synced for this table
}

// StateData is the root structure of the state file.
type StateData struct {
	Version        int                    `json:"version"`
	LastFile       string                 `json:"last_file"`       // Last processed change file
	LastFileTime   time.Time              `json:"last_file_time"`  // Modification time of last file
	GlobalLSN      string                 `json:"global_lsn"`      // Global checkpoint LSN
	Tables         map[string]*TableState `json:"tables"`
	TotalSynced    int64                  `json:"total_synced"`    // Total records synced since start
	LastSyncCycle  time.Time              `json:"last_sync_cycle"` // Time of last sync cycle
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
			Tables:  make(map[string]*TableState),
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
				Tables:  make(map[string]*TableState),
			}
			return nil
		}
		return fmt.Errorf("failed to read state file: %w (%s)", err, LOC_STATE_LOAD)
	}

	var state StateData
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to parse state file: %w (%s)", err, LOC_STATE_LOAD)
	}

	if state.Tables == nil {
		state.Tables = make(map[string]*TableState)
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
	tmpFile, err := os.CreateTemp(dir, ".syncdata_state_*.tmp")
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

// GetLastFile returns the last processed change file name.
func (sm *StateManager) GetLastFile() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.data.LastFile
}

// GetLastFileTime returns the modification time of the last processed file.
func (sm *StateManager) GetLastFileTime() time.Time {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.data.LastFileTime
}

// SetLastFile updates the last processed file and saves the state.
func (sm *StateManager) SetLastFile(filename string, modTime time.Time) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.data.LastFile = filename
	sm.data.LastFileTime = modTime
	sm.data.LastSyncCycle = time.Now()

	return sm.saveLocked()
}

// GetGlobalLSN returns the global checkpoint LSN.
func (sm *StateManager) GetGlobalLSN() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.data.GlobalLSN
}

// SetGlobalLSN updates the global checkpoint LSN.
func (sm *StateManager) SetGlobalLSN(lsn string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.data.GlobalLSN = lsn
	return sm.saveLocked()
}

// GetTableState returns the state for a specific table.
func (sm *StateManager) GetTableState(tableName string) *TableState {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if ts, ok := sm.data.Tables[tableName]; ok {
		return ts
	}
	return nil
}

// UpdateTableState updates the state for a specific table.
func (sm *StateManager) UpdateTableState(tableName, lsn string, recordsDelta int64) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.data.Tables[tableName] == nil {
		sm.data.Tables[tableName] = &TableState{}
	}

	sm.data.Tables[tableName].LastLSN = lsn
	sm.data.Tables[tableName].LastSyncedAt = time.Now()
	sm.data.Tables[tableName].RecordCount += recordsDelta
	sm.data.TotalSynced += recordsDelta

	return sm.saveLocked()
}

// GetTotalSynced returns the total number of records synced.
func (sm *StateManager) GetTotalSynced() int64 {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.data.TotalSynced
}

// GetLastSyncCycle returns the time of the last sync cycle.
func (sm *StateManager) GetLastSyncCycle() time.Time {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.data.LastSyncCycle
}

// Reset clears all state (for full resync).
func (sm *StateManager) Reset() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.data = &StateData{
		Version: 1,
		Tables:  make(map[string]*TableState),
	}

	return sm.saveLocked()
}

// ResetTable clears state for a specific table.
func (sm *StateManager) ResetTable(tableName string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.data.Tables, tableName)
	return sm.saveLocked()
}

// GetTrackedTables returns the list of tables that have sync state.
func (sm *StateManager) GetTrackedTables() []string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	tables := make([]string, 0, len(sm.data.Tables))
	for t := range sm.data.Tables {
		tables = append(tables, t)
	}
	return tables
}
