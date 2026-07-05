package extractor

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
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
const MEDIA_HOST = ""

const (
	maxRetries = 3
	retryDelay = 2 * time.Second
)

type AniVietSubExtractor struct {
	domain      string
	client      *http.Client
	jar         *cookiejar.Jar
	useAdaptive bool
}

type EncryptedPlaylist struct {
	Success int    `json:"success"`
	Title   string `json:"title"`
	Link    string `json:"link"`
}

type LinkObj struct {
	File string `json:"file"`
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

	// Fetch homepage to get Cloudflare cookies before any real request
	if err := ex.warmUp(); err != nil {
		return nil, fmt.Errorf("warmup failed: %w", err)
	}

	return ex, nil
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

func (ex *AniVietSubExtractor) setCommonHeaders(req *http.Request) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", USER_AGENT)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "same-origin")
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

	body, headers, err := ex.fetchPlaylist(playlistURL)
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

func (ex *AniVietSubExtractor) fetchPlaylist(playlistURL string) ([]byte, http.Header, error) {
	req, err := http.NewRequest(http.MethodGet, playlistURL, nil)
	if err != nil {
		return nil, nil, err
	}
	ex.setCommonHeaders(req)

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
