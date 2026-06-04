package services

import (
	"context"
	"regexp"
	"time"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/smazurov/videonode/internal/api"
	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// composerImportIDPattern mirrors the create-request id constraint
// (^[a-zA-Z0-9_-]+$, 1..64 chars) for the import path, which bypasses
// huma's request-body validation by accepting raw TOML.
var composerImportIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// ExportComposer marshals a stored composer to a standalone TOML document.
func (s *composerService) ExportComposer(_ context.Context, id string) ([]byte, error) {
	c, ok := s.store.GetComposerEntity(id)
	if !ok {
		return nil, &api.ComposerError{Code: api.ComposerErrNotFound, Message: "composer " + id + " not found"}
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return nil, &api.ComposerError{Code: api.ComposerErrInternal, Message: err.Error()}
	}
	return data, nil
}

// ImportComposer parses a TOML composer document and upserts it by the id in
// the document: a new id is created, an existing id is overwritten in place.
// Returns created=true when the composer did not previously exist.
func (s *composerService) ImportComposer(_ context.Context, data []byte) (*models.ComposerData, bool, error) {
	return s.importTOML("", data)
}

// ImportComposerInto overwrites an existing composer with a TOML document,
// forcing the document's id to the target id so any composer's exported
// config can be pasted onto another. The target must already exist (404
// otherwise); its original CreatedAt is preserved.
func (s *composerService) ImportComposerInto(_ context.Context, id string, data []byte) (*models.ComposerData, error) {
	out, _, err := s.importTOML(id, data)
	return out, err
}

// importTOML is the shared decode → validate → upsert → mirror path. An empty
// targetID upserts by the document's own id; a non-empty targetID forces that
// id onto the document and requires the composer to already exist. The store
// write is the source of truth; when the switch is on the pipeline re-applies
// it (restart), and when off there's nothing to do — the pipeline reads the
// new spec through the store on its next spawn.
func (s *composerService) importTOML(targetID string, data []byte) (*models.ComposerData, bool, error) {
	var c pipeline.Composer
	if err := toml.Unmarshal(data, &c); err != nil {
		return nil, false, &api.ComposerError{Code: api.ComposerErrInvalid, Message: "invalid composer TOML: " + err.Error()}
	}
	if targetID != "" {
		c.ID = targetID
	}
	if err := validateImportedComposer(c); err != nil {
		return nil, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	prev, existed := s.store.GetComposerEntity(c.ID)
	if targetID != "" && !existed {
		return nil, false, &api.ComposerError{Code: api.ComposerErrNotFound, Message: "composer " + targetID + " not found"}
	}

	now := time.Now()
	c.UpdatedAt = now
	if existed {
		c.CreatedAt = prev.CreatedAt
		if err := s.store.UpdateComposerEntity(c.ID, c); err != nil {
			return nil, false, &api.ComposerError{Code: api.ComposerErrInternal, Message: err.Error()}
		}
	} else {
		c.CreatedAt = now
		if err := s.store.AddComposerEntity(c); err != nil {
			return nil, false, &api.ComposerError{Code: api.ComposerErrInternal, Message: err.Error()}
		}
	}

	if err := s.applyImportedComposer(prev, c, existed); err != nil {
		return nil, false, err
	}
	out := s.toEnrichedAPI(c)
	return &out, !existed, nil
}

// applyImportedComposer mirrors an imported composer onto the pipeline.
// Switch off → nothing to do (the spec is persisted; the pipeline reads it
// through the store on its next spawn). Switch on → full re-apply (restart),
// rolling the store back to the prior state on rejection so a bad import is
// not persisted; a canvas-dimension change also bounces dependent encoders so
// their launch-time ffmpeg `-s` tracks the new broadcast size.
func (s *composerService) applyImportedComposer(prev, c pipeline.Composer, existed bool) error {
	if s.pipe == nil {
		return nil
	}
	if !s.pipelineSwitchEnabled() {
		// Switch off: the imported spec is already persisted; the pipeline
		// reads through to the store, so there's nothing to spawn or cache.
		return nil
	}
	if err := s.pipe.ApplyComposer(c); err != nil {
		s.rollbackImport(prev, c.ID, existed)
		return &api.ComposerError{Code: api.ComposerErrInvalid, Message: "pipeline rejected composer: " + err.Error()}
	}
	if existed && prev.Canvas != c.Canvas {
		s.rebuildDependentEncoders(c.ID)
	}
	return nil
}

// rollbackImport undoes the store write after a failed pipeline apply: drop a
// freshly created composer, or restore the prior config of an overwritten one.
func (s *composerService) rollbackImport(prev pipeline.Composer, id string, existed bool) {
	var err error
	if existed {
		err = s.store.UpdateComposerEntity(id, prev)
	} else {
		err = s.store.RemoveComposerEntity(id)
	}
	if err != nil {
		s.logger.Error("ImportComposer: rollback after pipeline failure also failed",
			logging.KeyComposerID, id, logging.KeyRollbackError, err)
	}
}

func validateImportedComposer(c pipeline.Composer) error {
	if !composerImportIDPattern.MatchString(c.ID) {
		return &api.ComposerError{Code: api.ComposerErrInvalid, Message: "composer id must match ^[a-zA-Z0-9_-]+$ and be 1-64 characters"}
	}
	if c.Canvas.W <= 0 || c.Canvas.H <= 0 {
		return &api.ComposerError{Code: api.ComposerErrInvalid, Message: "canvas dimensions must be positive"}
	}
	if c.Canvas.FPS < 0 || c.Canvas.FPS > 240 {
		return &api.ComposerError{Code: api.ComposerErrInvalid, Message: "canvas fps must be in 0..240 (0 = daemon default)"}
	}
	if len(c.Inputs) == 0 {
		return &api.ComposerError{Code: api.ComposerErrInvalid, Message: "at least one input is required"}
	}
	return validateComposerLayout(c)
}
