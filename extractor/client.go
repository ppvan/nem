package extractor

import (
	"bytes"
	"fmt"
	"io"
	"math/rand/v2"
	"net/url"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const SEARCH_API = "/ajax/suggest"
const PLAYLIST_API = "/ajax/player"
const TRENDING_API = "/bang-xep-hang/season.html"
const (
	maxRetries = 3
	retryDelay = 2 * time.Second
)
const (
	chromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
	secCHUA = `"Chromium";v="133", "Not(A:Brand";v="24", "Google Chrome";v="133"`

	acceptDocument = "text/html,application/xhtml+xml,application/xml;q=0.9," +
		"image/avif,image/webp,image/apng,*/*;q=0.8"
	acceptDefault  = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	acceptLanguage = "vi-VN,vi;q=0.9,en-US;q=0.8,en;q=0.7"
	acceptEncoding = "gzip, deflate, br, zstd"
)

var (
	headerOrder = []string{
		"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
		"upgrade-insecure-requests", "user-agent", "accept",
		"x-client-env",
		"sec-fetch-site", "sec-fetch-mode", "sec-fetch-user", "sec-fetch-dest",
		"accept-encoding", "accept-language", "cookie",
	}
	pseudoHeaderOrder = []string{":method", ":authority", ":scheme", ":path"}
)

// fetchMode is the Sec-Fetch-* / Accept quartet Chrome derives from the
// request's initiator. Zero-value user means "not a user activation".
type fetchMode struct {
	accept, site, mode, dest, user string
}

var (
	modeXHR = fetchMode{acceptDefault, "same-origin", "same-origin", "empty", ""}
)

type AniVietSubExtractor struct {
	domain      string
	client      tls_client.HttpClient
	useAdaptive bool
}

func NewAniVietSubExtractor(domain string) (*AniVietSubExtractor, error) {

	jar := tls_client.NewCookieJar()
	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(),
		tls_client.WithClientProfile(profiles.Chrome_133), // use newest profile your version has
		tls_client.WithCookieJar(jar),
		tls_client.WithTimeoutSeconds(20),
		tls_client.WithRandomTLSExtensionOrder(), // Chrome 106+ shuffles per connection
	)

	if err != nil {
		return nil, err
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
	}

	// Fetch homepage to get Cloudflare cookies before any real request
	if err := ex.warmUp(); err != nil {
		return nil, fmt.Errorf("warmup failed: %w", err)
	}

	return ex, nil
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

	r, err := ex.client.Do(req)
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

func (ex *AniVietSubExtractor) GetAnimeDetailsHref(href string) (*AnimeDetail, error) {
	id := extractLargestNumber(href)
	return ex.GetAnimeDetails(id)
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

func (ex *AniVietSubExtractor) GetM3UPlaylist(e Episode) ([]byte, error) {
	rawEpisode, err := ex.fetchHtml(e.Href)
	if err != nil {
		return nil, fmt.Errorf("fetch episode: %w", err)
	}

	playerLink, err := extractPlaylistLink(rawEpisode)
	if err != nil {
		return nil, fmt.Errorf("extract playlist link: %w", err)
	}

	playerHtml, err := ex.fetchHtml(playerLink.String())
	if err != nil {
		return nil, fmt.Errorf("fetch player: %w", err)
	}

	playerData, err := extractPlayerData(playerHtml)
	if err != nil {
		return nil, fmt.Errorf("extract player data: %w", err)
	}

	origin := fmt.Sprint(playerLink.Scheme, "://", playerLink.Host)
	playlistURL := fmt.Sprintf("%s/playlist/%s/playlist.m3u8?token=%s", origin, playerData.VideoID, playerData.AVSToken)

	body, headers, err := ex.fetchPlaylist(playlistURL, playerLink.String())
	if err != nil {
		return nil, fmt.Errorf("fetch playlist: %w", err)
	}

	envelope := extractEnvelope(headers)
	playlist, err := decryptPlaylist(body, &envelope, playerData.AVSToken, origin)
	if err != nil {
		return nil, fmt.Errorf("decrypt playlist: %w", err)
	}

	return playlist, nil
}

func (ex *AniVietSubExtractor) Download(e Episode, w io.Writer, callback func(progress float64)) error {
	playlist, err := ex.GetM3UPlaylist(e)
	if err != nil {
		return err
	}
	segmentURLs := extractSegmentURLs(playlist)
	if len(segmentURLs) == 0 {
		return fmt.Errorf("no segment URLs found in playlist")
	}

	var downloader SegmentDownloader
	if ex.useAdaptive {
		downloader = newAdaptiveDownloader(ex.client, ex.domain)
	} else {
		downloader = newGreedyDownloader(ex.client, ex.domain)
	}
	return downloader.downloadSegments(segmentURLs, w, callback)
}

// DownloadSegment downloads a single HLS segment and returns its decoded
// bytes.
//
// Segments on this site are disguised as PNG files — the real .ts payload
// is appended after the PNG's IEND chunk, presumably to dodge naive
// content-type/extension filtering. extractDataAfterIEND (already defined
// elsewhere in this package) strips that wrapper back off.
//
// Retries on HTTP 429 with exponential backoff + jitter, since segment
// CDNs on this site rate-limit aggressively; any other non-200 status or
// transport error fails immediately without retrying.
func (ex *AniVietSubExtractor) DownloadSegment(url string) ([]byte, error) {
	const (
		segmentMaxRetries     = 10
		segmentInitialBackoff = 50 * time.Millisecond
		segmentMaxBackoff     = 2 * time.Second
	)

	backoff := segmentInitialBackoff

	for range segmentMaxRetries {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("build segment request: %w", err)
		}
		req.Header.Set("Referer", ex.domain)
		req.Header.Set("User-Agent", chromeUA)

		resp, err := ex.client.Do(req)
		if err != nil {
			// resp is nil on a transport error, so there's nothing to
			// close — and unlike a 429, this isn't something retrying is
			// likely to fix, so fail immediately.
			return nil, fmt.Errorf("fetch segment: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			jitter := backoff/2 + time.Duration(rand.Float64()*float64(backoff/2))
			time.Sleep(jitter)
			backoff = min(backoff*2, segmentMaxBackoff)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
		}

		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read segment body: %w", err)
		}

		data, err := extractDataAfterIEND(raw)
		if err != nil {
			return nil, fmt.Errorf("extract segment data: %w", err)
		}
		return data, nil
	}

	return nil, fmt.Errorf("max retries exceeded for URL: %s", url)
}

func (ex *AniVietSubExtractor) fetchPlaylist(playlistURL string, origin string) ([]byte, http.Header, error) {
	req, err := http.NewRequest(http.MethodGet, playlistURL, nil)
	if err != nil {
		return nil, nil, err
	}
	ex.setCommonHeaders(req)
	req.Header.Set("Referer", origin)

	resp, err := ex.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("playlist fetch status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read playlist body: %w", err)
	}

	return body, resp.Header, nil
}

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

