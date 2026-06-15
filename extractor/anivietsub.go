package extractor

import (
	"bytes"
	"compress/flate"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/publicsuffix"
)

// KEY is kept for backward-compat with the old AES-CBC path (decryptVideoSourceLegacy).
// New playlists use decryptPlaylist() which derives the key from HMAC + response headers.
var KEY = []byte{100, 109, 95, 116, 104, 97, 110, 103, 95, 115, 117, 99, 95, 118, 97, 116, 95, 103, 101, 116, 95, 108, 105, 110, 107, 95, 97, 110, 95, 100, 98, 116}

const USER_AGENT = "Mozilla/5.0 (iPhone; CPU iPhone OS 16_1_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) GSA/383.0.797833943 Mobile/15E148 Safari/604.1"
const SEARCH_API = "/ajax/suggest"
const PLAYLIST_API = "/ajax/player"
const TRENDING_API = "/bang-xep-hang/season.html"

const (
	maxRetries = 3
	retryDelay = 2 * time.Second
)

type AniVietSubExtractor struct {
	domain string
	client *http.Client
	jar    *cookiejar.Jar
}

type EncryptedPlaylist struct {
	Success int    `json:"success"`
	Title   string `json:"title"`
	Link    string `json:"link"`
}

type LinkObj struct {
	File string `json:"file"`
}

// playlistEncryptionHeaders holds the decryption parameters delivered via
// HTTP response headers alongside the encrypted M3U8 playlist.
//
// These map to the JS variables as follows:
//
//	X-Digest-Tag       → hmacKeyBase64  (base64url-encoded HMAC-SHA-256 key)
//	X-Proxy-After      → proxyAfter     (mixed into the HMAC message as "plaintext")
//	X-Cache-Playlist   → cachePlaylist  (segment counter / index)
//	X-Request-Playlist → requestPlaylist (URI-decoded extra context)
type playlistEncryptionHeaders struct {
	hmacKeyBase64   string // X-Digest-Tag
	proxyAfter      string // X-Proxy-After
	cachePlaylist   string // X-Cache-Playlist
	requestPlaylist string // X-Request-Playlist (already URL-decoded)
}

func NewAniVietSubExtractor(domain string) (*AniVietSubExtractor, error) {

	// Init cookie jar (uses publicsuffix to handle domain scoping correctly)
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: false,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	client := &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
		Jar:       jar,
	}

	// Auto resolve domain if not provided
	if domain == "" {
		resp, err := http.Get("https://bit.ly/animevietsubtv")
		if err != nil {
			return nil, fmt.Errorf("can't auto resolve animevietsub domain: %w", err)
		}
		domain = resp.Request.URL.String()
	}

	ex := &AniVietSubExtractor{
		client: client,
		domain: domain,
		jar:    jar,
	}

	// Warm up: fetch homepage to get Cloudflare cookies before any real request
	if err := ex.warmUp(); err != nil {
		return nil, fmt.Errorf("warmup failed: %w", err)
	}

	return ex, nil
}

// warmUp fetches the homepage to collect Cloudflare session cookies.
func (ex *AniVietSubExtractor) warmUp() error {
	req, err := http.NewRequest(http.MethodGet, ex.domain, nil)
	if err != nil {
		return err
	}
	ex.setCommonHeaders(req)

	resp, err := ex.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return nil
}

// doWithRetry executes a request and retries on 403 (Cloudflare challenge).
func (ex *AniVietSubExtractor) doWithRetry(req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	if req.Body != nil && req.Body != http.NoBody {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	ex.setCommonHeaders(req)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			fmt.Printf("403 received, retrying (%d/%d) after warm-up...\n", attempt, maxRetries)
			time.Sleep(retryDelay)

			if err := ex.warmUp(); err != nil {
				return nil, fmt.Errorf("warm-up failed on retry: %w", err)
			}

			if bodyBytes != nil {
				req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}
		}

		resp, err := ex.client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusForbidden {
			return resp, nil
		}

		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	return nil, fmt.Errorf("request failed with 403 after %d retries", maxRetries)
}

