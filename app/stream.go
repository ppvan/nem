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

type streamServer struct {
	ext *extractor.AniVietSubExtractor

	baseURL string

	mu         sync.Mutex
	sessions   map[string]extractor.Episode
	watchLists map[string][]string

	nextToken atomic.Int64
}

func newStreamServer(ext *extractor.AniVietSubExtractor) *streamServer {
	return &streamServer{
		ext:        ext,
		sessions:   make(map[string]extractor.Episode),
		watchLists: make(map[string][]string),
	}
}

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
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		_ = srv.Serve(ln)
	}()

	return s.baseURL, nil
}

func (s *streamServer) NewSession(ep extractor.Episode) (playlistURL string) {
	token := strconv.FormatInt(s.nextToken.Add(1), 10)

	s.mu.Lock()
	s.sessions[token] = ep
	s.mu.Unlock()

	return s.baseURL + "/playlist.m3u8?t=" + token
}

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

func rewritePlaylist(data []byte, proxyBase string) []byte {
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")

		switch {
		case strings.HasPrefix(trimmed, "#EXT-X-KEY"):
			continue
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
			out = append(out, trimmed)
		default:

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
