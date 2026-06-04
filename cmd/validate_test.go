package cmd

import (
	"strings"
	"testing"
)

func TestValidateV2Config(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *V2Config
		wantSub []string // substrings that must appear in the error list
		wantOK  bool
	}{
		{
			name: "valid two-source one-composer",
			cfg: &V2Config{
				Version: 2,
				Sources: []V2Source{
					{ID: "cam-a", Device: "usb-1-2"},
					{ID: "cam-b", Device: "usb-1-3"},
				},
				Composers: []V2Composer{{
					ID:     "main",
					Canvas: V2CanvasDims{W: 1920, H: 1080},
					Inputs: []V2ComposerInput{
						{Ref: "source:cam-a"},
						{Ref: "source:cam-b"},
					},
					Layout: []V2LayoutSlot{
						{Input: "source:cam-a", X: 0, Y: 0, W: 960, H: 1080},
						{Input: "source:cam-b", X: 960, Y: 0, W: 960, H: 1080},
					},
				}},
				Streams: []V2Stream{
					{ID: "archive", Upstream: "composer:main"},
					{ID: "solo", Upstream: "source:cam-a"},
				},
			},
			wantOK: true,
		},
		{
			name: "dangling source upstream",
			cfg: &V2Config{
				Streams: []V2Stream{{ID: "s1", Upstream: "source:ghost"}},
			},
			wantSub: []string{"dangling upstream source"},
		},
		{
			name: "dangling composer upstream",
			cfg: &V2Config{
				Streams: []V2Stream{{ID: "s1", Upstream: "composer:ghost"}},
			},
			wantSub: []string{"dangling upstream composer"},
		},
		{
			name: "layout points at unknown input",
			cfg: &V2Config{
				Sources: []V2Source{{ID: "cam-a", Device: "usb-1-2"}},
				Composers: []V2Composer{{
					ID:     "main",
					Canvas: V2CanvasDims{W: 1920, H: 1080},
					Inputs: []V2ComposerInput{{Ref: "source:cam-a"}},
					Layout: []V2LayoutSlot{
						{Input: "source:cam-b", X: 0, Y: 0, W: 100, H: 100},
					},
				}},
			},
			wantSub: []string{"not declared in inputs"},
		},
		{
			name: "source id collision",
			cfg: &V2Config{
				Sources: []V2Source{
					{ID: "dup", Device: "usb-1-2"},
					{ID: "dup", Device: "usb-1-3"},
				},
			},
			wantSub: []string{"source id collision"},
		},
		{
			name: "composer id collision",
			cfg: &V2Config{
				Composers: []V2Composer{
					{ID: "dup", Canvas: V2CanvasDims{W: 1, H: 1}},
					{ID: "dup", Canvas: V2CanvasDims{W: 1, H: 1}},
				},
			},
			wantSub: []string{"composer id collision"},
		},
		{
			name: "stream id collision",
			cfg: &V2Config{
				Sources: []V2Source{{ID: "cam", Device: "x"}},
				Streams: []V2Stream{
					{ID: "dup", Upstream: "source:cam"},
					{ID: "dup", Upstream: "source:cam"},
				},
			},
			wantSub: []string{"stream id collision"},
		},
		{
			name: "source testmode and device both set",
			cfg: &V2Config{
				Sources: []V2Source{{ID: "bad", Device: "usb-1-2", TestMode: true}},
			},
			wantSub: []string{"test_mode and device are mutually exclusive"},
		},
		{
			name: "source neither testmode nor device",
			cfg: &V2Config{
				Sources: []V2Source{{ID: "bad"}},
			},
			wantSub: []string{"must set either device or test_mode"},
		},
		{
			name: "malformed upstream ref",
			cfg: &V2Config{
				Streams: []V2Stream{{ID: "s1", Upstream: "garbage"}},
			},
			wantSub: []string{"malformed upstream"},
		},
		{
			name: "unknown upstream kind",
			cfg: &V2Config{
				Streams: []V2Stream{{ID: "s1", Upstream: "device:foo"}},
			},
			wantSub: []string{"must be source or composer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateV2Config(tt.cfg)
			if tt.wantOK {
				if len(errs) > 0 {
					t.Fatalf("expected no errors, got: %v", errs)
				}
				return
			}
			joined := strings.Join(errs, "\n")
			for _, sub := range tt.wantSub {
				if !strings.Contains(joined, sub) {
					t.Errorf("expected error containing %q; got:\n%s", sub, joined)
				}
			}
		})
	}
}

func TestSplitRef(t *testing.T) {
	tests := []struct {
		in       string
		kind, id string
		ok       bool
	}{
		{"source:cam-a", "source", "cam-a", true},
		{"composer:main", "composer", "main", true},
		{"source:", "", "", false},
		{":id", "", "", false},
		{"garbage", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			kind, id, ok := splitRef(tt.in)
			if kind != tt.kind || id != tt.id || ok != tt.ok {
				t.Errorf("splitRef(%q) = (%q, %q, %v); want (%q, %q, %v)",
					tt.in, kind, id, ok, tt.kind, tt.id, tt.ok)
			}
		})
	}
}