// setCommonHeaders sets browser-like headers to reduce bot detection.
func (ex *AniVietSubExtractor) setCommonHeaders(req *http.Request) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", USER_AGENT)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "vi-VN,vi;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Referer", ex.domain)
}

func (ex *AniVietSubExtractor) Search(query string) ([]SimpleAnime, error) {
	api := mustJoinPath(ex.domain, SEARCH_API)
	body := url.Values{
		"ajaxSearch": {"1"},
		"keysearch":  {query},
	}

	req, err := http.NewRequest(http.MethodPost, api, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	r, err := ex.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %s", err)
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error: %s", r.Status)
	}

	movies, err := extractMovies(r.Body)
	if err != nil {
		return nil, fmt.Errorf("parse error: %s", err)
	}

	return movies, nil
}

func (ex *AniVietSubExtractor) GetAnimeDetails(id int) (*AnimeDetail, error) {
	u := mustJoinPath(ex.domain, "phim", fmt.Sprintf("-%d", id), "xem-phim.html")

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	r, err := ex.doWithRetry(req)
	if err != nil {
		return nil, err
	}

	bodyBytes, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return nil, err
	}

	return parseAnimeVietsubAnimeDetails(id, bytes.NewReader(bodyBytes))
}

func (ex *AniVietSubExtractor) Trending() ([]SimpleAnime, error) {
	api := mustJoinPath(ex.domain, TRENDING_API)

	req, err := http.NewRequest(http.MethodGet, api, nil)
	if err != nil {
		return nil, err
	}

	r, err := ex.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %s", err)
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error: %s", r.Status)
	}

	movies, err := extractTrendingMovies(r.Body)
	if err != nil {
		return nil, fmt.Errorf("parse error: %s", err)
	}

	return movies, nil
}

// GetM3UPlaylist fetches the encrypted playlist for an episode and returns the
// decrypted M3U8 content.
//
// Flow:
//  1. POST to PLAYLIST_API with the episode hash + movie ID.
//  2. Decode the JSON response to get the playlist URL (playlist.Link).
//  3. Fetch that URL; the response headers carry the AES-GCM encryption params.
//  4. Parse the M3U8 body to extract the joined _t= tokens from segment lines.
//  5. Decrypt the tokens using the HMAC-derived AES-GCM key.
//  6. Replace the encrypted segment lines with the decrypted real URLs.
func (ex *AniVietSubExtractor) GetM3UPlaylist(e Episode) ([]byte, error) {

	req, err := http.NewRequest(http.MethodGet, e.Href, nil)
	if err != nil {
		return nil, err
	}
	r, err := ex.doWithRetry(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	s, _ := (io.ReadAll(r.Body))

	playerLinkRe := regexp.MustCompile(`PLAYER_DATA.+("link":)"(https[^"]+)"`)
	match := playerLinkRe.FindSubmatch(s)

	rawPlayerLink := (string(match[2]))

	playerLink := strings.ReplaceAll(rawPlayerLink, `\/`, `/`)

	req, err = http.NewRequest(http.MethodGet, playerLink, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create player page request: %w", err)
	}
	resp, err := ex.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch player page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read player page body: %w", err)
	}

	// Extract id and avsToken from the <script> block using regex
	idRegex := regexp.MustCompile(`const id\s*=\s*"([^"]+)"`)
	tokenRegex := regexp.MustCompile(`const avsToken\s*=\s*"([^"]+)"`)

	idMatch := idRegex.FindSubmatch(body)
	if idMatch == nil {
		return nil, fmt.Errorf("failed to extract id from player page")
	}
	tokenMatch := tokenRegex.FindSubmatch(body)
	if tokenMatch == nil {
		return nil, fmt.Errorf("failed to extract avsToken from player page")
	}

	videoID := string(idMatch[1])
	avsToken := string(tokenMatch[1])

	// Build the actual playlist URL
	// https://storage.googleapiscdn.com/playlist/$id/playlist.m3u8?token=$token
	playlistURL := fmt.Sprintf("https://storage.googleapiscdn.com/playlist/%s/playlist.m3u8?token=%s", videoID, avsToken)

	return ex.fetchAndDecryptPlaylist(playlistURL, avsToken)
}

