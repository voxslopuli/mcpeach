//go:build !windows

package main

import (
	"os"
	"syscall"
)

// notifySignals are the signals that cancel the command context. SIGTERM is
// how launchd/systemd stop the installed service; os.Interrupt covers Ctrl-C.
var notifySignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
