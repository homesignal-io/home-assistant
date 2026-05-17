package pipeline

import (
	"context"
	"sync"
)

type MemoryWriter struct {
	mu       sync.Mutex
	Messages []ValidatedMessage
}

func (w *MemoryWriter) WriteLatest(_ context.Context, message ValidatedMessage) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Messages = append(w.Messages, message)
	return nil
}

func (w *MemoryWriter) Count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.Messages)
}

type MemoryDedupeStore struct {
	mu             sync.Mutex
	messages       map[MessageDedupeKey]struct{}
	materialHashes map[StateDedupeKey]string
}

func NewMemoryDedupeStore() *MemoryDedupeStore {
	return &MemoryDedupeStore{
		messages:       map[MessageDedupeKey]struct{}{},
		materialHashes: map[StateDedupeKey]string{},
	}
}

func (s *MemoryDedupeStore) SeenMessage(_ context.Context, key MessageDedupeKey) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.messages[key]
	return ok, nil
}

func (s *MemoryDedupeStore) RecordMessage(_ context.Context, key MessageDedupeKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[key] = struct{}{}
	return nil
}

func (s *MemoryDedupeStore) LastMaterialHash(_ context.Context, key StateDedupeKey) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.materialHashes[key]
	return value, ok, nil
}

func (s *MemoryDedupeStore) RecordMaterialHash(_ context.Context, key StateDedupeKey, materialHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.materialHashes[key] = materialHash
	return nil
}

type MemoryFailureSink struct {
	mu       sync.Mutex
	Failures []IngestFailure
}

func (s *MemoryFailureSink) RecordFailure(_ context.Context, failure IngestFailure) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Failures = append(s.Failures, failure)
	return nil
}

func (s *MemoryFailureSink) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Failures)
}
