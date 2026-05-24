package store

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/types"
)

// SchemaVersion is the persisted on-disk format version. V1 was the
// legacy monolithic [[streams]] with inputs/effects/force_composer; v2
// is the split top-level [[sources]] / [[composers]] / [[streams]].
const schemaVersion = 2

// config is the persisted v2 layout marshalled to/from TOML. Legacy v1
// fields (Streams map + intermediate [[streams]] with inputs) are kept on
// load through a parallel rawV1 decode, then converted in-memory.
type config struct {
	Version    int                      `toml:"version" json:"version"`
	Validation *types.ValidationResults `toml:"validation,omitempty" json:"validation,omitempty"`
	Pipeline   *streams.PipelineConfig  `toml:"pipeline,omitempty" json:"pipeline,omitempty"`

	Sources   []V2Source   `toml:"sources,omitempty" json:"sources,omitempty"`
	Composers []V2Composer `toml:"composers,omitempty" json:"composers,omitempty"`
	Streams   []V2Stream   `toml:"streams,omitempty" json:"streams,omitempty"`

	// LegacyStreams is the v1 map-shape [streams.<id>] table. Populated only
	// when an old config is read; emptied after migration. Kept as a typed
	// field so legacy fixtures still round-trip through the marshaler when
	// a downstream caller adds via the deprecated AddStream path.
	LegacyStreams map[string]streams.StreamSpec `toml:"-" json:"-"`
}

// streamsRawV1Entry is just enough of the legacy StreamSpec to seed a
// migration. We only need device + test_mode + minimal encoder shape.
type streamsRawV1Entry struct {
	ID                  string               `toml:"id"`
	Name                string               `toml:"name"`
	Device              string               `toml:"device"`
	TestMode            bool                 `toml:"test_mode"`
	FFmpeg              v1LegacyFFmpeg       `toml:"ffmpeg"`
	Canvas              *v1LegacyCanvas      `toml:"canvas"`
	CustomFFmpegCommand string               `toml:"custom_ffmpeg_command"`
	Perspective         *v1LegacyPerspective `toml:"perspective"`
	CreatedAt           string               `toml:"created_at"`
	UpdatedAt           string               `toml:"updated_at"`
}

type v1LegacyFFmpeg struct {
	Codec       string   `toml:"codec"`
	InputFormat string   `toml:"input_format"`
	Resolution  string   `toml:"resolution"`
	FPS         string   `toml:"fps"`
	AudioDevice string   `toml:"audio_device"`
	Options     []string `toml:"options"`
}

type v1LegacyCanvas struct {
	Width         int      `toml:"width"`
	Height        int      `toml:"height"`
	FPS           string   `toml:"fps"`
	SourceStreams []string `toml:"source_streams"`
	AudioDevices  []string `toml:"audio_devices"`
}

type v1LegacyPerspective struct {
	Corners [4][2]int `toml:"corners"`
}

// tomlStore implements Store using TOML file storage with auto-migration
// from v1 (legacy + intermediate) shapes to v2.
type tomlStore struct {
	configPath string
	config     *config
}

// NewTOML creates a new TOML-based store.
func NewTOML(configPath string) streams.Store {
	if configPath == "" {
		// Silent fallback hid a real bug: callers that forgot to thread a path
		// got a store pointing at a phantom file in $PWD, never reaching the
		// server's real config. Warn so a future regression is visible in logs.
		slog.Warn("streams store opened with empty path, defaulting to ./streams.toml; caller should pass an explicit path")
		configPath = "streams.toml"
	}

	return &tomlStore{
		configPath: configPath,
		config: &config{
			Version:       schemaVersion,
			LegacyStreams: make(map[string]streams.StreamSpec),
		},
	}
}

