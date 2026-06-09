package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"

	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/store"
)

// CreateValidateEncodersCmd creates the validate-encoders command. The streamsConfigPath
// closure resolves the path at run time (after cobra has parsed flags), so callers can
// thread the same flag/env precedence the server uses.
func CreateValidateEncodersCmd(streamsConfigPath func(cmd *cobra.Command) string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate-encoders",
		Short: "Validate hardware encoder availability",
		Long: `This command tests hardware encoders (H.264 and H.265) to determine which ones actually work ` +
			`on the current system. Results are saved to streams.toml.`,
		Run: func(cmd *cobra.Command, _ []string) {
			quiet, _ := cmd.Flags().GetBool("quiet")
			path := streamsConfigPath(cmd)
			streamStore := store.NewTOML(path)
			validationService := streams.NewValidationService(streamStore)
			encoders.RunValidateCommandWithOptions(validationService, quiet)
		},
	}

	cmd.Flags().BoolP("quiet", "q", false, "Suppress detailed validation progress output")
	cmd.Flags().String("streams-config", "", "Path to streams.toml (overrides STREAMS_CONFIG_FILE env)")
	return cmd
}

// CreateValidateConfigCmd creates the validate-config command. Loads a v2
// streams.toml and runs structural validation against the v2 schema
// (sources/composers/streams). Prints actionable errors and exits non-zero
// on the first failure.
func CreateValidateConfigCmd(streamsConfigPath func(cmd *cobra.Command) string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate-config",
		Short: "Validate streams.toml structure (sources/composers/streams refs)",
		Long: `Catches dangling upstream refs ("source:X" where source X does not exist), ` +
			`layout entries pointing at unknown inputs, source-id / composer-id / stream-id ` +
			`collisions, and source TestMode/Device conflicts.`,
		RunE: func(c *cobra.Command, _ []string) error {
			path := streamsConfigPath(c)
			cfg, err := loadV2Config(path)
			if err != nil {
				if _, perr := fmt.Fprintf(c.ErrOrStderr(), "validate-config: %s\n", err); perr != nil {
					return perr
				}
				os.Exit(1)
			}
			if errs := ValidateV2Config(cfg); len(errs) > 0 {
				if _, perr := fmt.Fprintf(c.ErrOrStderr(), "validate-config: %d error(s) in %s\n", len(errs), path); perr != nil {
					return perr
				}
				for _, e := range errs {
					if _, perr := fmt.Fprintf(c.ErrOrStderr(), "  - %s\n", e); perr != nil {
						return perr
					}
				}
				os.Exit(1)
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "validate-config: ok (%d sources, %d composers, %d streams)\n",
				len(cfg.Sources), len(cfg.Composers), len(cfg.Streams))
			return err
		},
	}
	cmd.Flags().String("streams-config", "", "Path to streams.toml (overrides STREAMS_CONFIG_FILE env)")
	return cmd
}

// loadV2Config reads a v2 streams.toml at path. A non-v2 file is rejected
// with a clear error; v1 auto-migration has been removed.
func loadV2Config(path string) (*V2Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	// Peek at the version before binding the full document so any non-v2
	// shape (including the legacy [streams.<id>] table) fails with a clear
	// message rather than a confusing table-vs-slice decode error.
	var head struct {
		Version int `toml:"version"`
	}
	if err := toml.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if head.Version != 2 {
		return nil, fmt.Errorf("%s: version %d unsupported: v1→v2 auto-migration was "+
			"removed; restore a version-2 config", path, head.Version)
	}

	var cfg V2Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &cfg, nil
}

