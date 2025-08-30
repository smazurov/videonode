package validation

import (
	"fmt"
	"sync"

	"github.com/smazurov/videonode/internal/types"
)

// Manager handles encoder validation data storage and retrieval
type Manager struct {
	mu         sync.RWMutex
	validation *types.ValidationResults
	// Storage backend for persisting validation data
	storage Storage
}

// Storage interface for persisting validation data
type Storage interface {
	Save(validation *types.ValidationResults) error
	Load() (*types.ValidationResults, error)
}

// NewManager creates a new validation manager with the given storage backend
func NewManager(storage Storage) *Manager {
	return &Manager{
		storage: storage,
	}
}

// LoadValidation loads validation data from storage
func (m *Manager) LoadValidation() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.storage == nil {
		return fmt.Errorf("no storage backend configured")
	}

	validation, err := m.storage.Load()
	if err != nil {
		return err
	}

	m.validation = validation
	return nil
}

// SaveValidation saves validation data to storage
func (m *Manager) SaveValidation(validation *types.ValidationResults) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.storage == nil {
		return fmt.Errorf("no storage backend configured")
	}

	if err := m.storage.Save(validation); err != nil {
		return err
	}

	m.validation = validation
	return nil
}

// GetValidation returns the current validation data
func (m *Manager) GetValidation() *types.ValidationResults {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.validation
}

// IsEncoderWorking checks if an encoder is in the working list
func (m *Manager) IsEncoderWorking(encoder string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.validation == nil {
		return false
	}

	// Check both H264 and H265 working lists
	for _, working := range m.validation.H264.Working {
		if working == encoder {
			return true
		}
	}

	for _, working := range m.validation.H265.Working {
		if working == encoder {
			return true
		}
	}

	return false
}

// GetWorkingEncodersForCodec returns working encoders for a specific codec type
func (m *Manager) GetWorkingEncodersForCodec(codecType string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.validation == nil {
		return nil
	}

	switch codecType {
	case "h264", "H264":
		return m.validation.H264.Working
	case "h265", "H265", "hevc", "HEVC":
		return m.validation.H265.Working
	default:
		return nil
	}
}
