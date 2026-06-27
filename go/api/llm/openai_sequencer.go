package llm

import (
	"os"
	"strings"
	"sync"
)

// inputTextSequencer serialises LLM calls that share the same InputText so
// identical document/chunk prefixes arrive at the provider back-to-back,
// maximising DeepSeek prompt-cache reuse. It is a keyed binary semaphore: calls
// with different InputText keys proceed concurrently; calls with the same key
// queue behind each other.
//
// Env LLM_INPUT_TEXT_SEQUENCER (default "true", set to "false" to disable).
// See ADR 2026062701 Phase 3.
type inputTextSequencer struct {
	mu    sync.Mutex
	slots map[string]chan struct{} // buffered(1) channel per InputText key
}

func (s *inputTextSequencer) acquire(key string) (release func()) {
	if s == nil || key == "" {
		return func() {}
	}
	s.mu.Lock()
	ch, ok := s.slots[key]
	if !ok {
		ch = make(chan struct{}, 1)
		s.slots[key] = ch
	}
	s.mu.Unlock()
	ch <- struct{}{} // acquire the per-key slot (blocks if another caller holds it)
	return func() { <-ch }
}

var globalInputTextSequencer = newInputTextSequencer()

func newInputTextSequencer() *inputTextSequencer {
	if !inputTextSequencerEnabled() {
		return nil
	}
	return &inputTextSequencer{slots: make(map[string]chan struct{})}
}

func inputTextSequencerEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_INPUT_TEXT_SEQUENCER")))
	return v != "false" && v != "0" && v != "no" && v != "off"
}
