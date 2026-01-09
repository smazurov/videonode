//go:build linux

package alsa

import "testing"

func TestFormatALSADevice(t *testing.T) {
	tests := []struct {
		name      string
		cardNum   int
		deviceNum int
		expected  string
	}{
		{
			name:      "first card first device",
			cardNum:   0,
			deviceNum: 0,
			expected:  "hw:0,0",
		},
		{
			name:      "first card second device",
			cardNum:   0,
			deviceNum: 1,
			expected:  "hw:0,1",
		},
		{
			name:      "second card first device",
			cardNum:   1,
			deviceNum: 0,
			expected:  "hw:1,0",
		},
		{
			name:      "high card number",
			cardNum:   10,
			deviceNum: 5,
			expected:  "hw:10,5",
		},
		{
			name:      "large numbers",
			cardNum:   99,
			deviceNum: 99,
			expected:  "hw:99,99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatALSADevice(tt.cardNum, tt.deviceNum)
			if result != tt.expected {
				t.Errorf("FormatALSADevice(%d, %d) = %q, want %q",
					tt.cardNum, tt.deviceNum, result, tt.expected)
			}
		})
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected string
	}{
		{
			name:     "zero",
			input:    0,
			expected: "0",
		},
		{
			name:     "single digit",
			input:    5,
			expected: "5",
		},
		{
			name:     "two digits",
			input:    42,
			expected: "42",
		},
		{
			name:     "three digits",
			input:    123,
			expected: "123",
		},
		{
			name:     "negative single digit",
			input:    -5,
			expected: "-5",
		},
		{
			name:     "negative two digits",
			input:    -42,
			expected: "-42",
		},
		{
			name:     "negative three digits",
			input:    -123,
			expected: "-123",
		},
		{
			name:     "large positive",
			input:    1234567890,
			expected: "1234567890",
		},
		{
			name:     "large negative",
			input:    -1234567890,
			expected: "-1234567890",
		},
		{
			name:     "max int32",
			input:    2147483647,
			expected: "2147483647",
		},
		{
			name:     "min int32",
			input:    -2147483648,
			expected: "-2147483648",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := itoa(tt.input)
			if result != tt.expected {
				t.Errorf("itoa(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatName(t *testing.T) {
	tests := []struct {
		name     string
		format   int
		expected string
	}{
		// Signed 8-bit
		{name: "S8", format: FormatS8, expected: "S8"},
		// Unsigned 8-bit
		{name: "U8", format: FormatU8, expected: "U8"},
		// Signed 16-bit
		{name: "S16_LE", format: FormatS16LE, expected: "S16_LE"},
		{name: "S16_BE", format: FormatS16BE, expected: "S16_BE"},
		// Unsigned 16-bit
		{name: "U16_LE", format: FormatU16LE, expected: "U16_LE"},
		{name: "U16_BE", format: FormatU16BE, expected: "U16_BE"},
		// Signed 24-bit
		{name: "S24_LE", format: FormatS24LE, expected: "S24_LE"},
		{name: "S24_BE", format: FormatS24BE, expected: "S24_BE"},
		// Unsigned 24-bit
		{name: "U24_LE", format: FormatU24LE, expected: "U24_LE"},
		{name: "U24_BE", format: FormatU24BE, expected: "U24_BE"},
		// Signed 32-bit
		{name: "S32_LE", format: FormatS32LE, expected: "S32_LE"},
		{name: "S32_BE", format: FormatS32BE, expected: "S32_BE"},
		// Unsigned 32-bit
		{name: "U32_LE", format: FormatU32LE, expected: "U32_LE"},
		{name: "U32_BE", format: FormatU32BE, expected: "U32_BE"},
		// Float 32-bit
		{name: "FLOAT_LE", format: FormatFloatLE, expected: "FLOAT_LE"},
		{name: "FLOAT_BE", format: FormatFloatBE, expected: "FLOAT_BE"},
		// Float 64-bit
		{name: "FLOAT64_LE", format: FormatFloat64LE, expected: "FLOAT64_LE"},
		{name: "FLOAT64_BE", format: FormatFloat64BE, expected: "FLOAT64_BE"},
		// Special formats
		{name: "MU_LAW", format: FormatMuLaw, expected: "MU_LAW"},
		{name: "A_LAW", format: FormatALaw, expected: "A_LAW"},
		// Unknown formats
		{name: "unknown negative", format: -1, expected: "UNKNOWN"},
		{name: "unknown gap 18", format: 18, expected: "UNKNOWN"},
		{name: "unknown gap 19", format: 19, expected: "UNKNOWN"},
		{name: "unknown large", format: 100, expected: "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatName(tt.format)
			if result != tt.expected {
				t.Errorf("FormatName(%d) = %q, want %q", tt.format, result, tt.expected)
			}
		})
	}
}
