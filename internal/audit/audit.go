package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Entry struct {
	Time    string `json:"time"`
	Task    string `json:"task,omitempty"`
	Step    int    `json:"step"`
	Kind    string `json:"kind"`
	Target  string `json:"target,omitempty"`
	Outcome string `json:"outcome"`
	Detail  string `json:"detail,omitempty"`
}

type Logger struct {
	mu      sync.Mutex
	writer  io.Writer
	encoder *json.Encoder
}

func NewLogger(writer io.Writer) *Logger {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return &Logger{writer: writer, encoder: encoder}
}

func Open(path string) (*Logger, func() error, error) {
	if path == "" || path == "-" {
		return NewLogger(os.Stderr), func() error { return nil }, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open audit log: %w", err)
	}
	return NewLogger(file), file.Close, nil
}

func (l *Logger) Record(entry Entry) {
	if l == nil {
		return
	}
	if entry.Time == "" {
		entry.Time = time.Now().UTC().Format(time.RFC3339)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.encoder.Encode(entry)
}
