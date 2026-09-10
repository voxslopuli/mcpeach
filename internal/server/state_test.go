package server

import (
	"testing"
)

func TestStateTransitions(t *testing.T) {
	tests := []struct {
		name    string
		from    State
		to      State
		wantErr bool
	}{
		{"stopped to running", Stopped, Running, false},
		{"stopped to error", Stopped, Error, false},
		{"running to stopped", Running, Stopped, false},
		{"running to error", Running, Error, false},
		{"error to stopped", Error, Stopped, false},
		{"error to running", Error, Running, false},
		{"stopped to stopped", Stopped, Stopped, true},
		{"running to running", Running, Running, true},
		{"error to error", Error, Error, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{state: tt.from}
			err := s.transition(tt.to)
			if tt.wantErr && err == nil {
				t.Errorf("transition %s->%s: want error, got nil", tt.from, tt.to)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("transition %s->%s: want nil, got %v", tt.from, tt.to, err)
			}
			if !tt.wantErr && s.state != tt.to {
				t.Errorf("state = %s, want %s", s.state, tt.to)
			}
		})
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{Stopped, "stopped"},
		{Running, "running"},
		{Error, "error"},
		{State(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}
