package obs

import (
	"context"
	"time"
)

// Collector defines the interface that all data collectors must implement
type Collector interface {
	// Name returns a unique identifier for this collector
	Name() string

	// Start begins data collection. The collector should send data points
	// to the provided channel until the context is cancelled.
	Start(ctx context.Context, dataChan chan<- DataPoint) error

	// Stop gracefully stops the collector
	Stop() error

	// Interval returns the collection interval (0 means no regular interval)
	Interval() time.Duration

	// Config returns the current configuration of the collector
	Config() CollectorConfig

	// UpdateConfig updates the collector's configuration
	UpdateConfig(config CollectorConfig) error
}

// CollectorConfig represents common configuration for collectors
type CollectorConfig struct {
	Name       string                 `json:"name"`
	Enabled    bool                   `json:"enabled"`
	Interval   time.Duration          `json:"interval"`
	Labels     Labels                 `json:"labels"`      // Additional labels to add to all data points
	BufferSize int                    `json:"buffer_size"` // Internal buffer size
	Timeout    time.Duration          `json:"timeout"`     // Operation timeout
	Retries    int                    `json:"retries"`     // Number of retries on failure
	Config     map[string]interface{} `json:"config"`      // Collector-specific configuration
}

// DefaultCollectorConfig returns a default configuration
func DefaultCollectorConfig(name string) CollectorConfig {
	return CollectorConfig{
		Name:       name,
		Enabled:    true,
		Interval:   30 * time.Second,
		Labels:     make(Labels),
		BufferSize: 1000,
		Timeout:    10 * time.Second,
		Retries:    3,
		Config:     make(map[string]interface{}),
	}
}

// CollectorRegistry manages a collection of collectors
type CollectorRegistry struct {
	collectors map[string]Collector
}

// NewCollectorRegistry creates a new collector registry
func NewCollectorRegistry() *CollectorRegistry {
	return &CollectorRegistry{
		collectors: make(map[string]Collector),
	}
}

// Register adds a collector to the registry
func (r *CollectorRegistry) Register(collector Collector) error {
	name := collector.Name()
	if _, exists := r.collectors[name]; exists {
		return NewObsError(ErrCollectorExists, "collector already registered", map[string]interface{}{
			"name": name,
		})
	}
	r.collectors[name] = collector
	return nil
}

// Unregister removes a collector from the registry
func (r *CollectorRegistry) Unregister(name string) error {
	collector, exists := r.collectors[name]
	if !exists {
		return NewObsError(ErrCollectorNotFound, "collector not found", map[string]interface{}{
			"name": name,
		})
	}

	// Stop the collector if it's running
	if err := collector.Stop(); err != nil {
		return NewObsError(ErrCollectorStop, "failed to stop collector", map[string]interface{}{
			"name":  name,
			"error": err.Error(),
		})
	}

	delete(r.collectors, name)
	return nil
}

// Get retrieves a collector by name
func (r *CollectorRegistry) Get(name string) (Collector, bool) {
	collector, exists := r.collectors[name]
	return collector, exists
}

// List returns all registered collector names
func (r *CollectorRegistry) List() []string {
	names := make([]string, 0, len(r.collectors))
	for name := range r.collectors {
		names = append(names, name)
	}
	return names
}

// GetAll returns all registered collectors
func (r *CollectorRegistry) GetAll() map[string]Collector {
	result := make(map[string]Collector)
	for name, collector := range r.collectors {
		result[name] = collector
	}
	return result
}

// BaseCollector provides common functionality for all collectors
type BaseCollector struct {
	name     string
	config   CollectorConfig
	stopChan chan struct{}
	running  bool
}

// NewBaseCollector creates a new base collector
func NewBaseCollector(name string, config CollectorConfig) *BaseCollector {
	return &BaseCollector{
		name:     name,
		config:   config,
		stopChan: make(chan struct{}),
		running:  false,
	}
}

// Name returns the collector name
func (b *BaseCollector) Name() string {
	return b.name
}

// Config returns the collector configuration
func (b *BaseCollector) Config() CollectorConfig {
	return b.config
}

// UpdateConfig updates the collector configuration
func (b *BaseCollector) UpdateConfig(config CollectorConfig) error {
	b.config = config
	return nil
}

// Interval returns the collection interval
func (b *BaseCollector) Interval() time.Duration {
	return b.config.Interval
}

// Stop stops the collector
func (b *BaseCollector) Stop() error {
	if b.running {
		close(b.stopChan)
		b.running = false
	}
	return nil
}

// IsRunning returns whether the collector is currently running
func (b *BaseCollector) IsRunning() bool {
	return b.running
}

// SetRunning sets the running status
func (b *BaseCollector) SetRunning(running bool) {
	b.running = running
}

// StopChan returns the stop channel
func (b *BaseCollector) StopChan() <-chan struct{} {
	return b.stopChan
}

// AddLabels adds the configured labels to a labels map
func (b *BaseCollector) AddLabels(labels Labels) Labels {
	result := make(Labels)

	// Add existing labels
	for k, v := range labels {
		result[k] = v
	}

	// Add collector labels (may override existing)
	for k, v := range b.config.Labels {
		result[k] = v
	}

	// Always add collector name
	result["collector"] = b.name

	return result
}
