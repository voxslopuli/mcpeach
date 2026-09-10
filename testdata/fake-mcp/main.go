// Command fake-mcp is a minimal MCP server used in tests. It emits a JSON log
// line on startup and then blocks, so the server manager can exercise
// spawn/capture/stop lifecycle.
package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	// Emit a JSON log line to stderr (the manager captures stderr).
	fmt.Fprintln(os.Stderr, `{"level":"info","msg":"fake-mcp started","time":"`+time.Now().Format(time.RFC3339)+`"}`)
	// Block until killed.
	select {}
}
