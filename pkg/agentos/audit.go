package agentos

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type AuditEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	ActorID   string                 `json:"actor_id"`
	Action    string                 `json:"action"`
	Resource  string                 `json:"resource"`
	Allowed   bool                   `json:"allowed"`
	Reason    string                 `json:"reason,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

type AuditLog interface {
	Record(entry AuditEntry) error
	Query(filter AuditFilter) ([]AuditEntry, error)
}

type AuditFilter struct {
	ActorID  string
	Action   string
	Resource string
	Since    time.Time
	Limit    int
}

type MemoryAuditLog struct {
	mu      sync.RWMutex
	entries []AuditEntry
	maxSize int
}

func NewMemoryAuditLog(maxSize int) *MemoryAuditLog {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &MemoryAuditLog{
		entries: make([]AuditEntry, 0, 256),
		maxSize: maxSize,
	}
}

func (l *MemoryAuditLog) Record(entry AuditEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	l.entries = append(l.entries, entry)
	if len(l.entries) > l.maxSize {
		l.entries = l.entries[len(l.entries)-l.maxSize:]
	}
	return nil
}

func (l *MemoryAuditLog) Query(filter AuditFilter) ([]AuditEntry, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	var result []AuditEntry
	for i := len(l.entries) - 1; i >= 0 && len(result) < limit; i-- {
		e := l.entries[i]
		if filter.ActorID != "" && e.ActorID != filter.ActorID {
			continue
		}
		if filter.Action != "" && e.Action != filter.Action {
			continue
		}
		if filter.Resource != "" && e.Resource != filter.Resource {
			continue
		}
		if !filter.Since.IsZero() && e.Timestamp.Before(filter.Since) {
			continue
		}
		result = append(result, e)
	}
	return result, nil
}

func (l *MemoryAuditLog) Entries() []AuditEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	cp := make([]AuditEntry, len(l.entries))
	copy(cp, l.entries)
	return cp
}

type FileAuditLog struct {
	mu   sync.Mutex
	path string
}

func NewFileAuditLog(path string) (*FileAuditLog, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}
	return &FileAuditLog{path: path}, nil
}

func (l *FileAuditLog) Record(entry AuditEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open audit log: %w", err)
	}
	defer f.Close()
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal audit entry: %w", err)
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

func (l *FileAuditLog) Query(filter AuditFilter) ([]AuditEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	data, err := os.ReadFile(l.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read audit log: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	var result []AuditEntry
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var e AuditEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if filter.ActorID != "" && e.ActorID != filter.ActorID {
			continue
		}
		if filter.Action != "" && e.Action != filter.Action {
			continue
		}
		if filter.Resource != "" && e.Resource != filter.Resource {
			continue
		}
		if !filter.Since.IsZero() && e.Timestamp.Before(filter.Since) {
			continue
		}
		result = append(result, e)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