func (ex *AniVietSubExtractor) fetchHtml(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("setup request: %w", err)
	}
	r, err := ex.doWithRetry(req)
	if err != nil {
		return "", fmt.Errorf("fetch: %w", err)
	}
	defer r.Body.Close()

	content, err := io.ReadAll(r.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	return string(content), nil
}

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

		if resp.StatusCode == http.StatusForbidden {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			fmt.Printf("403 from %s\n  server=%s cf-ray=%s cf-mitigated=%s\n  body: %.400s\n",
				req.URL, resp.Header.Get("server"), resp.Header.Get("cf-ray"),
				resp.Header.Get("cf-mitigated"), b)
		}

		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	return nil, fmt.Errorf("request failed with 403 after %d retries", maxRetries)
}

func (ex *AniVietSubExtractor) setCommonHeaders(req *http.Request) {
	ex.setHeaders(req, modeXHR)
}

func (ex *AniVietSubExtractor) setHeaders(req *http.Request, fm fetchMode) {
	h := http.Header{
		"sec-ch-ua":          {secCHUA},
		"sec-ch-ua-mobile":   {"?0"},
		"sec-ch-ua-platform": {`"Windows"`},
		"user-agent":         {chromeUA},
		"accept":             {fm.accept},
		"x-client-env":       {"de8964fbdb3b64558decc2012fc242fc"},
		"sec-fetch-site":     {fm.site},
		"sec-fetch-mode":     {fm.mode},
		"sec-fetch-dest":     {fm.dest},
		"accept-encoding":    {acceptEncoding},
		"accept-language":    {acceptLanguage},
		"referer":            {ex.domain},

		http.HeaderOrderKey:  headerOrder,
		http.PHeaderOrderKey: pseudoHeaderOrder,
	}
	if fm.user != "" {
		h["sec-fetch-user"] = []string{fm.user}
		h["upgrade-insecure-requests"] = []string{"1"}
	}
	req.Header = h
}
