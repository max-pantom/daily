package idle

import (
	"bytes"
	"errors"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Duration returns approximate system idle time.
// Supports macOS (ioreg) and Linux (xprintidle). Returns error if unavailable.
func Duration() (time.Duration, error) {
	switch runtime.GOOS {
	case "darwin":
		return idleDarwin()
	case "linux":
		return idleLinux()
	default:
		return 0, errors.New("idle detection not supported")
	}
}

func idleDarwin() (time.Duration, error) {
	// ioreg returns nanoseconds in HIDIdleTime
	out, err := exec.Command("ioreg", "-c", "IOHIDSystem").Output()
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "HIDIdleTime") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			valStr := fields[len(fields)-1]
			// trim commas
			valStr = strings.Trim(valStr, ",")
			ns, err := strconv.ParseInt(valStr, 10, 64)
			if err != nil {
				continue
			}
			return time.Duration(ns), nil
		}
	}
	return 0, errors.New("HIDIdleTime not found")
}

func idleLinux() (time.Duration, error) {
	cmd := exec.Command("xprintidle")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	msStr := strings.TrimSpace(buf.String())
	ms, err := strconv.ParseInt(msStr, 10, 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(ms) * time.Millisecond, nil
}
