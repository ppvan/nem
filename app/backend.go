package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ppvan/nem/extractor"
	"golang.org/x/image/bmp"
)

const thumbnailFetchTimeout = 10 * time.Second

// fetchThumbnailBitmap downloads AnimeDetail.Thumbnail (a JPEG) and
// converts it to a 24-bit BMP in memory. That conversion is necessary
// because windigo's OleLoadPicture documents JPEG as a supported format,
// but it doesn't actually render JPEGs in practice — only BMP reliably
// works (see https://github.com/rodrigocfd/windigo/issues/46, filed by
// vye's own author). vye works around the same problem for PNG via
// pngToBitmapInMemory; this does the equivalent for JPEG.
//
// Returns (nil, nil) if url is empty — that's a normal "no poster" case,
// not an error.
func fetchThumbnailBitmap(url string) ([]byte, error) {
	jpegData, err := fetchThumbnail(url)
	if err != nil || jpegData == nil {
		return nil, err
	}
	return jpegToBitmapInMemory(jpegData)
}

// fetchThumbnail downloads the raw poster image bytes for AnimeDetail's
// Thumbnail URL (a plain .jpg link). Returns (nil, nil) if url is empty.
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

// jpegToBitmapInMemory decodes JPEG bytes and re-encodes them as a BMP.
// Mirrors vye's pngToBitmapInMemory, just starting from a different source
// format. Note: golang.org/x/image/bmp only supports certain bit depths on
// encode, same caveat vye's own comment calls out.
func jpegToBitmapInMemory(jpegData []byte) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return nil, fmt.Errorf("decode jpeg: %w", err)
	}

	var buf bytes.Buffer
	if err := bmp.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode bmp: %w", err)
	}
	return buf.Bytes(), nil
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
