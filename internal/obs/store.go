package obs

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Series represents a time series with a ring buffer for efficient storage
type Series struct {
	name      string
	dataType  DataType
	labels    Labels
	points    []DataPoint
	head      int // Current write position
	size      int // Current number of points
	capacity  int // Maximum capacity
	mutex     sync.RWMutex
	firstSeen time.Time
	lastSeen  time.Time
}

// NewSeries creates a new time series with the specified capacity
func NewSeries(name string, dataType DataType, labels Labels, capacity int) *Series {
	return &Series{
		name:     name,
		dataType: dataType,
		labels:   copyLabels(labels),
		points:   make([]DataPoint, capacity),
		head:     0,
		size:     0,
		capacity: capacity,
	}
}

// Add adds a data point to the series
func (s *Series) Add(point DataPoint) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	timestamp := point.Timestamp()

	// Update first/last seen
	if s.size == 0 {
		s.firstSeen = timestamp
	}
	s.lastSeen = timestamp

	// Add point to ring buffer
	s.points[s.head] = point
	s.head = (s.head + 1) % s.capacity

	if s.size < s.capacity {
		s.size++
	}
}

// Query returns points within the specified time range
func (s *Series) Query(start, end time.Time, limit int) []DataPoint {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if s.size == 0 {
		return nil
	}

	var result []DataPoint
	visited := 0

	// Calculate starting position (oldest point)
	var startPos int
	if s.size == s.capacity {
		startPos = s.head // In a full buffer, head points to oldest
	} else {
		startPos = 0 // In a partial buffer, start from beginning
	}

	// Collect points within time range
	for i := 0; i < s.size && (limit == 0 || len(result) < limit); i++ {
		pos := (startPos + i) % s.capacity
		point := s.points[pos]

		if point == nil {
			continue
		}

		timestamp := point.Timestamp()
		if (start.IsZero() || timestamp.After(start) || timestamp.Equal(start)) &&
			(end.IsZero() || timestamp.Before(end) || timestamp.Equal(end)) {
			result = append(result, point)
		}
		visited++
	}

	// Sort by timestamp (oldest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp().Before(result[j].Timestamp())
	})

	return result
}

// Info returns metadata about the series
func (s *Series) Info() SeriesInfo {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return SeriesInfo{
		Name:       s.name,
		DataType:   s.dataType,
		Labels:     copyLabels(s.labels),
		FirstSeen:  s.firstSeen,
		LastSeen:   s.lastSeen,
		PointCount: int64(s.size),
	}
}

// Size returns the current number of points in the series
func (s *Series) Size() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.size
}

// Store manages multiple time series with efficient storage and querying
type Store struct {
	config       StoreConfig
	series       map[string]*Series   // key: seriesKey(name, labels)
	seriesByName map[string][]*Series // index by name for faster lookups
	mutex        sync.RWMutex
	lastCleanup  time.Time
}

// NewStore creates a new in-memory store
func NewStore(config StoreConfig) *Store {
	return &Store{
		config:       config,
		series:       make(map[string]*Series),
		seriesByName: make(map[string][]*Series),
		lastCleanup:  time.Now(),
	}
}

// Add adds a data point to the store
func (s *Store) Add(point DataPoint) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check if we need cleanup
	if time.Since(s.lastCleanup) > s.config.FlushInterval {
		s.cleanupExpiredData()
		s.lastCleanup = time.Now()
	}

	// Generate series key
	name := s.getPointName(point)
	labels := point.Labels()
	seriesKey := s.seriesKey(name, labels)

	// Get or create series
	series, exists := s.series[seriesKey]
	if !exists {
		// Check if we're at max series limit
		if len(s.series) >= s.config.MaxSeries {
			return NewObsError(ErrStoreFull, "maximum number of series reached", map[string]interface{}{
				"max_series":     s.config.MaxSeries,
				"current_series": len(s.series),
				"series_key":     seriesKey,
			})
		}

		series = NewSeries(name, point.Type(), labels, s.config.MaxPointsPerSeries)
		s.series[seriesKey] = series

		// Add to name index
		if s.seriesByName[name] == nil {
			s.seriesByName[name] = []*Series{}
		}
		s.seriesByName[name] = append(s.seriesByName[name], series)
	}

	series.Add(point)
	return nil
}

