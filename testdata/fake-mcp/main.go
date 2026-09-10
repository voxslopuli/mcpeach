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
	// Emit a JSON log line to stderr (the manager captures stderr), including
	// any args so tests can verify they reach the subprocess.
	fmt.Fprintf(os.Stderr, `{"level":"info","msg":"fake-mcp started","args":%q,"time":%q}`+"\n", os.Args[1:], time.Now().Format(time.RFC3339))
	// Block until killed.
	select {}
}
