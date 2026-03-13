package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// Sink receives sealed audit events for storage.
type Sink interface {
	Write(evt *Event) error
	Close() error
}

// FileSink writes NDJSON events to a file. Each event is one line.
type FileSink struct {
	mu   sync.Mutex
	file *os.File
	enc  *json.Encoder
}

// NewFileSink creates a sink that appends NDJSON to the given path.
func NewFileSink(path string) (*FileSink, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("audit: open log file: %w", err)
	}
	return &FileSink{
		file: f,
		enc:  json.NewEncoder(f),
	}, nil
}

func (s *FileSink) Write(evt *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enc.Encode(evt)
}

func (s *FileSink) Close() error {
	return s.file.Close()
}

// WriterSink writes NDJSON events to any io.Writer.
type WriterSink struct {
	mu  sync.Mutex
	enc *json.Encoder
}

// NewWriterSink creates a sink that writes NDJSON to w.
func NewWriterSink(w io.Writer) *WriterSink {
	return &WriterSink{enc: json.NewEncoder(w)}
}

func (s *WriterSink) Write(evt *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enc.Encode(evt)
}

func (s *WriterSink) Close() error {
	return nil
}

// MultiSink fans out events to multiple sinks.
type MultiSink struct {
	sinks []Sink
}

// NewMultiSink creates a sink that writes to all provided sinks.
func NewMultiSink(sinks ...Sink) *MultiSink {
	return &MultiSink{sinks: sinks}
}

func (m *MultiSink) Write(evt *Event) error {
	var firstErr error
	for _, s := range m.sinks {
		if err := s.Write(evt); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *MultiSink) Close() error {
	var firstErr error
	for _, s := range m.sinks {
		if err := s.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
