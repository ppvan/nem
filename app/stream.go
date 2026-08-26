package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ppvan/nem/extractor"
)

// streamServer is a local loopback HTTP server that lets an external
// player (mpv) play one or more episodes back-to-back without ever
// talking to the real site directly. It re-exposes
// extractor.AniVietSubExtractor.GetM3UPlaylist and DownloadSegment as
// plain HTTP endpoints, rewriting every URI in the playlist to point back
// at itself — so mpv only ever sees ordinary HTTP URLs on 127.0.0.1, and
// every actual fetch (with whatever headers, retries, or decryption the
// site needs) goes through the same extractor code Download() already
// uses.
//
// Two layers: /playlist.m3u8 serves one episode's actual HLS stream
// (proxying GetM3UPlaylist/DownloadSegment); /watch.m3u8 serves a plain
// mpv playlist chaining several /playlist.m3u8 URLs together, so clicking
// one episode can queue up everything from there to the end of the series
// — see NewWatchSession's doc comment for why that's a different thing
// from an HLS "master playlist" despite the similar name.
//
// IMPORTANT ASSUMPTION: this assumes DownloadSegment returns final,
// already-decrypted, directly playable segment bytes — mirroring how
// Download() writes finished playable output to its io.Writer. On that
// assumption, handlePlaylist strips any #EXT-X-KEY line from the playlist
// before serving it, so mpv doesn't also try to fetch a key and decrypt
// already-decrypted data itself. If DownloadSegment actually returns
// still-encrypted raw segment bytes instead, drop the key-stripping below
// and let mpv fetch the key/decrypt on its own (assuming the key URI is
// independently reachable).
type streamServer struct {
	ext *extractor.AniVietSubExtractor

	baseURL string // set once by Start, read-only after that

	mu         sync.Mutex
	sessions   map[string]extractor.Episode // token -> single episode, served via /playlist.m3u8
	watchLists map[string][]string          // token -> ordered per-episode playlist URLs, served via /watch.m3u8

	nextToken atomic.Int64
}

func newStreamServer(ext *extractor.AniVietSubExtractor) *streamServer {
	return &streamServer{
		ext:        ext,
		sessions:   make(map[string]extractor.Episode),
		watchLists: make(map[string][]string),
	}
}

// Start picks a free loopback port, starts serving in the background, and
// returns the server's base URL (e.g. "http://127.0.0.1:54321"). Runs for
// the lifetime of the process — there's no Stop(), since the whole app
// exiting is what tears it down.
func (s *streamServer) Start() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("start stream server: %w", err)
	}
	s.baseURL = fmt.Sprintf("http://%s", ln.Addr().String())

	mux := http.NewServeMux()
	mux.HandleFunc("/playlist.m3u8", s.handlePlaylist)
	mux.HandleFunc("/watch.m3u8", s.handleWatch)
	mux.HandleFunc("/seg", s.handleSeg)

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second, // local-only, but cheap hardening
	}
	go func() {
		_ = srv.Serve(ln) // process exit tears this down; error return is expected on shutdown
	}()

	return s.baseURL, nil
}

// NewSession registers ep under a fresh token and returns the playlist URL
// to hand to mpv. Each call gets its own token/entry, so playing several
// episodes (even concurrently, in separate mpv windows) never lets one
// playback's requests get served the wrong episode — unlike a design that
// tracked a single mutable "current episode".
func (s *streamServer) NewSession(ep extractor.Episode) (playlistURL string) {
	token := strconv.FormatInt(s.nextToken.Add(1), 10)

	s.mu.Lock()
	s.sessions[token] = ep
	s.mu.Unlock()

	return s.baseURL + "/playlist.m3u8?t=" + token
}

