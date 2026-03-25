package pdfparser

import (
	"encoding/json"
	"fmt"
	"strings"
)

type OperationStatus struct {
	Operation string `json:"operation"`
	Time      string `json:"time"`
	Status    string `json:"status"`
	Error     string `json:"error"`
}

func hasOperation(rawStatus string, operation string) bool {
	entries, err := decodeStatus(rawStatus)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if strings.EqualFold(strings.TrimSpace(entry.Operation), strings.TrimSpace(operation)) {
			return true
		}
	}

	return false
}

func appendStatusEntry(rawStatus string, entry OperationStatus) (string, error) {
	entries, err := decodeStatus(rawStatus)
	if err != nil {
		return "", err
	}
	entries = append(entries, entry)
	encoded, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("marshal status entries failed: %w", err)
	}
	return string(encoded), nil
}

func decodeStatus(rawStatus string) ([]OperationStatus, error) {
	rawStatus = strings.TrimSpace(rawStatus)
	if rawStatus == "" || rawStatus == "null" {
		return make([]OperationStatus, 0), nil
	}

	var entries []OperationStatus
	if err := json.Unmarshal([]byte(rawStatus), &entries); err == nil {
		return entries, nil
	}

	// Fallback: handle a single object in case legacy rows stored status as object.
	var single OperationStatus
	if err := json.Unmarshal([]byte(rawStatus), &single); err == nil {
		return []OperationStatus{single}, nil
	}

	return nil, fmt.Errorf("invalid status JSON: %s", rawStatus)
}
