package api

import (
	"sync"

	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/streaming"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
	"github.com/smazurov/videonode/internal/streams/sensors"
)

var registerEntityVariantsOnce sync.Once

// registerEntityVariants declares every EntityEvent arm so the SSE schema
// (built by events.EntityEvent.Schema during registerSSERoutes, including the
// `openapi` CLI) is a complete discriminated union. Idempotent — safe across
// multiple Server constructions in one process. Called before sse.Register.
// Lives here because this is the lowest package that can import all payload
// types without a cycle.
func registerEntityVariants() {
	registerEntityVariantsOnce.Do(func() {
		events.RegisterVariant[models.SourceData]("source", "created")
		events.RegisterVariant[models.SourceData]("source", "updated")
		events.RegisterDeleteVariant("source")
		events.RegisterVariant[pipelinectl.StatusParams]("source", "status")
		events.RegisterVariant[pipelinectl.SourceConsumersInfo]("source", "consumers")

		events.RegisterVariant[models.ComposerData]("composer", "created")
		events.RegisterVariant[models.ComposerData]("composer", "updated")
		events.RegisterDeleteVariant("composer")

		events.RegisterVariant[models.StreamData]("stream", "created")
		events.RegisterVariant[models.StreamData]("stream", "updated")
		events.RegisterDeleteVariant("stream")
		events.RegisterVariant[streaming.StreamStatusPayload]("stream", "status")
		events.RegisterVariant[streaming.StreamMetricsPayload]("stream", "metrics")
		events.RegisterVariant[streaming.StreamConsumersPayload]("stream", "consumers")

		events.RegisterVariant[models.SensorData]("sensor", "created")
		events.RegisterVariant[models.SensorData]("sensor", "updated")
		events.RegisterDeleteVariant("sensor")
		events.RegisterVariant[sensors.FindingEvent]("sensor", "status")
	})
}
