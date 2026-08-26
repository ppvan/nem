package main

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"syscall"
	"time"
)

const thumbnailFetchTimeout = 10 * time.Second

// newHiddenCmd builds an exec.Cmd for name/args that won't flash a console
// window when started from this GUI app.
//
// Windows allocates a new console window for a spawned process whenever
// that process is a *console-subsystem* executable (as most portable mpv
// builds are, even though mpv opens its own GUI video window once it's
// running) — CreateProcess does this by default unless told not to.
// exec.Command doesn't set that flag itself, so without this, launching
// mpv briefly shows a console window before mpv's own window appears on
// top of it.
func newHiddenCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	return cmd
}

// launchMPV starts mpv as a separate, detached process pointed at url
// (expected to be a local http://127.0.0.1:... playlist URL from
// streamServer — see stream.go). Assumes mpv is already installed and on
// PATH; if it isn't, the returned error says so plainly enough
// ("exec: \"mpv\": executable file not found in %PATH%") that no extra
// wrapping is needed.
func launchMPV(url string) error {
	return newHiddenCmd("mpv", url).Start()
}

// fetchThumbnail downloads the raw poster image bytes for AnimeDetail's
// Thumbnail URL (a plain .jpg link). Returns (nil, nil) if url is empty —
// that's a normal "no poster" case, not an error.
//
// This is a plain HTTP GET with no COM/GDI involved, so — unlike
// decodeJpegPixels in thumbnail.go — it's safe to call from a background
// goroutine.
func fetchThumbnail(url string) ([]byte, error) {
	if url == "" {
		return nil, nil
	}

	client := http.Client{Timeout: thumbnailFetchTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch thumbnail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch thumbnail: unexpected status %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read thumbnail: %w", err)
	}
	return data, nil
}