// fetchAndDecryptPlaylist fetches an M3U8 URL, reads the encryption headers,
// and returns the decrypted playlist body.
func (ex *AniVietSubExtractor) fetchAndDecryptPlaylist(playlistURL string, token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, playlistURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", ex.domain)
	req.Header.Set("User-Agent", USER_AGENT)

	resp, err := ex.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("playlist fetch returned HTTP %d", resp.StatusCode)
	}

	headers := ExtractEnvelope(resp.Header)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read playlist body: %w", err)
	}

	return DecryptPlaylist(body, &headers, token, req.Host)
}

// ─── Encryption header extraction ────────────────────────────────────────────

// extractEncryptionHeaders reads the four custom headers that carry the
// AES-GCM decryption parameters.
func extractEncryptionHeaders(h http.Header) playlistEncryptionHeaders {
	requestPlaylist, _ := url.QueryUnescape(h.Get("X-Request-Playlist"))
	if requestPlaylist == "" {
		requestPlaylist = "dest"
	}

	cachePlaylist := h.Get("X-Cache-Playlist")
	if cachePlaylist == "" {
		cachePlaylist = "0"
	}

	return playlistEncryptionHeaders{
		hmacKeyBase64:   h.Get("X-Digest-Tag"),
		proxyAfter:      h.Get("X-Proxy-After"),
		cachePlaylist:   cachePlaylist,
		requestPlaylist: requestPlaylist,
	}
}

// ─── Playlist decryption ──────────────────────────────────────────────────────

var tokenRegexp = regexp.MustCompile(`[?&]_t=([^&\s]+)`)
var encryptedSegmentRegexp = regexp.MustCompile(`[?&]_c=[0-9]+`)

// decryptPlaylist inspects an M3U8 body, extracts the encrypted _t= tokens
// from segment URL lines, decrypts them using the response headers, and
// returns a valid M3U8 with real segment URLs.
//
// If the playlist does not appear to be encrypted (no _c= parameter, or
// missing required headers), the body is returned unchanged.
func decryptPlaylist(body []byte, headers playlistEncryptionHeaders) ([]byte, error) {
	lines := strings.Split(string(body), "\n")

	if !playlistHasEncryptedSegments(lines) ||
		headers.hmacKeyBase64 == "" ||
		headers.proxyAfter == "" {
		return body, nil // not encrypted, pass through
	}

	commentLines, encryptedTokens := splitPlaylistLines(lines)

	joinedTokens := strings.Join(encryptedTokens, "")
	if joinedTokens == "" {
		return body, nil
	}

	decrypted, err := decryptToken(
		joinedTokens,
		headers.hmacKeyBase64,
		headers.proxyAfter,
		headers.requestPlaylist,
		headers.cachePlaylist,
	)
	if err != nil {
		return nil, fmt.Errorf("AES-GCM decryption failed: %w", err)
	}

	result := strings.Join(commentLines, "\n") + "\n" + decrypted
	return []byte(result), nil
}

// playlistHasEncryptedSegments returns true if any segment URL line contains
// the _c= marker that signals AVS encryption.
func playlistHasEncryptedSegments(lines []string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		// First non-comment, non-empty line is a segment URL
		return encryptedSegmentRegexp.MatchString(line)
	}
	return false
}

// splitPlaylistLines separates a playlist into:
//   - commentLines: all #EXTINF / #EXT-X-* / blank lines (preserved as-is)
//   - encryptedTokens: the _t=<value> token from each segment URL line
func splitPlaylistLines(lines []string) (commentLines []string, encryptedTokens []string) {
	for _, line := range lines {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			commentLines = append(commentLines, line)
		} else {
			m := tokenRegexp.FindStringSubmatch(line)
			if m != nil {
				encryptedTokens = append(encryptedTokens, m[1])
			}
		}
	}
	return
}