// NewWatchSession registers a fresh per-episode session (via NewSession)
// for every episode in episodes, then registers *that ordered list of
// playlist URLs* under its own token, returning a /watch.m3u8 URL.
//
// /watch.m3u8 is deliberately NOT an HLS master playlist
// (#EXT-X-STREAM-INF) — that tag is for bitrate/quality variants of the
// *same* video, and a real player picks exactly one variant and sticks
// with it, never sequences through them. What it serves instead is a
// plain mpv-style playlist: one URL per line, no HLS-specific tags. mpv
// (and ffmpeg's HLS demuxer, which mpv uses) only claims a .m3u8 as a real
// HLS stream when it contains HLS-required tags like #EXT-X-TARGETDURATION;
// without those, mpv's own playlist parser takes over and auto-advances
// through each URL in order — which is what gives "click once, watch
// through to the end" behavior across otherwise-unrelated episode files.
func (s *streamServer) NewWatchSession(episodes []extractor.Episode) (watchURL string) {
	epURLs := make([]string, len(episodes))
	for i, ep := range episodes {
		epURLs[i] = s.NewSession(ep)
	}

	token := strconv.FormatInt(s.nextToken.Add(1), 10)

	s.mu.Lock()
	s.watchLists[token] = epURLs
	s.mu.Unlock()

	return s.baseURL + "/watch.m3u8?t=" + token
}

func (s *streamServer) handleWatch(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("t")

	s.mu.Lock()
	epURLs, ok := s.watchLists[token]
	s.mu.Unlock()

	if !ok {
		http.Error(w, "unknown or expired session", http.StatusNotFound)
		return
	}

	var sb strings.Builder
	sb.WriteString("#EXTM3U\n")
	for _, u := range epURLs {
		sb.WriteString(u)
		sb.WriteString("\n")
	}

	// audio/x-mpegurl (the classic generic M3U type), not
	// application/vnd.apple.mpegurl (the HLS-specific one used by
	// /playlist.m3u8 below) — this isn't an HLS stream, it's a plain
	// playlist of separate HLS streams.
	w.Header().Set("Content-Type", "audio/x-mpegurl")
	_, _ = w.Write([]byte(sb.String()))
}

func (s *streamServer) handlePlaylist(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("t")

	s.mu.Lock()
	ep, ok := s.sessions[token]
	s.mu.Unlock()

	if !ok {
		http.Error(w, "unknown or expired session", http.StatusNotFound)
		return
	}

	data, err := s.ext.GetM3UPlaylist(ep)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Write(rewritePlaylist(data, s.baseURL))
}

// handleSeg proxies a single URI from the playlist through
// ext.DownloadSegment. If the fetched bytes turn out to be another
// playlist (a master playlist referencing per-quality media playlists,
// rather than a flat list of media segments), it's rewritten and served
// the same way — so nested playlists work without any special-casing,
// as long as DownloadSegment can fetch arbitrary URLs generically.
func (s *streamServer) handleSeg(w http.ResponseWriter, r *http.Request) {
	orig := r.URL.Query().Get("u")
	if orig == "" {
		http.Error(w, "missing u", http.StatusBadRequest)
		return
	}

	data, err := s.ext.DownloadSegment(orig)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	if looksLikePlaylist(orig, data) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Write(rewritePlaylist(data, s.baseURL))
		return
	}

	w.Header().Set("Content-Type", segmentContentType(orig))
	w.Write(data)
}

// rewritePlaylist rewrites every URI line in an m3u8 playlist to point at
// proxyBase+"/seg?u=<original>", so the player never fetches anything
// directly from the real site. Also drops #EXT-X-KEY lines — see the
// streamServer doc comment for why.
func rewritePlaylist(data []byte, proxyBase string) []byte {
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")

		switch {
		case strings.HasPrefix(trimmed, "#EXT-X-KEY"):
			continue // see streamServer's doc comment
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
			out = append(out, trimmed)
		default:
			// A URI line (segment or nested playlist reference).
			//
			// NOTE: assumes the URI is absolute. Relative URIs would need
			// to be resolved against the playlist's own URL first, which
			// GetM3UPlaylist doesn't expose — if segments in practice turn
			// out to be relative, this needs that URL threaded through.
			out = append(out, proxyBase+"/seg?u="+url.QueryEscape(trimmed))
		}
	}

	return []byte(strings.Join(out, "\n"))
}

func looksLikePlaylist(origURL string, data []byte) bool {
	path := strings.SplitN(origURL, "?", 2)[0]
	if strings.HasSuffix(strings.ToLower(path), ".m3u8") {
		return true
	}
	return bytes.HasPrefix(bytes.TrimSpace(data), []byte("#EXTM3U"))
}

func segmentContentType(origURL string) string {
	path := strings.ToLower(strings.SplitN(origURL, "?", 2)[0])
	switch {
	case strings.HasSuffix(path, ".ts"):
		return "video/mp2t"
	case strings.HasSuffix(path, ".m4s"), strings.HasSuffix(path, ".mp4"):
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}
