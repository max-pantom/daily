package notify

import (
	"os/exec"
	"runtime"
)

// Send best-effort desktop notification. Falls back silently if unavailable.
func Send(title, message string) {
	switch runtime.GOOS {
	case "darwin":
		// osascript native notification
		_ = exec.Command("osascript", "-e", `display notification "`+escape(message)+`" with title "`+escape(title)+`"`).Run()
	case "linux":
		_ = exec.Command("notify-send", title, message).Run()
	default:
		// no-op for other platforms
	}
}

func escape(s string) string {
	// Minimal escaping for osascript quotes.
	return escapeQuotes(s)
}

func escapeQuotes(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '"' {
			out = append(out, '\\', '"')
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}