// ─── Core crypto: HMAC-derived AES-256-GCM ───────────────────────────────────

// decryptToken mirrors the JS decryptToken() function exactly.
//
// Key derivation:
//  1. Decode hmacKeyBase64 (base64url) → raw key bytes.
//  2. HMAC-SHA-256( rawKey, "<requestPlaylist>:<cachePlaylist>:<proxyAfter>" )
//     → 32-byte AES key material.
//  3. IV = first 12 bytes of the original raw key bytes.
//  4. AES-256-GCM decrypt( key=step2, iv=step3, ciphertext=base64url(encryptedToken) ).
//
// Parameters mirror the JS function signature 1-to-1:
//
//	encryptedToken  – base64url ciphertext  (joined _t= tokens)
//	hmacKeyBase64   – base64url HMAC key    (X-Digest-Tag header)
//	proxyAfter      – HMAC message part     (X-Proxy-After header)
//	requestPlaylist – HMAC message part     (X-Request-Playlist header, URI-decoded)
//	cachePlaylist   – HMAC message part     (X-Cache-Playlist header)
func decryptToken(encryptedToken, hmacKeyBase64, proxyAfter, requestPlaylist, cachePlaylist string) (string, error) {
	// Step 1 – decode the HMAC key from base64url
	hmacKeyBytes, err := base64.RawURLEncoding.DecodeString(hmacKeyBase64)
	if err != nil {
		return "", fmt.Errorf("failed to decode X-Digest-Tag: %w", err)
	}

	// Step 2 – derive the AES key via HMAC-SHA-256
	//   message = "<requestPlaylist>:<cachePlaylist>:<proxyAfter>"
	message := fmt.Sprintf("%s:%s:%s", requestPlaylist, cachePlaylist, proxyAfter)
	mac := hmac.New(sha256.New, hmacKeyBytes)
	mac.Write([]byte(message))
	aesKeyBytes := mac.Sum(nil) // 32 bytes → AES-256

	// Step 3 – IV is the first 12 bytes of the original HMAC key bytes
	if len(hmacKeyBytes) < 12 {
		return "", fmt.Errorf("X-Digest-Tag too short: need at least 12 bytes, got %d", len(hmacKeyBytes))
	}
	iv := hmacKeyBytes[:12]

	// Step 4 – AES-256-GCM decrypt
	ciphertext, err := base64.RawURLEncoding.DecodeString(encryptedToken)
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted token: %w", err)
	}

	block, err := aes.NewCipher(aesKeyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCMWithNonceSize(block, 12)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("AES-GCM open failed: %w", err)
	}

	return string(plaintext), nil
}

// ─── Legacy AES-CBC decryption (kept for reference / fallback) ───────────────

// decryptVideoSourceLegacy is the original AES-CBC + deflate decryptor.
// Kept in case older playlist links still use the CBC scheme.
func decryptVideoSourceLegacy(encryptedData string) ([]byte, error) {
	key := sha256.Sum256(KEY)
	dataBytes, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("error decoding base64 data: %v", err)
	}
	if len(dataBytes) <= 16 {
		return nil, fmt.Errorf("encrypted data must have at least 16 bytes")
	}

	iv := dataBytes[:16]
	ciphertext := dataBytes[16:]

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("error creating cipher: %v", err)
	}

	cipher.NewCBCDecrypter(block, iv).CryptBlocks(ciphertext, ciphertext)

	reader := flate.NewReader(bytes.NewReader(ciphertext))
	defer reader.Close()
	return io.ReadAll(reader)
}

// ─── Download helpers ─────────────────────────────────────────────────────────

func (ex *AniVietSubExtractor) Download(e Episode, w io.Writer, callback func(progress float64)) error {
	playlist, err := ex.GetM3UPlaylist(e)
	if err != nil {
		return err
	}

	segmentURLs := extractSegmentURLs(playlist)
	if len(segmentURLs) == 0 {
		return fmt.Errorf("no segment URLs found in playlist")
	}

	downloader := newAdaptiveDownloader(ex.client, ex.domain)
	return downloader.downloadSegments(segmentURLs, w, callback)
}

