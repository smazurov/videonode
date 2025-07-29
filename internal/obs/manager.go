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
	mutex      sync.RWMutex
	running    bool
	config     ManagerConfig
}

// ManagerConfig represents configuration for the manager
type ManagerConfig struct {
	StoreConfig   StoreConfig   `json:"store_config"`
	DataChanSize  int           `json:"data_chan_size"`
	WorkerCount   int           `json:"worker_count"`
	FlushInterval time.Duration `json:"flush_interval"`
}

// DefaultManagerConfig returns a default configuration
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		StoreConfig:   DefaultStoreConfig(),
		DataChanSize:  10000,
		WorkerCount:   4,
		FlushInterval: 5 * time.Second,
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
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return NewObsError(ErrInvalidConfig, "manager already running", nil)
	}

	log.Println("OBS: Starting observability manager...")

	// Start data processing workers
	for i := 0; i < m.config.WorkerCount; i++ {
		m.wg.Add(1)
		go m.dataWorker(i)
	}

	// Start exporter flushing worker
	m.wg.Add(1)
	go m.exporterWorker()

	// Start all exporters
	for name, exporter := range m.exporters {
		if exporter.Config().Enabled {
			if err := exporter.Start(m.ctx); err != nil {
				log.Printf("OBS: Failed to start exporter %s: %v", name, err)
				continue
			}
			log.Printf("OBS: Started exporter: %s", name)
		}
	}

	// Start all collectors
	for _, collector := range m.collectors.GetAll() {
		if collector.Config().Enabled {
			m.wg.Add(1)
			go m.runCollector(collector)
		}
	}

	m.running = true
	log.Println("OBS: Observability manager started successfully")
	return nil
}

// Stop stops the manager and all collectors/exporters
func (m *Manager) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.running {
		return nil
	}

	log.Println("OBS: Stopping observability manager...")

	// Cancel context to signal all workers to stop
	m.cancel()

	// Stop all collectors
	for name, collector := range m.collectors.GetAll() {
		if err := collector.Stop(); err != nil {
			log.Printf("OBS: Error stopping collector %s: %v", name, err)
		}
	}

	// Stop all exporters
	for name, exporter := range m.exporters {
		if err := exporter.Stop(); err != nil {
			log.Printf("OBS: Error stopping exporter %s: %v", name, err)
		}
	}

	// Wait for all workers to finish
	m.wg.Wait()

	// Close data channel
	close(m.dataChan)

	m.running = false
	log.Println("OBS: Observability manager stopped")
	return nil
}

// AddCollector registers a collector
func (m *Manager) AddCollector(collector Collector) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if err := m.collectors.Register(collector); err != nil {
		return err
	}

	// If manager is running, start the collector immediately
	if m.running && collector.Config().Enabled {
		m.wg.Add(1)
		go m.runCollector(collector)
		log.Printf("OBS: Added and started collector: %s", collector.Name())
	} else {
		log.Printf("OBS: Added collector: %s", collector.Name())
	}

	return nil
}

// RemoveCollector unregisters a collector
func (m *Manager) RemoveCollector(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	log.Printf("OBS: Removing collector: %s", name)
	return m.collectors.Unregister(name)
}

// AddExporter registers an exporter
func (m *Manager) AddExporter(exporter Exporter) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	name := exporter.Name()
	if _, exists := m.exporters[name]; exists {
		return NewObsError(ErrCollectorExists, "exporter already registered", map[string]interface{}{
			"name": name,
		})
	}

	m.exporters[name] = exporter

	// If manager is running, start the exporter immediately
	if m.running && exporter.Config().Enabled {
		if err := exporter.Start(m.ctx); err != nil {
			return NewObsError(ErrExporterFailed, "failed to start exporter", map[string]interface{}{
				"name":  name,
				"error": err.Error(),
			})
		}
		log.Printf("OBS: Added and started exporter: %s", name)
	} else {
		log.Printf("OBS: Added exporter: %s", name)
	}

	return nil
}