// Load reads the config file, auto-migrating v1 → v2 in place. After a
// successful migration the rewritten v2 TOML is written back so subsequent
// loads are pure v2 reads.
func (s *tomlStore) Load() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to read streams config: %w", err)
	}

	// Peek at version + validation/pipeline first; never bind `streams` here
	// since its shape varies across v1 forms (array vs table).
	var head struct {
		Version    int                      `toml:"version"`
		Validation *types.ValidationResults `toml:"validation"`
		Pipeline   *streams.PipelineConfig  `toml:"pipeline"`
	}
	if err := toml.Unmarshal(data, &head); err != nil {
		return fmt.Errorf("failed to parse streams config: %w", err)
	}

	if head.Version == schemaVersion {
		// Pure v2 decode.
		cfg := &config{LegacyStreams: make(map[string]streams.StreamSpec)}
		if err := toml.Unmarshal(data, cfg); err != nil {
			return fmt.Errorf("failed to parse v2 streams config: %w", err)
		}
		s.config = cfg
		return nil
	}

	// Dispatch between intermediate ([[streams]] array) and legacy
	// ([streams.<id>] table) forms by trying each in turn.
	v1Streams, err := decodeV1Streams(data)
	if err != nil {
		return fmt.Errorf("failed to parse v1 streams config: %w", err)
	}

	mr, err := migrateV1Streams(v1Streams)
	if err != nil {
		return fmt.Errorf("v1→v2 migration failed: %w", err)
	}
	s.config = &config{
		Version:       schemaVersion,
		Validation:    head.Validation,
		Pipeline:      head.Pipeline,
		Sources:       mr.Sources,
		Composers:     mr.Composers,
		Streams:       mr.Streams,
		LegacyStreams: make(map[string]streams.StreamSpec),
	}

	if err := s.Save(); err != nil {
		return fmt.Errorf("failed to persist migrated config: %w", err)
	}
	slog.Info("streams.toml migrated to v2",
		"sources", len(s.config.Sources),
		"composers", len(s.config.Composers),
		"streams", len(s.config.Streams),
	)
	return nil
}

// decodeV1Streams tries the intermediate [[streams]] array shape first
// (canonical v1) and falls back to the legacy [streams.<id>] table shape
// if that yields zero streams.
func decodeV1Streams(data []byte) ([]v1RawStream, error) {
	var asArray struct {
		Streams []v1RawStream `toml:"streams"`
	}
	if err := toml.Unmarshal(data, &asArray); err == nil && len(asArray.Streams) > 0 {
		return asArray.Streams, nil
	}

	var asTable struct {
		Streams map[string]streamsRawV1Entry `toml:"streams"`
	}
	if err := toml.Unmarshal(data, &asTable); err != nil {
		return nil, err
	}
	return convertLegacyTableToIntermediate(asTable.Streams), nil
}

// convertLegacyTableToIntermediate maps the [streams.<id>] StreamSpec shape
// into the intermediate v1 [[streams]] form so a single migrateV1Streams
// path handles both. Canvas streams become multi-input intermediate streams.
func convertLegacyTableToIntermediate(table map[string]streamsRawV1Entry) []v1RawStream {
	out := make([]v1RawStream, 0, len(table))
	for id, e := range table {
		if e.ID == "" {
			e.ID = id
		}
		var inputs []v1RawInput
		var layout []v1RawSlot
		if e.Canvas != nil && len(e.Canvas.SourceStreams) > 0 {
			for i, src := range e.Canvas.SourceStreams {
				inputs = append(inputs, v1RawInput{
					ID:     fmt.Sprintf("inp%d", i+1),
					Device: src,
				})
			}
			cw := e.Canvas.Width
			if cw == 0 {
				cw = 1920
			}
			ch := e.Canvas.Height
			if ch == 0 {
				ch = 1080
			}
			layout = append(layout, v1RawSlot{Slot: 0, X: 0, Y: 0, W: cw, H: ch})
		} else {
			inputs = append(inputs, v1RawInput{ID: "inp1", Device: e.Device})
		}

		effects := map[string][]v1RawFx{}
		if e.Perspective != nil {
			effects[inputs[0].ID] = []v1RawFx{{Type: "perspective", Corners: e.Perspective.Corners}}
		}

		out = append(out, v1RawStream{
			ID:       e.ID,
			Name:     e.Name,
			Inputs:   inputs,
			Layout:   layout,
			Effects:  effects,
			TestMode: e.TestMode,
			Audio: V2AudioConfig{
				Devices: legacyAudioDevices(e),
			},
			Encoder: V2EncoderConfig{
				Codec: e.FFmpeg.Codec,
			},
		})
	}
	return out
}