func extractSegmentURLs(playlist []byte) []string {
	lines := strings.Split(string(playlist), "\n")
	urls := make([]string, 0, len(lines)/2)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "http") {
			urls = append(urls, line)
		}
	}
	return urls
}

// DownloadSegment fetches a single .ts segment, skipping the 128-byte fake
// PNG header prepended by the server to disguise segment files.
func (ex *AniVietSubExtractor) DownloadSegment(url string) ([]byte, error) {
	const fakePNGHeaderSize = 128
	const rateLimitDelay = 500 * time.Millisecond

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", ex.domain)
	req.Header.Set("User-Agent", USER_AGENT)

	r, err := ex.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	if _, err = io.CopyN(io.Discard, r.Body, fakePNGHeaderSize); err != nil {
		return nil, err
	}

	content, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	time.Sleep(rateLimitDelay)
	return content, nil
}

// ─── HTML parsing helpers ─────────────────────────────────────────────────────

func extractMovies(r io.Reader) ([]SimpleAnime, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}
	movies := []SimpleAnime{}
	doc.Find("li:not(.ss-bottom)").Each(func(i int, s *goquery.Selection) {
		title := s.Find(".ss-title").Text()
		href := s.Find(".ss-title").AttrOr("href", "")
		movies = append(movies, SimpleAnime{
			Id:    extractLargestNumber(href),
			Title: title,
			Href:  href,
		})
	})
	return movies, nil
}

func extractTrendingMovies(r io.Reader) ([]SimpleAnime, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}
	movies := []SimpleAnime{}
	doc.Find("ul.bxh-movie-phimletv li").Each(func(i int, s *goquery.Selection) {
		a := s.Find("h3.title-item a")
		title := a.Text()
		href := a.AttrOr("href", "")
		thumbnail := s.Find("a.thumb img").AttrOr("src", "")
		if title != "" && href != "" {
			movies = append(movies, SimpleAnime{
				Id:        extractLargestNumber(href),
				Title:     title,
				Href:      href,
				Thumbnail: thumbnail,
			})
		}
	})
	return movies, nil
}

func parseAnimeVietsubAnimeDetails(movieId int, r io.Reader) (*AnimeDetail, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("error loading document: %w", err)
	}

	href := doc.Find("meta[property='og:url']").First().AttrOr("content", "")

	var episodes []Episode
	episodeListTag := doc.Find("#list-server").First()
	episodeListTag.Find("li.episode>a.btn-episode").Each(func(i int, s *goquery.Selection) {
		episodes = append(episodes, Episode{
			MovieId: movieId,
			Title:   s.AttrOr("title", ""),
			Href:    s.AttrOr("href", ""),
			Hash:    s.AttrOr("data-hash", ""),
		})
	})

	articleTag := doc.Find("article.TPost")
	title := strings.TrimSpace(articleTag.Find("h1.Title").Text())
	subtitle := strings.TrimSpace(articleTag.Find("h2.SubTitle").Text())
	description := strings.TrimSpace(articleTag.Find("div.Description").Text())
	accessTime := strings.TrimSpace(articleTag.Find("span.Time").Text())

	scoreStr := strings.TrimSpace(articleTag.Find("#TPVotes").AttrOr("data-percent", "0"))
	var rating float64
	if rv, err := strconv.ParseFloat(scoreStr, 64); err == nil {
		rating = rv / 10
	}

	return &AnimeDetail{
		Id:            movieId,
		Title:         title,
		Subtitle:      subtitle,
		Description:   description,
		Rating:        rating,
		Href:          href,
		TotalEpisodes: accessTime,
		Episodes:      episodes,
	}, nil
}

func extractLargestNumber(text string) int {
	max, cur := 0, 0
	for i := 0; i < len(text); i++ {
		if '0' <= text[i] && text[i] <= '9' {
			cur = 10*cur + int(text[i]-'0')
			if cur >= max {
				max = cur
			}
		} else {
			cur = 0
		}
	}
	return max
}
