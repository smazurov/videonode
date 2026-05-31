package streaming

import "testing"

// isICEChar reports whether r is in RFC 5245's ice-char set
// (ALPHA / DIGIT / "+" / "/"), which both ice-ufrag and ice-pwd must satisfy.
func isICEChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '+', r == '/':
		return true
	default:
		return false
	}
}

func TestICEUfragFromPeerID(t *testing.T) {
	tests := []struct {
		name   string
		peerID string
		want   string
	}{
		{"hyphen stripped", "causal-treefrog", "causaltreefrog"},
		{"multiple hyphens", "magnetic-cougar-1234", "magneticcougar1234"},
		{"already clean", "abcd", "abcd"},
		{"short input padded", "ab", "abpeer"},
		{"empty input padded", "", "peer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := iceUfragFromPeerID(tt.peerID)
			if got != tt.want {
				t.Errorf("iceUfragFromPeerID(%q) = %q, want %q", tt.peerID, got, tt.want)
			}
			if len(got) < 4 {
				t.Errorf("ufrag %q is shorter than the RFC 5245 minimum of 4", got)
			}
			for _, r := range got {
				if !isICEChar(r) {
					t.Errorf("ufrag %q contains non-ice-char %q", got, r)
				}
			}
		})
	}
}

func TestGenerateICEPassword(t *testing.T) {
	pw := generateICEPassword()
	if len(pw) < 22 {
		t.Errorf("ICE password %q is shorter than the RFC 5245 minimum of 22", pw)
	}
	for _, r := range pw {
		if !isICEChar(r) {
			t.Errorf("ICE password %q contains non-ice-char %q", pw, r)
		}
	}
}
