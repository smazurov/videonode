# OBS (Observability) Package

The OBS package provides a comprehensive observability solution for the VideoNode application, supporting metrics collection, log aggregation, and real-time data export.

## Architecture

```
┌─────────────────┐    ┌──────────────┐    ┌─────────────────┐
│   Collectors    │───▶│   Manager    │───▶│   Exporters     │
├─────────────────┤    ├──────────────┤    ├─────────────────┤
│ • System        │    │ • Data Chan  │    │ • Prometheus    │
│ • FFmpeg        │    │ • Store      │    │ • SSE           │
│ • Log Files     │    │ • Workers    │    │ • JSON API      │
│ • Prometheus    │    │ • Lifecycle  │    └─────────────────┘
└─────────────────┘    └──────────────┘
                              │
                       ┌──────────────┐
                       │  Ring Buffer │
                       │   Storage    │
                       └──────────────┘
```

## Features

### Data Collection
- **System Metrics**: CPU, memory, disk, network statistics
- **FFmpeg Monitoring**: Progress data and log analysis
- **Log File Tailing**: Real-time log monitoring with level detection
- **Prometheus Scraping**: Import metrics from external Prometheus endpoints

### Data Storage
- **In-Memory Time Series**: Efficient ring buffer storage
- **Configurable Retention**: Time-based and size-based limits
- **Label-based Indexing**: Fast queries by metric name and labels
- **Automatic Cleanup**: Expired data removal

### Data Export
- **Prometheus**: Standard /metrics endpoint for monitoring
- **Server-Sent Events**: Real-time updates for frontend
- **JSON API**: RESTful endpoints for historical data queries

## Quick Start

### Easiest Setup (Recommended)

```go
package main

import (
    "log"
    "github.com/smazurov/videonode/internal/obs"
)

func main() {
    // Quick start with defaults
    manager, err := obs.QuickStart()
    if err != nil {
        log.Fatal(err)
    }
    
    // Start observability
    if err := manager.Start(); err != nil {
        log.Fatal(err)
    }
    defer manager.Stop()
    
    // Your application code here...
    // View metrics at http://localhost:8092/obs/metrics
}
```

### Production Setup for VideoNode

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/smazurov/videonode/internal/obs"
)

func main() {
    // Setup for VideoNode with existing systems
    promRegistry := prometheus.NewRegistry()
    sseBroadcaster := &YourSSEBroadcaster{} // Implement SSEBroadcaster interface
    
    manager, err := obs.SetupForVideoNode(promRegistry, sseBroadcaster)
    if err != nil {
        log.Fatal(err)
    }
    
    if err := manager.Start(); err != nil {
        log.Fatal(err)
    }
    defer manager.Stop()
}
```

### With Custom Configuration

```go
// Create custom configuration
config := obs.ManagerConfig{
    StoreConfig: obs.StoreConfig{
        MaxRetentionDuration: 24 * time.Hour,
        MaxPointsPerSeries:   86400,
        MaxSeries:            10000,
        FlushInterval:        30 * time.Second,
    },
    DataChanSize:  10000,
    WorkerCount:   4,
    FlushInterval: 5 * time.Second,
}

manager := obs.NewManager(config)

// Add system collector with custom settings
systemCollector := collectors.NewSystemCollector()
systemCollector.UpdateConfig(obs.CollectorConfig{
    Name:     "system_metrics",
    Enabled:  true,
    Interval: 30 * time.Second,
    Labels:   obs.Labels{"instance": "production"},
})
manager.AddCollector(systemCollector)

// Add Prometheus scraper with filtering
promCollector := collectors.NewPrometheusCollector("http://localhost:9998/metrics")
promCollector.SetMetricFilter("^(paths|paths_bytes).*")
manager.AddCollector(promCollector)
```

### Setup Helpers

For common scenarios, use the provided setup helpers:

```go
// Quick development setup
manager, err := obs.QuickStart()

// Production optimized setup
opts := obs.ProductionSetupOptions()
opts.PrometheusEndpoint = "http://localhost:9998/metrics"
opts.PrometheusFilter = "^(paths|streams).*"
manager, err := obs.Setup(opts)

