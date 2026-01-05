package streaming

import "testing"

func TestH264ProfileFromFmtp(t *testing.T) {
	tests := []struct {
		name     string
		fmtp     string
		expected byte
	}{
		{
			name:     "high profile from profile-level-id",
			fmtp:     "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640034",
			expected: 0x64,
		},
		{
			name:     "high profile level 5.0",
			fmtp:     "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640032",
			expected: 0x64,
		},
		{
			name:     "baseline profile 42001f",
			fmtp:     "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
			expected: 0x42,
		},
		{
			name:     "constrained baseline 42e01f",
			fmtp:     "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
			expected: 0x42,
		},
		{
			name:     "main profile",
			fmtp:     "profile-level-id=4d001f",
			expected: 0x4D,
		},
		{
			name:     "profile-level-id at start",
			fmtp:     "profile-level-id=640034;packetization-mode=1",
			expected: 0x64,
		},
		{
			name:     "sprop-parameter-sets baseline fallback",
			fmtp:     "sprop-parameter-sets=Z0IAKeKQFAe2AtwEBAaQeJEV,aM48gA==",
			expected: 0x42, // SPS starts with 0x42 (Baseline)
		},
		{
			name:     "empty fmtp",
			fmtp:     "",
			expected: 0,
		},
		{
			name:     "no profile info",
			fmtp:     "packetization-mode=1",
			expected: 0,
		},
		{
			name:     "profile-level-id with spaces",
			fmtp:     "profile-level-id=640034 ;packetization-mode=1",
			expected: 0x64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h264ProfileFromFmtp(tt.fmtp)
			if got != tt.expected {
				t.Errorf("h264ProfileFromFmtp(%q) = 0x%02X, want 0x%02X", tt.fmtp, got, tt.expected)
			}
		})
	}
}