// ValidateV2Config runs structural validation against a parsed v2 config.
// Returns the full list of problems so callers can report all errors at once
// instead of bailing on the first.
func ValidateV2Config(cfg *V2Config) []string {
	var errs []string

	// Source-id collisions + TestMode/Device exclusivity.
	sourceIDs := map[string]bool{}
	for _, s := range cfg.Sources {
		if s.ID == "" {
			errs = append(errs, "source with empty id")
			continue
		}
		if sourceIDs[s.ID] {
			errs = append(errs, fmt.Sprintf("source id collision: %q defined more than once", s.ID))
		}
		sourceIDs[s.ID] = true
		switch {
		case s.TestMode && s.Device != "":
			errs = append(errs, fmt.Sprintf("source %q: test_mode and device are mutually exclusive", s.ID))
		case !s.TestMode && s.Device == "":
			errs = append(errs, fmt.Sprintf("source %q: must set either device or test_mode", s.ID))
		}
	}

	// Composer-id collisions + per-input/layout ref validation.
	composerIDs := map[string]bool{}
	composerInputRefs := map[string]map[string]bool{} // composer-id → set of input refs
	for _, c := range cfg.Composers {
		if c.ID == "" {
			errs = append(errs, "composer with empty id")
			continue
		}
		if composerIDs[c.ID] {
			errs = append(errs, fmt.Sprintf("composer id collision: %q defined more than once", c.ID))
		}
		composerIDs[c.ID] = true
		if c.Canvas.W <= 0 || c.Canvas.H <= 0 {
			errs = append(errs, fmt.Sprintf("composer %q: canvas must have positive w/h (got %dx%d)",
				c.ID, c.Canvas.W, c.Canvas.H))
		}

		inputRefs := map[string]bool{}
		for i, in := range c.Inputs {
			if in.Ref == "" {
				errs = append(errs, fmt.Sprintf("composer %q input[%d]: empty ref", c.ID, i))
				continue
			}
			kind, id, ok := splitRef(in.Ref)
			if !ok {
				errs = append(errs, fmt.Sprintf("composer %q input[%d]: malformed ref %q (want \"source:<id>\")",
					c.ID, i, in.Ref))
				continue
			}
			if kind != "source" {
				errs = append(errs, fmt.Sprintf("composer %q input[%d]: ref %q must be a source",
					c.ID, i, in.Ref))
				continue
			}
			if !sourceIDs[id] {
				errs = append(errs, fmt.Sprintf("composer %q input[%d]: unknown source %q",
					c.ID, i, id))
			}
			if inputRefs[in.Ref] {
				errs = append(errs, fmt.Sprintf("composer %q input[%d]: duplicate ref %q",
					c.ID, i, in.Ref))
			}
			inputRefs[in.Ref] = true
		}
		composerInputRefs[c.ID] = inputRefs

		for i, slot := range c.Layout {
			if slot.Input == "" {
				errs = append(errs, fmt.Sprintf("composer %q layout[%d]: empty input", c.ID, i))
				continue
			}
			if !inputRefs[slot.Input] {
				errs = append(errs, fmt.Sprintf("composer %q layout[%d]: input %q not declared in inputs",
					c.ID, i, slot.Input))
			}
			if slot.W <= 0 || slot.H <= 0 {
				errs = append(errs, fmt.Sprintf("composer %q layout[%d]: w/h must be positive (got %dx%d)",
					c.ID, i, slot.W, slot.H))
			}
		}
	}

	// Stream-id collisions + upstream ref validation.
	streamIDs := map[string]bool{}
	for _, s := range cfg.Streams {
		if s.ID == "" {
			errs = append(errs, "stream with empty id")
			continue
		}
		if streamIDs[s.ID] {
			errs = append(errs, fmt.Sprintf("stream id collision: %q defined more than once", s.ID))
		}
		streamIDs[s.ID] = true
		if s.Upstream == "" {
			errs = append(errs, fmt.Sprintf("stream %q: missing upstream", s.ID))
			continue
		}
		kind, id, ok := splitRef(s.Upstream)
		if !ok {
			errs = append(errs, fmt.Sprintf("stream %q: malformed upstream %q (want \"source:<id>\" or \"composer:<id>\")",
				s.ID, s.Upstream))
			continue
		}
		switch kind {
		case "source":
			if !sourceIDs[id] {
				errs = append(errs, fmt.Sprintf("stream %q: dangling upstream source %q", s.ID, id))
			}
		case "composer":
			if !composerIDs[id] {
				errs = append(errs, fmt.Sprintf("stream %q: dangling upstream composer %q", s.ID, id))
			}
		default:
			errs = append(errs, fmt.Sprintf("stream %q: upstream kind must be source or composer (got %q)",
				s.ID, kind))
		}
	}

	sort.Strings(errs)
	return errs
}

// splitRef splits an upstream reference like "source:foo" into ("source", "foo", true).
// Returns ok=false for malformed input.
func splitRef(s string) (kind, id string, ok bool) {
	idx := strings.IndexByte(s, ':')
	if idx <= 0 || idx == len(s)-1 {
		return "", "", false
	}
	return s[:idx], s[idx+1:], true
}