// Add monitoring to existing manager
obs.WithFFmpegMonitoring(manager, "stream_001", "/tmp/ffmpeg.sock", "/var/log/ffmpeg.log")
obs.WithLogFileMonitoring(manager, "/var/log/app.log", "application")
obs.WithPrometheusEndpoint(manager, "external_service", "http://service:9090/metrics", "")
```

## Collectors

### System Collector

Collects system-level metrics:

```go
systemCollector := collectors.NewSystemCollector()
```

**Metrics:**
- `cpu_usage_percent` - CPU utilization
- `memory_usage_percent` - Memory utilization  
- `disk_usage_percent` - Disk utilization
- `network_receive_bytes_total` - Network traffic
- `load_average` - System load

### FFmpeg Collector

Monitors FFmpeg processes via Unix sockets and log files:

```go
ffmpegCollector := collectors.NewFFmpegCollector(
    "/tmp/ffmpeg_progress.sock",
    "/var/log/ffmpeg.log", 
    "stream_001"
)
```

**Metrics:**
- `ffmpeg_fps` - Frames per second
- `ffmpeg_dropped_frames` - Dropped frame count
- `ffmpeg_processing_speed` - Processing speed ratio

**Logs:**
- Error/warning detection from FFmpeg logs
- Progress tracking
- Performance metrics extraction

### Prometheus Collector

Scrapes external Prometheus endpoints:

```go
promCollector := collectors.NewPrometheusCollector("http://localhost:9998/metrics")
promCollector.SetMetricFilter("^(paths|rtmp).*") // Optional filtering
```

### Log File Collector

Monitors log files for new entries:

```go
logCollector := collectors.NewLogFileCollector("/var/log/app.log", "application")
```

**Features:**
- Real-time log tailing
- Automatic log level detection
- Metric extraction from log patterns
- Log rotation handling

## Exporters

### Prometheus Exporter

Exposes metrics in Prometheus format:

```go
promExporter := exporters.NewPrometheusExporter(registry)
manager.AddExporter(promExporter)

// Serve metrics
http.Handle("/metrics/obs", promExporter.GetHandler())
```

### SSE Exporter

Provides real-time updates via Server-Sent Events:

```go
sseExporter := exporters.NewSSEExporter(sseBroadcaster)
manager.AddExporter(sseExporter)
```

**Events:**
- `obs-metrics` - Metric updates
- `obs-logs` - Log entries  
- `obs-alert` - Alert notifications
- Chart data for frontend visualization

### JSON API Exporter

RESTful API for historical data queries:

```go
jsonExporter := exporters.NewJSONExporter(store, ":8091")
manager.AddExporter(jsonExporter)
```

**Endpoints:**
- `GET /obs/metrics` - Query metrics
- `GET /obs/logs` - Query logs
- `GET /obs/series` - List time series
- `GET /obs/stats` - Store statistics

**Query Parameters:**
- `start`, `end` - Time range (RFC3339 format)
- `limit` - Max results
- `name` - Metric/log source name
- Custom labels for filtering

## Data Types

### Metrics

```go
metric := &obs.MetricPoint{
    Name:       "custom_metric",
    Value:      42.0,
    LabelsMap:  obs.Labels{"component": "api"},
    Timestamp_: time.Now(),
    Unit:       "requests/sec",
}
manager.SendData(metric)
```

### Logs

```go
logEntry := &obs.LogEntry{
    Message:    "Operation completed successfully",
    Level:      obs.LogLevelInfo,
    LabelsMap:  obs.Labels{"operation": "process"},
    Fields:     map[string]interface{}{"duration": "2.5s"},
    Timestamp_: time.Now(),
    Source:     "api_handler",
}
manager.SendData(logEntry)
```

## Querying Data

### Via Manager

```go
result, err := manager.Query(obs.QueryOptions{
    DataType: obs.DataTypeMetric,
    Name:     "cpu_usage_percent",
    Start:    time.Now().Add(-1 * time.Hour),
    End:      time.Now(),
    Labels:   obs.Labels{"instance": "production"},
})
```

### Via HTTP API

```bash
# Get recent metrics
curl "http://localhost:8091/obs/metrics?start=2024-01-01T10:00:00Z&limit=100"

# Get error logs
curl "http://localhost:8091/obs/logs?level=error&start=2024-01-01T10:00:00Z"

# Get specific metric
curl "http://localhost:8091/obs/query?type=metric&name=cpu_usage_percent&instance=production"
```

## Integration Examples

### With Existing Prometheus Registry

```go
// Reuse existing registry
existingRegistry := prometheus.NewRegistry()
promExporter := exporters.NewPrometheusExporter(existingRegistry)
```

### With Existing SSE System

```go
// Implement SSEBroadcaster interface
type MySSEManager struct {
    // Your SSE implementation
}

func (m *MySSEManager) BroadcastEvent(eventType string, data interface{}) error {
    // Forward to your SSE system
    return nil
}

