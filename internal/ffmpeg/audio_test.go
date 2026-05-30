package ffmpeg

import "testing"

func TestDsnoopDevice(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"hw card and device", "hw:3,0", "vncap:CARD=3,DEV=0"},
		{"hw card only defaults device", "hw:2", "vncap:CARD=2,DEV=0"},
		{"hw nonzero device", "hw:1,2", "vncap:CARD=1,DEV=2"},
		{"hw named card", "hw:Lyra,0", "vncap:CARD=Lyra,DEV=0"},
		{"plughw passthrough", "plughw:3,0", "plughw:3,0"},
		{"dsnoop passthrough", "dsnoop:CARD=3,DEV=0", "dsnoop:CARD=3,DEV=0"},
		{"default passthrough", "default", "default"},
		{"vncap passthrough", "vncap:CARD=3,DEV=0", "vncap:CARD=3,DEV=0"},
		{"empty passthrough", "", ""},
		{"bare hw prefix passthrough", "hw:", "hw:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DsnoopDevice(tt.in); got != tt.want {
				t.Errorf("DsnoopDevice(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMapCaptureDevice(t *testing.T) {
	t.Run("rewrites when ALSA_CONFIG_PATH set", func(t *testing.T) {
		t.Setenv("ALSA_CONFIG_PATH", "/etc/videonode/videonode.asound.conf")
		if got := MapCaptureDevice("hw:3,0"); got != "vncap:CARD=3,DEV=0" {
			t.Errorf("MapCaptureDevice(hw:3,0) = %q, want vncap:CARD=3,DEV=0", got)
		}
	})
	t.Run("passthrough when ALSA_CONFIG_PATH empty", func(t *testing.T) {
		t.Setenv("ALSA_CONFIG_PATH", "")
		if got := MapCaptureDevice("hw:3,0"); got != "hw:3,0" {
			t.Errorf("MapCaptureDevice(hw:3,0) = %q, want hw:3,0 (no rewrite)", got)
		}
	})
}
