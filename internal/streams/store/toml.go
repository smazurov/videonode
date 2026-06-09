package store

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/pelletier/go-toml/v2"

	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/types"
)

// schemaVersion is the persisted on-disk format version. The current format
// is the split top-level [[sources]] / [[composers]] / [[streams]]. The legacy
// monolithic v1 [[streams]] (with inputs/effects/force_composer) is no longer
// supported; its auto-migration has been removed.
const schemaVersion = 2

// config is the persisted v2 layout marshalled to/from TOML.
type config struct {
	Version    int                      `toml:"version" json:"version"`
	Validation *types.ValidationResults `toml:"validation,omitempty" json:"validation,omitempty"`
	Pipeline   *streams.PipelineConfig  `toml:"pipeline,omitempty" json:"pipeline,omitempty"`

	Sources   []V2Source   `toml:"sources,omitempty" json:"sources,omitempty"`
	Composers []V2Composer `toml:"composers,omitempty" json:"composers,omitempty"`
	Streams   []V2Stream   `toml:"streams,omitempty" json:"streams,omitempty"`
}

// tomlStore implements Store using TOML file storage.
type tomlStore struct {
	// mu guards config. The pipeline reads entity specs through the store
	// off the service-layer mutex, so reads and writes must be safe to
	// interleave. One lock, held only for a method body; save() runs under
	// it and never re-acquires.
	mu         sync.RWMutex
	configPath string
	config     *config
	inMemory   bool
}

// NewTOML creates a new TOML-based store.
func NewTOML(configPath string) streams.Store {
	if configPath == "" {
		// Silent fallback hid a real bug: callers that forgot to thread a path
		// got a store pointing at a phantom file in $PWD, never reaching the
		// server's real config. Warn so a future regression is visible in logs.
		// Callers that genuinely need an in-memory store (openapi codegen,
		// tests) should use NewInMemory instead.
		slog.Warn("streams store opened with empty path, defaulting to ./streams.toml; caller should pass an explicit path or use NewInMemory")
		configPath = "streams.toml"
	}

	return &tomlStore{
		configPath: configPath,
		config: &config{
			Version: schemaVersion,
		},
	}
}

// NewInMemory returns a store whose Save is a no-op. Intended for openapi
// codegen and tests that walk the entity surface without touching disk.
func NewInMemory() streams.Store {
	return &tomlStore{
		inMemory: true,
		config: &config{
			Version: schemaVersion,
		},
	}
}

// Load reads the v2 config file. A non-v2 file is rejected with a clear
// error; the v1 → v2 auto-migration has been removed.
func (s *tomlStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to read streams config: %w", err)
	}

	// Peek at the version before binding the full document so a non-v2 file
	// fails with a clear message rather than a confusing partial decode.
	var head struct {
		Version int `toml:"version"`
	}
	if err := toml.Unmarshal(data, &head); err != nil {
		return fmt.Errorf("failed to parse streams config: %w", err)
	}

	if head.Version != schemaVersion {
		return fmt.Errorf("streams.toml version %d unsupported: v1→v2 auto-migration "+
			"was removed; restore a version-2 config", head.Version)
	}

	cfg := &config{}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to parse v2 streams config: %w", err)
	}
	s.config = cfg
	return nil
}

// save marshals config to disk. The caller must hold s.mu (write lock);
// this method never re-acquires it. Unexported: nothing outside the store
// needs to force a flush — every mutator persists in its own locked body.
func (s *tomlStore) save() error {
	if s.inMemory {
		return nil
	}
	dir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Ensure version is always v2 on save.
	if s.config.Version != schemaVersion {
		s.config.Version = schemaVersion
	}

	data, err := toml.Marshal(s.config)
	if err != nil {
		return fmt.Errorf("failed to marshal streams config: %w", err)
	}

	if err := os.WriteFile(s.configPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write streams config: %w", err)
	}

	return nil
}

// GetValidation returns the current validation data.
func (s *tomlStore) GetValidation() *types.ValidationResults {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.Validation
}

// UpdateValidation updates the validation data in the configuration.
func (s *tomlStore) UpdateValidation(validation *types.ValidationResults) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.Validation = validation
	return s.save()
}

