// Package services provides concrete implementations of the
// api.SourceService and api.ComposerService interfaces (B9 service-layer
// split). The implementations live in a sibling package of
// internal/streams to avoid an import cycle: internal/api depends on
// internal/streams for the legacy StreamService; the new services
// reverse the dependency direction (services → api error types +
// streams.EntityStore + pipeline.Pipeline).
package services
