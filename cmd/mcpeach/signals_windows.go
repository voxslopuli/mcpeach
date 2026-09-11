//go:build windows

package main

import "os"

// notifySignals are the signals that cancel the command context. Windows has
// no SIGTERM equivalent; os.Interrupt is the only portable signal.
var notifySignals = []os.Signal{os.Interrupt}