sseExporter := exporters.NewSSEExporter(&MySSEManager{})
```

### Adding FFmpeg Monitoring for Streams

```go
// Add monitoring when starting a stream
func startStream(streamID, devicePath string) {
    socketPath := fmt.Sprintf("/tmp/ffmpeg_%s.sock", streamID)
    logPath := fmt.Sprintf("/var/log/ffmpeg_%s.log", streamID)
    
    obs.AddFFmpegMonitoring(manager, streamID, socketPath, logPath)
}
```

## Configuration

All configuration is done programmatically in Go code. There are no configuration files.

### Manager Configuration

```go
// Custom manager configuration
config := obs.ManagerConfig{
    StoreConfig: obs.StoreConfig{
        MaxRetentionDuration: 12 * time.Hour,     // How long to keep data
        MaxPointsPerSeries:   43200,              // Max points per time series
        MaxSeries:            5000,               // Max number of time series
        FlushInterval:        30 * time.Second,   // Cleanup interval
    },
    DataChanSize:  5000,            // Internal data channel buffer size
    WorkerCount:   2,               // Number of data processing workers
    FlushInterval: 5 * time.Second, // Exporter flush interval
}
```

### Collector Configuration

```go
// Update collector configuration
collector.UpdateConfig(obs.CollectorConfig{
    Name:       "custom_collector",
    Enabled:    true,
    Interval:   60 * time.Second,
    Labels:     obs.Labels{"environment": "staging"},
    BufferSize: 1000,
    Timeout:    10 * time.Second,
    Retries:    3,
})

// Add/remove collectors dynamically
manager.AddCollector(newCollector)
manager.RemoveCollector("collector_name")
```

### Exporter Configuration

```go
// Prometheus exporter (uses existing registry)
promExporter := exporters.NewPrometheusExporter(registry)

// SSE exporter with custom broadcaster
sseExporter := exporters.NewSSEExporter(broadcaster)

// JSON API exporter on custom port
jsonExporter := exporters.NewJSONExporter(store, ":8093")
```

## Performance Considerations

### Resource Usage

- **Memory**: Ring buffers use fixed memory based on configuration
- **CPU**: Configurable worker count and collection intervals
- **Network**: Efficient batching for external systems

### Recommended Settings

For production VideoNode deployment:

```go
config := obs.ManagerConfig{
    StoreConfig: obs.StoreConfig{
        MaxRetentionDuration: 12 * time.Hour,
        MaxPointsPerSeries:   43200, // 12h at 1 point/sec
        MaxSeries:            5000,
        FlushInterval:        30 * time.Second,
    },
    DataChanSize:  5000,
    WorkerCount:   2,
    FlushInterval: 5 * time.Second,
}
```

### Scaling

- Increase worker count for high-throughput scenarios
- Adjust retention based on available memory
- Use metric filtering to reduce data volume
- Configure appropriate collection intervals

## Error Handling

The package includes comprehensive error handling:

```go
// Check for specific error types
if obsErr, ok := err.(*obs.ObsError); ok {
    switch obsErr.Code {
    case obs.ErrStoreFull:
        // Handle storage limits
    case obs.ErrCollectorNotFound:
        // Handle missing collector
    }
}
```

## Monitoring the Monitor

The OBS package exports its own metrics:

- `obs_data_points_processed_total` - Total data points processed
- `obs_storage_series_count` - Number of active time series
- `obs_collector_errors_total` - Collector error count
- `obs_exporter_flush_duration_seconds` - Export performance

## Best Practices

1. **Start Simple**: Begin with system metrics and basic exporters
2. **Configure Retention**: Set appropriate retention for your use case
3. **Use Labels Wisely**: Avoid high-cardinality labels
4. **Monitor Performance**: Watch resource usage and adjust accordingly
5. **Filter Data**: Use metric and label filters to reduce noise
6. **Test Queries**: Verify query performance with realistic data volumes

## Troubleshooting

### Common Issues

1. **High Memory Usage**
   - Reduce retention duration or max series count
   - Lower collection frequencies
   - Implement stricter filtering

2. **Missing Data**
   - Check collector configurations and errors
   - Verify data channel capacity
   - Review log outputs for errors

3. **Slow Queries**
   - Reduce time ranges
   - Add appropriate filters
   - Consider aggregation options

### Debug Mode

Enable debug logging for detailed operation information:

```go
// Set log level to debug
log.SetLevel(log.DebugLevel)
```

## Future Enhancements

- Distributed tracing support
- Additional storage backends
- Alert rule engine
- Metric aggregation functions
- Dashboard templates
- Auto-discovery mechanisms