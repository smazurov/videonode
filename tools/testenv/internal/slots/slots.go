// Package slots is the predictable port-slot allocator.
//
// Slot i in [1..9] maps to the port triple:
//
//	http = 8090 + 10*i
//	rtsp = 8554 + 10*i
//	srt  = 6001 + 10*i
//
// Slot 0 is reserved for the canonical air-driven daemon on default
// ports and is never handed out.
//
// Pick returns the smallest unused slot whose three ports actually
// bind right now. The held *Listeners must be released (closed)
// immediately before exec'ing the daemon — the bind-and-hold pattern
// minimizes the TOCTOU window between "looks free" and "daemon binds".
package slots

import (
	"errors"
	"fmt"
	"net"

	"github.com/smazurov/videonode/tools/testenv/internal/store"
)

// MaxSlot is the highest slot number the allocator hands out.
const MaxSlot = 9

// Triple holds the URLs the daemon will advertise for a slot.
type Triple struct {
	Slot int
	HTTP string // e.g. "http://localhost:8100"
	RTSP string // e.g. "rtsp://localhost:8564"
	SRT  string // e.g. "srt://localhost:6011"
}

// Held is a successful slot pick with all three port listeners held
// open. Caller must call Release before spawning the daemon and must
// not let Held escape without releasing it (Linux releases the FDs at
// process exit, so the worst case is the slot's ports stay free until
// the next allocator run).
type Held struct {
	Triple    Triple
	Listeners []net.Listener
}

// Release closes the held listeners. Safe to call multiple times.
func (h *Held) Release() {
	for _, ln := range h.Listeners {
		if ln != nil {
			_ = ln.Close()
		}
	}
	h.Listeners = nil
}

// PortsForSlot returns the port triple for slot i.
func PortsForSlot(i int) (http, rtsp, srt int) {
	return 8090 + 10*i, 8554 + 10*i, 6001 + 10*i
}

// Pick walks slots 1..MaxSlot in order, skipping any taken by
// registered envs, and returns the first whose three ports all bind.
// Returns nil with an error if no slot is available.
func Pick(s *store.Store) (*Held, error) {
	taken, err := s.TakenSlots()
	if err != nil {
		return nil, fmt.Errorf("read taken slots: %w", err)
	}
	takenSet := map[int]bool{}
	for _, t := range taken {
		takenSet[t] = true
	}
	var lastBindErr error
	for i := 1; i <= MaxSlot; i++ {
		if takenSet[i] {
			continue
		}
		held, err := bindTriple(i)
		if err != nil {
			lastBindErr = err
			continue
		}
		return held, nil
	}
	if lastBindErr != nil {
		return nil, fmt.Errorf("no slot has all three ports free; last bind error: %w", lastBindErr)
	}
	return nil, errors.New("no free slot (1..9 all taken)")
}

func bindTriple(slot int) (*Held, error) {
	http, rtsp, srt := PortsForSlot(slot)
	var held []net.Listener
	for _, p := range []int{http, rtsp, srt} {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			for _, h := range held {
				_ = h.Close()
			}
			return nil, fmt.Errorf("bind :%d: %w", p, err)
		}
		held = append(held, ln)
	}
	return &Held{
		Triple: Triple{
			Slot: slot,
			HTTP: fmt.Sprintf("http://localhost:%d", http),
			RTSP: fmt.Sprintf("rtsp://localhost:%d", rtsp),
			SRT:  fmt.Sprintf("srt://localhost:%d", srt),
		},
		Listeners: held,
	}, nil
}