func legacyAudioDevices(e streamsRawV1Entry) []string {
	if e.FFmpeg.AudioDevice != "" {
		return []string{e.FFmpeg.AudioDevice}
	}
	if e.Canvas != nil && len(e.Canvas.AudioDevices) > 0 {
		return e.Canvas.AudioDevices
	}
	return nil
}

// Save writes the in-memory config to disk as v2 TOML.
func (s *tomlStore) Save() error {
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

// --- Legacy StreamSpec methods (kept for B9-pending consumers). After
// migration these operate on an empty in-memory map; B9 rewires callers
// to the new entity accessors below.

// AddStream stores a legacy StreamSpec in the in-memory map only. Persistence
// goes through Save which emits the v2 shape; legacy specs added here are
// not round-tripped to disk.
func (s *tomlStore) AddStream(stream streams.StreamSpec) error {
	if s.config.LegacyStreams == nil {
		s.config.LegacyStreams = make(map[string]streams.StreamSpec)
	}
	s.config.LegacyStreams[stream.ID] = stream
	return s.Save()
}

// UpdateStream updates a legacy StreamSpec in the in-memory map.
func (s *tomlStore) UpdateStream(id string, updates streams.StreamSpec) error {
	if s.config.LegacyStreams == nil {
		s.config.LegacyStreams = make(map[string]streams.StreamSpec)
	}
	s.config.LegacyStreams[id] = updates
	return s.Save()
}

// RemoveStream removes a legacy StreamSpec from the in-memory map.
func (s *tomlStore) RemoveStream(id string) error {
	delete(s.config.LegacyStreams, id)
	return s.Save()
}

// GetStream returns a legacy StreamSpec by ID.
func (s *tomlStore) GetStream(id string) (streams.StreamSpec, bool) {
	stream, exists := s.config.LegacyStreams[id]
	return stream, exists
}

// GetAllStreams returns all legacy StreamSpecs (empty after migration).
func (s *tomlStore) GetAllStreams() map[string]streams.StreamSpec {
	if s.config.LegacyStreams == nil {
		return map[string]streams.StreamSpec{}
	}
	return s.config.LegacyStreams
}

// GetValidation returns the current validation data.
func (s *tomlStore) GetValidation() *types.ValidationResults {
	return s.config.Validation
}

// UpdateValidation updates the validation data in the configuration.
func (s *tomlStore) UpdateValidation(validation *types.ValidationResults) error {
	s.config.Validation = validation
	return s.Save()
}

// GetPipeline returns the persisted pipeline master switch.
func (s *tomlStore) GetPipeline() streams.PipelineConfig {
	if s.config.Pipeline == nil {
		return streams.PipelineConfig{Enabled: true}
	}
	return *s.config.Pipeline
}

// SetPipeline writes the pipeline master switch and persists.
func (s *tomlStore) SetPipeline(cfg streams.PipelineConfig) error {
	c := cfg
	s.config.Pipeline = &c
	return s.Save()
}

// --- v2 entity accessors (Sources / Composers / Streams).

// GetAllSources returns all v2 sources.
func (s *tomlStore) GetAllSources() []V2Source {
	out := make([]V2Source, len(s.config.Sources))
	copy(out, s.config.Sources)
	return out
}

// GetSource returns one v2 source by id.
func (s *tomlStore) GetSource(id string) (V2Source, bool) {
	for _, src := range s.config.Sources {
		if src.ID == id {
			return src, true
		}
	}
	return V2Source{}, false
}

// AddSource appends a new v2 source and persists.
func (s *tomlStore) AddSource(src V2Source) error {
	for _, existing := range s.config.Sources {
		if existing.ID == src.ID {
			return fmt.Errorf("source %q already exists", src.ID)
		}
	}
	s.config.Sources = append(s.config.Sources, src)
	return s.Save()
}

// UpdateSource replaces an existing source in-place and persists.
func (s *tomlStore) UpdateSource(id string, src V2Source) error {
	for i, existing := range s.config.Sources {
		if existing.ID == id {
			s.config.Sources[i] = src
			return s.Save()
		}
	}
	return fmt.Errorf("source %q not found", id)
}

// RemoveSource deletes a source by id and persists.
func (s *tomlStore) RemoveSource(id string) error {
	for i, existing := range s.config.Sources {
		if existing.ID == id {
			s.config.Sources = append(s.config.Sources[:i], s.config.Sources[i+1:]...)
			return s.Save()
		}
	}
	return fmt.Errorf("source %q not found", id)
}

// GetAllComposers returns all v2 composers.
func (s *tomlStore) GetAllComposers() []V2Composer {
	out := make([]V2Composer, len(s.config.Composers))
	copy(out, s.config.Composers)
	return out
}

// GetComposer returns one v2 composer by id.
func (s *tomlStore) GetComposer(id string) (V2Composer, bool) {
	for _, c := range s.config.Composers {
		if c.ID == id {
			return c, true
		}
	}
	return V2Composer{}, false
}

// AddComposer appends a new v2 composer and persists.
func (s *tomlStore) AddComposer(c V2Composer) error {
	for _, existing := range s.config.Composers {
		if existing.ID == c.ID {
			return fmt.Errorf("composer %q already exists", c.ID)
		}
	}
	s.config.Composers = append(s.config.Composers, c)
	return s.Save()
}

// UpdateComposer replaces a composer in-place and persists.
func (s *tomlStore) UpdateComposer(id string, c V2Composer) error {
	for i, existing := range s.config.Composers {
		if existing.ID == id {
			s.config.Composers[i] = c
			return s.Save()
		}
	}
	return fmt.Errorf("composer %q not found", id)
}

// RemoveComposer deletes a composer by id and persists.
func (s *tomlStore) RemoveComposer(id string) error {
	for i, existing := range s.config.Composers {
		if existing.ID == id {
			s.config.Composers = append(s.config.Composers[:i], s.config.Composers[i+1:]...)
			return s.Save()
		}
	}
	return fmt.Errorf("composer %q not found", id)
}

// GetAllV2Streams returns all v2 streams.
func (s *tomlStore) GetAllV2Streams() []V2Stream {
	out := make([]V2Stream, len(s.config.Streams))
	copy(out, s.config.Streams)
	return out
}

// GetV2Stream returns one v2 stream by id.
func (s *tomlStore) GetV2Stream(id string) (V2Stream, bool) {
	for _, st := range s.config.Streams {
		if st.ID == id {
			return st, true
		}
	}
	return V2Stream{}, false
}

// AddV2Stream appends a new v2 stream and persists.
func (s *tomlStore) AddV2Stream(st V2Stream) error {
	for _, existing := range s.config.Streams {
		if existing.ID == st.ID {
			return fmt.Errorf("stream %q already exists", st.ID)
		}
	}
	s.config.Streams = append(s.config.Streams, st)
	return s.Save()
}

// UpdateV2Stream replaces a stream in-place and persists.
func (s *tomlStore) UpdateV2Stream(id string, st V2Stream) error {
	for i, existing := range s.config.Streams {
		if existing.ID == id {
			s.config.Streams[i] = st
			return s.Save()
		}
	}
	return fmt.Errorf("stream %q not found", id)
}

// RemoveV2Stream deletes a stream by id and persists.
func (s *tomlStore) RemoveV2Stream(id string) error {
	for i, existing := range s.config.Streams {
		if existing.ID == id {
			s.config.Streams = append(s.config.Streams[:i], s.config.Streams[i+1:]...)
			return s.Save()
		}
	}
	return fmt.Errorf("stream %q not found", id)
}
