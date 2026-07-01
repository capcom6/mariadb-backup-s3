package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/scheduler/repository"
)

type JobStatus string

const (
	StatusSuccess JobStatus = "success"
	StatusFailed  JobStatus = "failed"
	StatusRunning JobStatus = "running"
	StatusSkipped JobStatus = "skipped"
)

type JobState struct {
	Name          string    `json:"name"`
	LastRunAt     time.Time `json:"last_run_at"`
	LastRunStatus JobStatus `json:"last_run_status,omitempty"`
	LastRunError  string    `json:"last_run_error,omitempty"`
	LastDuration  string    `json:"last_duration,omitempty"`
	NextRunAt     time.Time `json:"next_run_at"`
	RunCount      int64     `json:"run_count"`
	FailureCount  int64     `json:"failure_count"`
}

type State struct {
	mu       sync.RWMutex
	jobs     map[string]JobState
	filePath string
	dirty    bool
}

func loadState(filePath string) (*State, error) {
	if filePath == "" {
		filePath = repository.DefaultStateFile
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	s := &State{
		mu:       sync.RWMutex{},
		jobs:     make(map[string]JobState),
		filePath: filePath,
		dirty:    false,
	}

	data, readErr := os.ReadFile(filePath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return s, nil
		}
		return nil, fmt.Errorf("read state file: %w", readErr)
	}

	var raw struct {
		Jobs map[string]JobState `json:"jobs"`
	}
	if unmarshalErr := json.Unmarshal(data, &raw); unmarshalErr != nil {
		return nil, fmt.Errorf("parse state file: %w", unmarshalErr)
	}
	if raw.Jobs != nil {
		s.jobs = raw.Jobs
	}

	return s, nil
}

func (s *State) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.filePath)
	if dirErr := os.MkdirAll(dir, 0750); dirErr != nil {
		return fmt.Errorf("create state directory: %w", dirErr)
	}

	tmpPath := s.filePath + ".tmp"
	data, err := json.MarshalIndent(struct {
		Jobs map[string]JobState `json:"jobs"`
	}{Jobs: s.jobs}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if writeErr := os.WriteFile(tmpPath, data, 0600); writeErr != nil {
		return fmt.Errorf("write temp state: %w", writeErr)
	}
	if renameErr := os.Rename(tmpPath, s.filePath); renameErr != nil {
		return fmt.Errorf("rename state file: %w", renameErr)
	}

	return nil
}

func (s *State) RecordRun(name string, status JobStatus, err error, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.jobs[name]
	state.Name = name
	state.LastRunAt = time.Now().UTC()
	state.LastRunStatus = status
	state.LastDuration = duration.Round(time.Second).String()
	state.RunCount++

	if err != nil {
		state.LastRunError = err.Error()
		state.FailureCount++
	} else {
		state.LastRunError = ""
	}

	s.jobs[name] = state
	s.dirty = true
}

func (s *State) RecordNextRun(name string, next time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.jobs[name]
	state.Name = name
	state.NextRunAt = next
	s.jobs[name] = state
	s.dirty = true
}

func (s *State) Status() []JobState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	states := make([]JobState, 0, len(s.jobs))
	for _, st := range s.jobs {
		states = append(states, st)
	}

	return states
}

func (s *State) Close() error {
	s.mu.RLock()
	if !s.dirty {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	return s.Save()
}
