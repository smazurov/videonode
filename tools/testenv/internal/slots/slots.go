// Package slots is the predictable port-slot allocator.
//
// Port names and formulas come from the project's .testenv.toml config.
// Slot 0 is reserved and never handed out.
//
// Pick returns the smallest unused slot whose ports all bind right
// now. The held *Listeners must be released (closed) immediately
// before exec'ing the daemon — the bind-and-hold pattern minimizes
// the TOCTOU window.
package slots

import (
	"errors"
	"fmt"
	"net"

	"github.com/smazurov/videonode/tools/testenv/internal/config"
	"github.com/smazurov/videonode/tools/testenv/internal/store"
)

// Held is a successful slot pick with listeners held open.
type Held struct {
	Slot      int
	Ports     map[string]int // port name → port number
	Listeners []net.Listener
}

// Release closes the held listeners. Safe to call multiple times.
func (h *Held) Release() {
	for _, ln := range h.Listeners {
		if ln != nil {
			ln.Close()
		}
	}
	h.Listeners = nil
}

// Pick walks slots 1..MaxSlots in order, skipping any taken by
// registered envs, and returns the first whose ports all bind.
func Pick(s *store.Store, cfg *config.V1) (*Held, error) {
	taken, err := s.TakenSlots()
	if err != nil {
		return nil, fmt.Errorf("read taken slots: %w", err)
	}
	takenSet := map[int]bool{}
	for _, t := range taken {
		takenSet[t] = true
	}
	portNames := cfg.PortNames()
	var lastBindErr error
	for i := 1; i <= cfg.MaxSlots; i++ {
		if takenSet[i] {
			continue
		}
		held, err := bindSlot(cfg, portNames, i)
		if err != nil {
			lastBindErr = err
			continue
		}
		return held, nil
	}
	if lastBindErr != nil {
		return nil, fmt.Errorf("no slot has all ports free; last bind error: %w", lastBindErr)
	}
	return nil, errors.New("no free slot (all taken)")
}

func bindSlot(cfg *config.V1, portNames []string, slot int) (*Held, error) {
	ports := map[string]int{}
	var listeners []net.Listener
	for _, name := range portNames {
		p := cfg.PortForSlot(name, slot)
		ports[name] = p
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			for _, l := range listeners {
				l.Close()
			}
			return nil, fmt.Errorf("bind :%d: %w", p, err)
		}
		listeners = append(listeners, ln)
	}
	return &Held{Slot: slot, Ports: ports, Listeners: listeners}, nil
}
