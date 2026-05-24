//go:build planv2_tests

package streams

import (
	"maps"
	"sync"

	"github.com/smazurov/videonode/internal/types"
)

// mockStore is a minimal in-memory Store for tests. Lifted from the
// deleted process_manager_test.go so service_test.go still compiles
// after the legacy file deletion.
type mockStore struct {
	mu       sync.RWMutex
	streams  map[string]StreamSpec
	pipeline PipelineConfig
}

func (m *mockStore) Load() error { return nil }
func (m *mockStore) Save() error { return nil }

func (m *mockStore) AddStream(s StreamSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.streams == nil {
		m.streams = make(map[string]StreamSpec)
	}
	m.streams[s.ID] = s
	return nil
}

func (m *mockStore) UpdateStream(id string, s StreamSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.streams == nil {
		m.streams = make(map[string]StreamSpec)
	}
	m.streams[id] = s
	return nil
}

func (m *mockStore) RemoveStream(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.streams, id)
	return nil
}

func (m *mockStore) GetStream(id string) (StreamSpec, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.streams[id]
	return s, ok
}

func (m *mockStore) GetAllStreams() map[string]StreamSpec {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]StreamSpec, len(m.streams))
	maps.Copy(out, m.streams)
	return out
}

func (m *mockStore) GetValidation() *types.ValidationResults { return nil }

func (m *mockStore) UpdateValidation(*types.ValidationResults) error {
	return nil
}

func (m *mockStore) GetPipeline() PipelineConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pipeline
}

func (m *mockStore) SetPipeline(cfg PipelineConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pipeline = cfg
	return nil
}
