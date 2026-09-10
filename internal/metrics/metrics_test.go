package metrics

import (
	"sync"
	"testing"
	"time"
)

func TestRecordToolCall(t *testing.T) {
	m := New()
	m.RecordToolCall("github", "create_or_update_file", 5*time.Millisecond, nil)

	got := m.Snapshot()
	if len(got.Servers) != 1 {
		t.Fatalf("servers = %d, want 1", len(got.Servers))
	}
	s := got.Servers["github"]
	if s.Total != 1 || s.Success != 1 || s.Failure != 0 {
		t.Errorf("github = %+v, want total=1 success=1 failure=0", s)
	}
	if s.TotalLatency != 5*time.Millisecond {
		t.Errorf("total latency = %v, want 5ms", s.TotalLatency)
	}
}

func TestRecordToolCallFailure(t *testing.T) {
	m := New()
	m.RecordToolCall("github", "t1", 2*time.Millisecond, errBoom)

	s := m.Snapshot().Servers["github"]
	if s.Total != 1 || s.Success != 0 || s.Failure != 1 {
		t.Errorf("github = %+v, want total=1 success=0 failure=1", s)
	}
}

func TestRecordToolCallMultiple(t *testing.T) {
	m := New()
	for i := 0; i < 10; i++ {
		m.RecordToolCall("a", "t1", time.Millisecond, nil)
	}
	s := m.Snapshot().Servers["a"]
	if s.Total != 10 || s.Success != 10 {
		t.Errorf("a = %+v, want total=10 success=10", s)
	}
	if s.TotalLatency != 10*time.Millisecond {
		t.Errorf("total latency = %v, want 10ms", s.TotalLatency)
	}
}

func TestSnapshotEmpty(t *testing.T) {
	m := New()
	if got := m.Snapshot(); len(got.Servers) != 0 {
		t.Errorf("snapshot = %d servers, want 0", len(got.Servers))
	}
}

func TestConcurrent(t *testing.T) {
	m := New()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.RecordToolCall("a", "t1", time.Millisecond, nil)
		}()
	}
	wg.Wait()
	if s := m.Snapshot().Servers["a"]; s.Total != 50 {
		t.Errorf("total = %d, want 50", s.Total)
	}
}

var errBoom = &testErr{}

type testErr struct{}

func (e *testErr) Error() string { return "boom" }