// GetPipeline returns the persisted pipeline master switch.
func (s *tomlStore) GetPipeline() streams.PipelineConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config.Pipeline == nil {
		return streams.PipelineConfig{Enabled: true}
	}
	return *s.config.Pipeline
}

// SetPipeline writes the pipeline master switch and persists.
func (s *tomlStore) SetPipeline(cfg streams.PipelineConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := cfg
	s.config.Pipeline = &c
	return s.save()
}

// --- v2 entity accessors (Sources / Composers / Streams).

// GetAllSources returns all v2 sources.
func (s *tomlStore) GetAllSources() []V2Source {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]V2Source, len(s.config.Sources))
	copy(out, s.config.Sources)
	return out
}

// GetSource returns one v2 source by id.
func (s *tomlStore) GetSource(id string) (V2Source, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, src := range s.config.Sources {
		if src.ID == id {
			return src, true
		}
	}
	return V2Source{}, false
}

// AddSource appends a new v2 source and persists.
func (s *tomlStore) AddSource(src V2Source) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.config.Sources {
		if existing.ID == src.ID {
			return fmt.Errorf("source %q already exists", src.ID)
		}
	}
	s.config.Sources = append(s.config.Sources, src)
	return s.save()
}

// UpdateSource replaces an existing source in-place and persists.
func (s *tomlStore) UpdateSource(id string, src V2Source) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.config.Sources {
		if existing.ID == id {
			s.config.Sources[i] = src
			return s.save()
		}
	}
	return fmt.Errorf("source %q not found", id)
}

// RemoveSource deletes a source by id and persists.
func (s *tomlStore) RemoveSource(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.config.Sources {
		if existing.ID == id {
			s.config.Sources = append(s.config.Sources[:i], s.config.Sources[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("source %q not found", id)
}

// GetAllComposers returns all v2 composers.
func (s *tomlStore) GetAllComposers() []V2Composer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]V2Composer, len(s.config.Composers))
	copy(out, s.config.Composers)
	return out
}

// GetComposer returns one v2 composer by id.
func (s *tomlStore) GetComposer(id string) (V2Composer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.config.Composers {
		if c.ID == id {
			return c, true
		}
	}
	return V2Composer{}, false
}

// AddComposer appends a new v2 composer and persists.
func (s *tomlStore) AddComposer(c V2Composer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.config.Composers {
		if existing.ID == c.ID {
			return fmt.Errorf("composer %q already exists", c.ID)
		}
	}
	s.config.Composers = append(s.config.Composers, c)
	return s.save()
}

// UpdateComposer replaces a composer in-place and persists.
func (s *tomlStore) UpdateComposer(id string, c V2Composer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.config.Composers {
		if existing.ID == id {
			s.config.Composers[i] = c
			return s.save()
		}
	}
	return fmt.Errorf("composer %q not found", id)
}

// RemoveComposer deletes a composer by id and persists.
func (s *tomlStore) RemoveComposer(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.config.Composers {
		if existing.ID == id {
			s.config.Composers = append(s.config.Composers[:i], s.config.Composers[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("composer %q not found", id)
}

// GetAllV2Streams returns all v2 streams.
func (s *tomlStore) GetAllV2Streams() []V2Stream {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]V2Stream, len(s.config.Streams))
	copy(out, s.config.Streams)
	return out
}

// GetV2Stream returns one v2 stream by id.
func (s *tomlStore) GetV2Stream(id string) (V2Stream, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, st := range s.config.Streams {
		if st.ID == id {
			return st, true
		}
	}
	return V2Stream{}, false
}

// AddV2Stream appends a new v2 stream and persists.
func (s *tomlStore) AddV2Stream(st V2Stream) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.config.Streams {
		if existing.ID == st.ID {
			return fmt.Errorf("stream %q already exists", st.ID)
		}
	}
	s.config.Streams = append(s.config.Streams, st)
	return s.save()
}

// UpdateV2Stream replaces a stream in-place and persists.
func (s *tomlStore) UpdateV2Stream(id string, st V2Stream) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.config.Streams {
		if existing.ID == id {
			s.config.Streams[i] = st
			return s.save()
		}
	}
	return fmt.Errorf("stream %q not found", id)
}

// RemoveV2Stream deletes a stream by id and persists.
func (s *tomlStore) RemoveV2Stream(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.config.Streams {
		if existing.ID == id {
			s.config.Streams = append(s.config.Streams[:i], s.config.Streams[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("stream %q not found", id)
}

// --- EntityStore implementation (B9 service split).
//
// These adapt persisted V2* types into the canonical pipeline.Source /
// pipeline.Composer / pipeline.Stream shapes the new services consume.

// ListSourceEntities returns all sources as pipeline.Source values.
func (s *tomlStore) ListSourceEntities() []streams.Source {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]streams.Source, len(s.config.Sources))
	for i, v := range s.config.Sources {
		out[i] = sourceFromV2(v)
	}
	return out
}

// GetSourceEntity returns a single source by id.
func (s *tomlStore) GetSourceEntity(id string) (streams.Source, bool) {
	v, ok := s.GetSource(id)
	if !ok {
		return streams.Source{}, false
	}
	return sourceFromV2(v), true
}

// AddSourceEntity persists a new source from its canonical shape.
func (s *tomlStore) AddSourceEntity(src streams.Source) error {
	return s.AddSource(sourceToV2(src))
}

// UpdateSourceEntity replaces an existing source in-place by id.
func (s *tomlStore) UpdateSourceEntity(id string, src streams.Source) error {
	return s.UpdateSource(id, sourceToV2(src))
}

// RemoveSourceEntity deletes a source by id.
func (s *tomlStore) RemoveSourceEntity(id string) error {
	return s.RemoveSource(id)
}

// ListComposerEntities returns all composers as pipeline.Composer values.
func (s *tomlStore) ListComposerEntities() []streams.Composer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]streams.Composer, len(s.config.Composers))
	for i, v := range s.config.Composers {
		out[i] = composerFromV2(v)
	}
	return out
}

