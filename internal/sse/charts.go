package sse

import (
	"log"
	"time"
)

// sendInitialChartConfigs sends chart configurations to newly connected clients
func (m *Manager) sendInitialChartConfigs() {
	// Add a small delay to ensure the client connection is fully established
	time.Sleep(100 * time.Millisecond)

	// Get chart configurations from the provided function
	if m.getChartConfigs != nil {
		configs := m.getChartConfigs()
		for _, config := range configs {
			if err := m.BroadcastCustomEvent("chart-config", config); err != nil {
				log.Printf("SSE: Failed to send chart config: %v", err)
			}
		}
	} else {
		// Send default chart configurations if no function provided
		m.sendDefaultChartConfigs()
	}
}

// sendDefaultChartConfigs sends default chart configurations
func (m *Manager) sendDefaultChartConfigs() {
	// WiFi chart config
	wifiConfig := map[string]interface{}{
		"id":         "wifi-chart",
		"type":       "line",
		"title":      "WiFi Signal Quality",
		"yAxisLabel": "Signal / Quality",
		"yAxisStart": "auto",
		"maxPoints":  60,
		"datasets": []map[string]interface{}{
			{
				"label":           "Signal Strength (dBm)",
				"borderColor":     "#F59E0B",
				"backgroundColor": "#F59E0B20",
			},
			{
				"label":           "Quality (%)",
				"borderColor":     "#8B5CF6",
				"backgroundColor": "#8B5CF620",
			},
		},
	}

	if err := m.BroadcastCustomEvent("chart-config", wifiConfig); err != nil {
		log.Printf("SSE: Failed to send WiFi chart config: %v", err)
	}

	// Note: Stream chart configs would be sent dynamically based on active streams
	// This would require access to the stream state, which should be provided
	// through the getChartConfigs function
}
