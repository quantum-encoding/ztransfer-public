package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// BigQuerySink streams audit events to a BigQuery table using the
// Storage Write API (insertAll). Events are buffered and flushed
// periodically or when the buffer reaches a threshold.
//
// Table schema must match the Event struct fields. Create with:
//
//	CREATE TABLE `project.dataset.session_audit` (
//	  session_id STRING NOT NULL,
//	  sequence INT64 NOT NULL,
//	  previous_hash STRING NOT NULL,
//	  hash STRING NOT NULL,
//	  timestamp TIMESTAMP NOT NULL,
//	  event_type STRING NOT NULL,
//	  actor_id STRING NOT NULL,
//	  target_id STRING,
//	  description STRING,
//	  redacted BOOL,
//	  command STRING,
//	  exit_code INT64,
//	  byte_count INT64,
//	  file_name STRING,
//	  direction STRING,
//	  error_msg STRING,
//	  metadata JSON
//	);
type BigQuerySink struct {
	mu        sync.Mutex
	projectID string
	datasetID string
	tableID   string
	token     TokenSource
	buffer    []bqRow
	client    *http.Client

	// Config
	flushSize     int
	flushInterval time.Duration

	// Background flush
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// TokenSource provides OAuth2 access tokens for BigQuery API calls.
// This is satisfied by ztransfer-mint's token machinery or any
// oauth2.TokenSource implementation.
type TokenSource interface {
	Token() (string, error)
}

// StaticToken is a TokenSource that always returns the same token.
// Useful for short-lived sessions where the token won't expire.
type StaticToken string

func (t StaticToken) Token() (string, error) {
	return string(t), nil
}

// bqRow is the insertAll API row format.
type bqRow struct {
	InsertID string          `json:"insertId"`
	JSON     json.RawMessage `json:"json"`
}

// BigQueryConfig configures the BigQuery sink.
type BigQueryConfig struct {
	ProjectID string
	DatasetID string
	TableID   string
	Token     TokenSource

	// FlushSize is the number of events to buffer before flushing.
	// Default: 10
	FlushSize int

	// FlushInterval is the maximum time between flushes.
	// Default: 5s
	FlushInterval time.Duration
}

// NewBigQuerySink creates a sink that streams events to BigQuery.
func NewBigQuerySink(cfg BigQueryConfig) *BigQuerySink {
	if cfg.FlushSize <= 0 {
		cfg.FlushSize = 10
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 5 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &BigQuerySink{
		projectID:     cfg.ProjectID,
		datasetID:     cfg.DatasetID,
		tableID:       cfg.TableID,
		token:         cfg.Token,
		buffer:        make([]bqRow, 0, cfg.FlushSize),
		client:        &http.Client{Timeout: 30 * time.Second},
		flushSize:     cfg.FlushSize,
		flushInterval: cfg.FlushInterval,
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
	}

	go s.flushLoop()
	return s
}

func (s *BigQuerySink) Write(evt *Event) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("audit/bigquery: marshal event: %w", err)
	}

	row := bqRow{
		InsertID: fmt.Sprintf("%s-%d", evt.SessionID, evt.Sequence),
		JSON:     data,
	}

	s.mu.Lock()
	s.buffer = append(s.buffer, row)
	shouldFlush := len(s.buffer) >= s.flushSize
	s.mu.Unlock()

	if shouldFlush {
		return s.flush()
	}
	return nil
}

func (s *BigQuerySink) Close() error {
	s.cancel()
	<-s.done
	// Final flush of remaining events
	return s.flush()
}

func (s *BigQuerySink) flushLoop() {
	defer close(s.done)
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.flush()
		}
	}
}

func (s *BigQuerySink) flush() error {
	s.mu.Lock()
	if len(s.buffer) == 0 {
		s.mu.Unlock()
		return nil
	}
	rows := s.buffer
	s.buffer = make([]bqRow, 0, s.flushSize)
	s.mu.Unlock()

	return s.insertRows(rows)
}

func (s *BigQuerySink) insertRows(rows []bqRow) error {
	token, err := s.token.Token()
	if err != nil {
		return fmt.Errorf("audit/bigquery: get token: %w", err)
	}

	// BigQuery insertAll API
	url := fmt.Sprintf(
		"https://bigquery.googleapis.com/bigquery/v2/projects/%s/datasets/%s/tables/%s/insertAll",
		s.projectID, s.datasetID, s.tableID,
	)

	payload := struct {
		Kind                string  `json:"kind"`
		SkipInvalidRows     bool    `json:"skipInvalidRows"`
		IgnoreUnknownValues bool    `json:"ignoreUnknownValues"`
		Rows                []bqRow `json:"rows"`
	}{
		Kind:                "bigquery#tableDataInsertAllRequest",
		SkipInvalidRows:     false,
		IgnoreUnknownValues: false,
		Rows:                rows,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("audit/bigquery: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(s.ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("audit/bigquery: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("audit/bigquery: API call: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return fmt.Errorf("audit/bigquery: API %d: %s", resp.StatusCode, respBody)
	}

	// Check for per-row errors
	var result struct {
		InsertErrors []struct {
			Index  int `json:"index"`
			Errors []struct {
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"insertErrors"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && len(result.InsertErrors) > 0 {
		return fmt.Errorf("audit/bigquery: %d row errors, first: %s",
			len(result.InsertErrors), result.InsertErrors[0].Errors[0].Message)
	}

	return nil
}