// GetComposerEntity returns a single composer by id.
func (s *tomlStore) GetComposerEntity(id string) (streams.Composer, bool) {
	v, ok := s.GetComposer(id)
	if !ok {
		return streams.Composer{}, false
	}
	return composerFromV2(v), true
}

// AddComposerEntity persists a new composer from its canonical shape.
func (s *tomlStore) AddComposerEntity(c streams.Composer) error {
	return s.AddComposer(composerToV2(c))
}

// UpdateComposerEntity replaces an existing composer in-place.
func (s *tomlStore) UpdateComposerEntity(id string, c streams.Composer) error {
	return s.UpdateComposer(id, composerToV2(c))
}

// RemoveComposerEntity deletes a composer by id.
func (s *tomlStore) RemoveComposerEntity(id string) error {
	return s.RemoveComposer(id)
}

// ListPipelineStreams returns all v2 streams as pipeline.Stream values.
func (s *tomlStore) ListPipelineStreams() []streams.PipelineStream {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]streams.PipelineStream, len(s.config.Streams))
	for i, v := range s.config.Streams {
		out[i] = pipelineStreamFromV2(v)
	}
	return out
}

// GetPipelineStream returns one v2 stream as pipeline.Stream.
func (s *tomlStore) GetPipelineStream(id string) (streams.PipelineStream, bool) {
	v, ok := s.GetV2Stream(id)
	if !ok {
		return streams.PipelineStream{}, false
	}
	return pipelineStreamFromV2(v), true
}

// AddPipelineStream persists a new v2 stream from its canonical shape.
func (s *tomlStore) AddPipelineStream(st streams.PipelineStream) error {
	return s.AddV2Stream(pipelineStreamToV2(st))
}

// UpdatePipelineStream replaces an existing v2 stream in-place.
func (s *tomlStore) UpdatePipelineStream(id string, st streams.PipelineStream) error {
	return s.UpdateV2Stream(id, pipelineStreamToV2(st))
}

// RemovePipelineStream deletes a v2 stream by id.
func (s *tomlStore) RemovePipelineStream(id string) error {
	return s.RemoveV2Stream(id)
}
