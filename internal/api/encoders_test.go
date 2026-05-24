//go:build !planv2_tests

// Pre-rewrite encoders-validation test — drives the monolithic
// StreamService interface (StreamSpec, etc.) which B9 splits up.
// Excluded from planv2_tests builds; replaced once B9 lands.
package api

import (
	"os"
	"strings"
	"testing"

	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/types"
)

// stubValidationProvider returns canned validation results to verify that
// getValidatedEncoders consults the StreamService-injected provider rather
// than opening a fresh streams.toml from $PWD.
type stubValidationProvider struct {
	results *types.ValidationResults
	updated *types.ValidationResults
}

func (p *stubValidationProvider) GetValidation() *types.ValidationResults {
	return p.results
}

func (p *stubValidationProvider) UpdateValidation(results *types.ValidationResults) error {
	p.updated = results
	return nil
}

func TestGetValidatedEncoders_UsesInjectedProvider_NoFilesystem(t *testing.T) {
	// Run from a temp dir with no streams.toml — guarantees that any attempt
	// to read a $PWD-relative store would fail visibly.
	tmp := t.TempDir()
	t.Chdir(tmp)

	if _, err := os.Stat("streams.toml"); !os.IsNotExist(err) {
		t.Fatalf("preflight: expected no streams.toml in tempdir, got %v", err)
	}

	tests := []struct {
		name           string
		provider       *stubValidationProvider
		wantErrContain string
	}{
		{
			name:           "nil validation results surfaces as not-found",
			provider:       &stubValidationProvider{results: nil},
			wantErrContain: "validation data not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockStreamService{
				streams:            make(map[string]*streams.Stream),
				streamSpecs:        make(map[string]*streams.StreamSpec),
				validationProvider: tt.provider,
			}

			server := &Server{streamService: svc}

			_, err := server.getValidatedEncoders()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrContain) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrContain)
			}
		})
	}
}

func TestGetValidatedEncoders_NilProvider(t *testing.T) {
	svc := &mockStreamService{
		streams:     make(map[string]*streams.Stream),
		streamSpecs: make(map[string]*streams.StreamSpec),
		// validationProvider intentionally left nil
	}
	server := &Server{streamService: svc}

	_, err := server.getValidatedEncoders()
	if err == nil {
		t.Fatal("expected error when provider is nil")
	}
	if !strings.Contains(err.Error(), "validation data not found") {
		t.Errorf("error %q does not mention missing validation", err.Error())
	}
}