// RemoveExporter unregisters an exporter
func (m *Manager) RemoveExporter(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	exporter, exists := m.exporters[name]
	if !exists {
		return NewObsError(ErrCollectorNotFound, "exporter not found", map[string]interface{}{
			"name": name,
		})
	}

	if err := exporter.Stop(); err != nil {
		log.Printf("OBS: Error stopping exporter %s: %v", name, err)
	}

	delete(m.exporters, name)
	log.Printf("OBS: Removed exporter: %s", name)
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
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	collectorStats := make(map[string]interface{})
	for name, collector := range m.collectors.GetAll() {
		collectorStats[name] = map[string]interface{}{
			"enabled":  collector.Config().Enabled,
			"interval": collector.Interval().String(),
		}
	}

	exporterStats := make(map[string]interface{})
	for name, exporter := range m.exporters {
		exporterStats[name] = map[string]interface{}{
			"enabled": exporter.Config().Enabled,
		}
	}

	stats := m.store.Stats()
	stats["running"] = m.running
	stats["collectors"] = collectorStats
	stats["exporters"] = exporterStats
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
		log.Printf("OBS: Warning - data channel full, dropping point: %s", point.String())
	}
}

// runCollector runs a single collector
func (m *Manager) runCollector(collector Collector) {
	defer m.wg.Done()

	name := collector.Name()
	log.Printf("OBS: Starting collector: %s", name)

	// Create a collector-specific context
	collectorCtx, collectorCancel := context.WithCancel(m.ctx)
	defer collectorCancel()

	// Start the collector
	if err := collector.Start(collectorCtx, m.dataChan); err != nil {
		log.Printf("OBS: Failed to start collector %s: %v", name, err)
		return
	}

	// Wait for shutdown signal
	<-collectorCtx.Done()
	log.Printf("OBS: Stopped collector: %s", name)
}

// dataWorker processes incoming data points
func (m *Manager) dataWorker(id int) {
	defer m.wg.Done()

	log.Printf("OBS: Starting data worker %d", id)

	for {
		select {
		case point, ok := <-m.dataChan:
			if !ok {
				log.Printf("OBS: Data worker %d stopping - channel closed", id)
				return
			}

			// Store the data point
			if err := m.store.Add(point); err != nil {
				log.Printf("OBS: Failed to store data point: %v", err)
			}

		case <-m.ctx.Done():
			log.Printf("OBS: Data worker %d stopping - context cancelled", id)
			return
		}
	}
}

// exporterWorker periodically flushes data to exporters
func (m *Manager) exporterWorker() {
	defer m.wg.Done()

	log.Println("OBS: Starting exporter worker")
	ticker := time.NewTicker(m.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.flushToExporters()

		case <-m.ctx.Done():
			log.Println("OBS: Exporter worker stopping")
			// Final flush before stopping
			m.flushToExporters()
			return
		}
	}
}

// flushToExporters sends recent data to all exporters
func (m *Manager) flushToExporters() {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if len(m.exporters) == 0 {
		return
	}

	// Query recent data (last flush interval + buffer)
	end := time.Now()
	start := end.Add(-m.config.FlushInterval * 2)

	// Get recent metrics
	metricsResult, err := m.store.Query(QueryOptions{
		DataType: DataTypeMetric,
		Start:    start,
		End:      end,
	})
	if err != nil {
		log.Printf("OBS: Failed to query metrics for export: %v", err)
		return
	}

	// Get recent logs
	logsResult, err := m.store.Query(QueryOptions{
		DataType: DataTypeLog,
		Start:    start,
		End:      end,
	})
	if err != nil {
		log.Printf("OBS: Failed to query logs for export: %v", err)
		return
	}

	// Combine all points
	var allPoints []DataPoint
	allPoints = append(allPoints, metricsResult.Points...)
	allPoints = append(allPoints, logsResult.Points...)

	if len(allPoints) == 0 {
		return
	}

	// Send to all enabled exporters
	for name, exporter := range m.exporters {
		if exporter.Config().Enabled {
			if err := exporter.Export(allPoints); err != nil {
				log.Printf("OBS: Failed to export to %s: %v", name, err)
			}
		}
	}
}
