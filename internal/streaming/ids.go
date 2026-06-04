package streaming

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"

	"github.com/smazurov/videonode/internal/logging"
)

// generateClientID returns a short random identifier shared by both streaming
// consumer types (WebRTC peers and SRT consumers): 4 bytes of crypto/rand
// encoded as 8 hex chars. Hex keeps the ID within RFC 5245's ice-char set, so
// it is safe to reuse directly as a WebRTC ICE ufrag.
func generateClientID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		slog.Error("Failed to generate random bytes for client ID", logging.KeyError, err)
	}
	return hex.EncodeToString(b)
}
