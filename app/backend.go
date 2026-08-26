package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/ppvan/nem/extractor"
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

// openInBrowser launches url in the OS default browser. Uses the standard
// rundll32 trick (delegates to the shell's URL file-association handler)
// rather than a third-party package, since it's one line of stdlib
// os/exec and needs no extra dependency.
func openInBrowser(url string) error {
	if url == "" {
		return fmt.Errorf("no URL to open")
	}
	return newHiddenCmd("rundll32", "url.dll,FileProtocolHandler", url).Start()
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

// selectedEpisode pairs a 1-based position with the extractor.Episode
// needed to actually fetch it (episode.Title is used for display/filenames;
// Id/Href/Hash/MovieId just flow straight into ext.Download).
type selectedEpisode struct {
	number int
	ep     extractor.Episode
}

// downloadProgress is reported back to the UI while a batch download runs.
// Exactly one of (err != nil), done, or a plain fraction update is true
// per message.
type downloadProgress struct {
	label    string  // e.g. the episode's title, or "Ep. 3" if it has none
	fraction float64 // 0..1, updated via ext.Download's progress callback
	done     bool
	err      error
}

// downloadEpisodes downloads each selected episode in turn to destDir,
// reporting progress on the returned channel. The channel is closed once
// every episode has been attempted; a failure on one episode does not stop
// the rest of the batch.
func downloadEpisodes(ext *extractor.AniVietSubExtractor, episodes []selectedEpisode, destDir string) <-chan downloadProgress {
	ch := make(chan downloadProgress)

	go func() {
		defer close(ch)

		for _, se := range episodes {
			label := episodeLabel(se.ep, se.number)
			path := filepath.Join(destDir, episodeFileName(se))

			f, err := os.Create(path)
			if err != nil {
				ch <- downloadProgress{label: label, err: fmt.Errorf("create file: %w", err)}
				continue
			}

			dlErr := ext.Download(se.ep, f, func(progress float64) {
				ch <- downloadProgress{label: label, fraction: progress}
			})

			closeErr := f.Close()
			if dlErr == nil {
				dlErr = closeErr
			}

			if dlErr != nil {
				ch <- downloadProgress{label: label, err: dlErr}
				continue
			}

			ch <- downloadProgress{label: label, fraction: 1, done: true}
		}
	}()

	return ch
}

var unsafeFileChars = regexp.MustCompile(`[\\/:*?"<>|]`)

// episodeFileName builds a filesystem-safe name from the episode's title
// (falling back to its position if the title is empty), prefixed with a
// zero-padded number so files sort in order regardless of title.
func episodeFileName(se selectedEpisode) string {
	name := se.ep.Title
	if name == "" {
		name = fmt.Sprintf("Episode %d", se.number)
	}
	name = strings.TrimSpace(unsafeFileChars.ReplaceAllString(name, "_"))
	return fmt.Sprintf("%02d - %s.mp4", se.number, name)
}
