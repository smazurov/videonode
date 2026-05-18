package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveStreamsConfigPath(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		env      string
		wantPath string
	}{
		{
			name:     "default fallback",
			wantPath: "streams.toml",
		},
		{
			name:     "env overrides default",
			env:      "/etc/videonode/streams.toml",
			wantPath: "/etc/videonode/streams.toml",
		},
		{
			name:     "flag overrides env",
			flag:     "/flag/path.toml",
			env:      "/env/path.toml",
			wantPath: "/flag/path.toml",
		},
		{
			name:     "flag without env",
			flag:     "/only/flag.toml",
			wantPath: "/only/flag.toml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("STREAMS_CONFIG_FILE", tt.env)

			c := &cobra.Command{Use: "fake"}
			c.Flags().String("streams-config", "", "")
			if tt.flag != "" {
				if err := c.Flags().Set("streams-config", tt.flag); err != nil {
					t.Fatalf("set flag: %v", err)
				}
			}

			got := ResolveStreamsConfigPath(c)
			if got != tt.wantPath {
				t.Errorf("ResolveStreamsConfigPath = %q, want %q", got, tt.wantPath)
			}
		})
	}
}

func TestResolveStreamsConfigPath_NilCommand(t *testing.T) {
	t.Setenv("STREAMS_CONFIG_FILE", "/from/env.toml")
	if got := ResolveStreamsConfigPath(nil); got != "/from/env.toml" {
		t.Errorf("expected env fallback when cmd is nil, got %q", got)
	}

	t.Setenv("STREAMS_CONFIG_FILE", "")
	if got := ResolveStreamsConfigPath(nil); got != "streams.toml" {
		t.Errorf("expected default when cmd nil and env empty, got %q", got)
	}
}
