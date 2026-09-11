// Package server manages the lifecycle of MCP server subprocesses: spawn,
// capture output, stop, restart, and state tracking. Remote (SSE/streamable
// HTTP) connections are handled by internal/connect, not this package.
//
// Synchronization: the Manager's mutex protects the servers map and log rings.
// Each Server has its own mutex protecting its lifecycle state, so a Server
// pointer returned by Manager.Server can be used safely from multiple
// goroutines (each method is individually thread-safe). Lock ordering:
// Manager.mu is never held while acquiring a Server.mu.
package server

import (
	"errors"
	"fmt"
	"sync"
)

// State is the lifecycle state of a server.
type State int

const (
	// Stopped means the server is not running.
	Stopped State = iota
	// Running means the server process/connection is active.
	Running
	// Error means the server failed to start or crashed.
	Error
)

// String returns a human-readable state name.
func (s State) String() string {
	switch s {
	case Stopped:
		return "stopped"
	case Running:
		return "running"
	case Error:
		return "error"
	default:
		return "unknown"
	}
}

// validTransitions maps a current state to the set of allowed next states.
var validTransitions = map[State]map[State]bool{
	Stopped: {Running: true, Error: true},
	Running: {Stopped: true, Error: true},
	Error:   {Stopped: true, Running: true},
}

// Server is a single managed MCP server (stdio subprocess or remote endpoint).
// Its state is protected by its own mutex; each method is thread-safe.
type Server struct {
	mu    sync.RWMutex
	name  string
	state State
}

// New creates a Server in the Stopped state.
func New(name string) *Server {
	return &Server{name: name, state: Stopped}
}

// Name returns the server's configured name.
func (s *Server) Name() string { return s.name }

// State returns the current lifecycle state.
func (s *Server) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// transition moves the server to the given state, rejecting invalid jumps.
func (s *Server) transition(to State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validTransitions[s.state][to] {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, s.state, to)
	}
	s.state = to
	return nil
}

// Start transitions the server to Running.
func (s *Server) Start() error {
	if err := s.transition(Running); err != nil {
		return err
	}
	return nil
}

// Stop transitions the server to Stopped.
func (s *Server) Stop() error {
	if err := s.transition(Stopped); err != nil {
		return err
	}
	return nil
}

// Fail transitions the server to Error.
func (s *Server) Fail() error {
	if err := s.transition(Error); err != nil {
		return err
	}
	return nil
}

// ErrNotRunning is returned when an operation requires a running server.
var ErrNotRunning = errors.New("server is not running")

// ErrInvalidTransition is returned when a lifecycle transition is not allowed
// from the server's current state.
var ErrInvalidTransition = errors.New("invalid state transition")
