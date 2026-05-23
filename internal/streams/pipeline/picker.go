package pipeline

// NeedsComposer reports whether a stream's pipeline must include the
// Composer stage. True when either:
//   - more than one input is declared (canvas case), OR
//   - any input has an effect (perspective today; crop/bbox future)
//
// False otherwise — Encoder dials the producer's SCM socket directly.
//
// The picker runs once at Pipeline.Apply time, not per frame. Effects
// being added to or removed from a single-input stream flips this
// answer and triggers a Composer engage/disengage (which entails an
// Encoder restart — the encoder's input topology changes between
// producer-NV12-Y4M and composer-BGRA-raw).
func NeedsComposer(s Stream) bool {
	if len(s.Inputs) > 1 {
		return true
	}
	if len(s.Effects) > 0 {
		for _, list := range s.Effects {
			if len(list) > 0 {
				return true
			}
		}
	}
	return false
}
