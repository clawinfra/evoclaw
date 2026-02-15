package api

import (
	"io/fs"
	"net/http"
)

// handleTerminalPage serves the web terminal interface
func (s *Server) handleTerminalPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Try to serve from embedded FS first
	if s.webFS != nil {
		data, err := fs.ReadFile(s.webFS, "terminal.html")
		if err == nil {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write(data) // Ignore write errors (client disconnect)
			return
		}
	}

	// Fallback: serve basic HTML if terminal.html not embedded
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`
<!DOCTYPE html>
<html>
<head><title>EvoClaw Terminal</title></head>
<body>
	<h1>EvoClaw Terminal</h1>
	<p>Web terminal not available. Use the API at /api/chat</p>
	<pre>
POST /api/chat
{
  "agent": "agent-id",
  "message": "your message"
}
	</pre>
</body>
</html>
	`))
}