// Query queries the store for data points matching the given criteria
func (s *Store) Query(opts QueryOptions) (*QueryResult, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var allPoints []DataPoint
	var matchingSeries []*Series

	// Find matching series
	if opts.Name != "" {
		// Query by name
		if seriesList, exists := s.seriesByName[opts.Name]; exists {
			for _, series := range seriesList {
				if s.labelsMatch(series.labels, opts.Labels) {
					matchingSeries = append(matchingSeries, series)
				}
			}
		}
	} else {
		// Query all series of the specified type
		for _, series := range s.series {
			if series.dataType == opts.DataType && s.labelsMatch(series.labels, opts.Labels) {
				matchingSeries = append(matchingSeries, series)
			}
		}
	}

	// Collect points from matching series
	for _, series := range matchingSeries {
		points := series.Query(opts.Start, opts.End, 0) // No limit per series
		allPoints = append(allPoints, points...)
	}

	// Sort by timestamp
	sort.Slice(allPoints, func(i, j int) bool {
		return allPoints[i].Timestamp().Before(allPoints[j].Timestamp())
	})

	// Apply limit
	total := len(allPoints)
	truncated := false
	if opts.Limit > 0 && len(allPoints) > opts.Limit {
		allPoints = allPoints[:opts.Limit]
		truncated = true
	}

	return &QueryResult{
		DataType:  opts.DataType,
		Name:      opts.Name,
		Labels:    opts.Labels,
		Points:    allPoints,
		Total:     total,
		Truncated: truncated,
	}, nil
}

// ListSeries returns information about all series in the store
func (s *Store) ListSeries() []SeriesInfo {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var result []SeriesInfo
	for _, series := range s.series {
		result = append(result, series.Info())
	}

	// Sort by name for consistent output
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// Stats returns statistics about the store
func (s *Store) Stats() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	totalPoints := 0
	for _, series := range s.series {
		totalPoints += series.Size()
	}

	return map[string]interface{}{
		"total_series":           len(s.series),
		"total_points":           totalPoints,
		"max_series":             s.config.MaxSeries,
		"max_points_per_series":  s.config.MaxPointsPerSeries,
		"max_retention_duration": s.config.MaxRetentionDuration.String(),
		"last_cleanup":           s.lastCleanup.Format(time.RFC3339),
	}
}

// cleanupExpiredData removes old data points based on retention policy
func (s *Store) cleanupExpiredData() {
	cutoff := time.Now().Add(-s.config.MaxRetentionDuration)

	for seriesKey, series := range s.series {
		// Check if series has any recent data
		if series.lastSeen.Before(cutoff) {
			// Remove entire series if it's too old
			delete(s.series, seriesKey)

			// Remove from name index
			if seriesList, exists := s.seriesByName[series.name]; exists {
				filtered := make([]*Series, 0, len(seriesList))
				for _, s := range seriesList {
					if s != series {
						filtered = append(filtered, s)
					}
				}
				if len(filtered) == 0 {
					delete(s.seriesByName, series.name)
				} else {
					s.seriesByName[series.name] = filtered
				}
			}
		}
	}
}

// Helper functions

func (s *Store) getPointName(point DataPoint) string {
	switch p := point.(type) {
	case *MetricPoint:
		return p.Name
	case *LogEntry:
		if p.Source != "" {
			return p.Source
		}
		return "logs"
	case *SpanEntry:
		return p.Operation
	default:
		return "unknown"
	}
}

func (s *Store) seriesKey(name string, labels Labels) string {
	// Create a deterministic key from name and labels
	key := name
	if len(labels) > 0 {
		var labelPairs []string
		for k, v := range labels {
			labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", k, v))
		}
		sort.Strings(labelPairs)
		key += "{" + fmt.Sprintf("%v", labelPairs) + "}"
	}
	return key
}

func (s *Store) labelsMatch(seriesLabels, queryLabels Labels) bool {
	if len(queryLabels) == 0 {
		return true // No filter means match all
	}

	for k, v := range queryLabels {
		if seriesValue, exists := seriesLabels[k]; !exists || seriesValue != v {
			return false
		}
	}
	return true
}

func copyLabels(labels Labels) Labels {
	if labels == nil {
		return make(Labels)
	}

	result := make(Labels, len(labels))
	for k, v := range labels {
		result[k] = v
	}
	return result
}
