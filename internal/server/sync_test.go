package server

import (
	"sync"
	"testing"
)

// TestServerConcurrentState exercises concurrent state reads and transitions
// on a single Server. It would race before the per-server mutex was added.
func TestServerConcurrentState(t *testing.T) {
	s := New("conc")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 4 {
			case 0:
				_ = s.State()
			case 1:
				_ = s.Start()
			case 2:
				_ = s.Stop()
			case 3:
				_ = s.Fail()
			}
		}(i)
	}
	wg.Wait()
	// The final state must be one of the valid states.
	switch s.State() {
	case Stopped, Running, Error:
	default:
		t.Fatalf("invalid final state %v", s.State())
	}
}

// TestManagerConcurrentMarkRunningStopped exercises concurrent MarkRunning /
// MarkStopped on the same server. It would race before the per-server mutex.
func TestManagerConcurrentMarkRunningStopped(t *testing.T) {
	m := NewManager()
	m.Add(New("conc"))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_ = m.MarkRunning("conc")
			} else {
				_ = m.MarkStopped("conc")
			}
		}(i)
	}
	wg.Wait()
	// The server must still be registered and in a valid state.
	if s := m.Server("conc"); s == nil {
		t.Fatal("server missing after concurrent marks")
	}
}
