// Package browser opens URLs in the user's default browser. Wrapped behind a
// package-level var so tests and callers can stub the launcher without
// spawning a real browser process.
package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open hands url to the OS-default browser.
var Open = func(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("open browser: unsupported platform %s", runtime.GOOS)
	}
	return cmd.Start()
}
