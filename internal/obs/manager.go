package obs

import (
	"context"
	"log"
	"sync"
	"time"
)

// Exporter defines the interface for exporting observability data
type Exporter interface {
	Name() string
	Start(ctx context.Context) error
	Stop() error
	Export(points []DataPoint) error
	Config() ExporterConfig
}

// ExporterConfig represents configuration for exporters
type ExporterConfig struct {
	Name          string                 `json:"name"`
	Enabled       bool                   `json:"enabled"`
	BufferSize    int                    `json:"buffer_size"`
	FlushInterval time.Duration          `json:"flush_interval"`
	Config        map[string]interface{} `json:"config"`
}

// Manager coordinates collectors, store, and exporters
type Manager struct {
	store      *Store
	collectors *CollectorRegistry
	exporters  map[string]Exporter
	dataChan   chan DataPoint
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	config     ManagerConfig
}

// ManagerConfig represents configuration for the manager
type ManagerConfig struct {
	StoreConfig  StoreConfig `json:"store_config"`
	DataChanSize int         `json:"data_chan_size"`
	WorkerCount  int         `json:"worker_count"`
	LogLevel     string      `json:"log_level"`
}

// DefaultManagerConfig returns a default configuration
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		StoreConfig:  DefaultStoreConfig(),
		DataChanSize: 10000,
		WorkerCount:  4,
		LogLevel:     "info",
	}
}

// NewManager creates a new observability manager
func NewManager(config ManagerConfig) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		store:      NewStore(config.StoreConfig),
		collectors: NewCollectorRegistry(),
		exporters:  make(map[string]Exporter),
		dataChan:   make(chan DataPoint, config.DataChanSize),
		ctx:        ctx,
		cancel:     cancel,
		config:     config,
	}
}

// Start starts the manager and all registered collectors/exporters
func (m *Manager) Start() error {
	log.Println("OBS: Starting")

	// Start data workers
	for i := 0; i < m.config.WorkerCount; i++ {
		m.wg.Add(1)
		go m.dataWorker(i)
	}

	// Start exporters
	for name, exporter := range m.exporters {
		if exporter.Config().Enabled {
			if err := exporter.Start(m.ctx); err != nil {
				log.Printf("OBS: Failed to start exporter %s: %v", name, err)
			}
		}
	}

	// Start collectors
	for _, collector := range m.collectors.GetAll() {
		if collector.Config().Enabled {
			m.wg.Add(1)
			go m.runCollector(collector)
		}
	}

	return nil
}

// Stop stops the manager and all collectors/exporters
func (m *Manager) Stop() error {
	log.Println("OBS: Stopping")

	// Cancel context - this signals all goroutines to stop
	m.cancel()

	// Stop collectors explicitly (they should also stop via context)
	for _, collector := range m.collectors.GetAll() {
		collector.Stop()
	}

	// Stop exporters
	for _, exporter := range m.exporters {
		exporter.Stop()
	}

	// Wait for all goroutines to finish BEFORE closing the channel
	// This ensures no goroutine will try to send on a closed channel
	m.wg.Wait()

	// NOW it's safe to close the channel - all senders are done
	close(m.dataChan)

	return nil
}

// AddCollector registers a collector
func (m *Manager) AddCollector(collector Collector) error {
	err := m.collectors.Register(collector)
	if err != nil {
		return err
	}

	// If manager is already running and collector is enabled, start it immediately
	if m.ctx != nil && m.ctx.Err() == nil && collector.Config().Enabled {
		log.Printf("OBS: Manager is running, starting collector %s immediately", collector.Name())
		m.wg.Add(1)
		go m.runCollector(collector)
	}

	return nil
}

// RemoveCollector unregisters a collector
func (m *Manager) RemoveCollector(name string) error {
	collector, exists := m.collectors.Get(name)
	if !exists {
		// Not an error - collector already doesn't exist
		return nil
	}
	collector.Stop()
	return m.collectors.Unregister(name)
}

// AddExporter registers an exporter
func (m *Manager) AddExporter(exporter Exporter) error {
	name := exporter.Name()
	if _, exists := m.exporters[name]; exists {
		return NewObsError(ErrCollectorExists, "exporter already registered", map[string]interface{}{"name": name})
	}
	m.exporters[name] = exporter
	return nil
}

// RemoveExporter unregisters an exporter
func (m *Manager) RemoveExporter(name string) error {
	exporter, exists := m.exporters[name]
	if !exists {
		return NewObsError(ErrCollectorNotFound, "exporter not found", map[string]interface{}{"name": name})
	}
	exporter.Stop()
	delete(m.exporters, name)
	return nil
}

// Query queries the data store
func (m *Manager) Query(opts QueryOptions) (*QueryResult, error) {
	return m.store.Query(opts)
}

// ListSeries returns information about all time series
func (m *Manager) ListSeries() []SeriesInfo {
	return m.store.ListSeries()
}

// Stats returns statistics about the manager
func (m *Manager) Stats() map[string]interface{} {
	stats := m.store.Stats()
	stats["running"] = m.ctx.Err() == nil
	stats["data_chan_size"] = len(m.dataChan)
	stats["data_chan_capacity"] = cap(m.dataChan)
	return stats
}

// SendData sends a data point to the manager (used by collectors)
func (m *Manager) SendData(point DataPoint) {
	select {
	case m.dataChan <- point:
		// Successfully sent
	default:
		// Channel is full, drop the point
		log.Printf("OBS: Warning - data channel full, dropping point")
	}
}

// runCollector runs a single collector
func (m *Manager) runCollector(collector Collector) {
	defer m.wg.Done()

	// Start the collector - this blocks until context is cancelled
	if err := collector.Start(m.ctx, m.dataChan); err != nil {
		log.Printf("OBS: Collector %s error: %v", collector.Name(), err)
	}
}

// dataWorker processes incoming data points
func (m *Manager) dataWorker(id int) {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		case point, ok := <-m.dataChan:
			if !ok {
				return
			}

			// Store the data point
			if err := m.store.Add(point); err != nil {
				log.Printf("OBS: Failed to store data point: %v", err)
			}

			// Export to all enabled exporters
			for _, exporter := range m.exporters {
				if exporter.Config().Enabled {
					if err := exporter.Export([]DataPoint{point}); err != nil {
						log.Printf("OBS: Failed to export: %v", err)
					}
				}
			}
		}
	}
}
