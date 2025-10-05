package extractor

import (
	"bytes"
	"compress/flate"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var KEY = []byte{100, 109, 95, 116, 104, 97, 110, 103, 95, 115, 117, 99, 95, 118, 97, 116, 95, 103, 101, 116, 95, 108, 105, 110, 107, 95, 97, 110, 95, 100, 98, 116}

const USER_AGENT = "Mozilla/5.0 (iPhone; CPU iPhone OS 16_1_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) GSA/383.0.797833943 Mobile/15E148 Safari/604.1"
const SEARCH_API = "/ajax/suggest"
const PLAYLIST_API = "/ajax/player"

type AniVietSubExtractor struct {
	domain string
	client *http.Client
}

type EncryptedPlaylist struct {
	Success int       `json:"success"`
	Title   string    `json:"title"`
	Link    []LinkObj `json:"link"`
}

type LinkObj struct {
	File string `json:"file"`
}

func NewAniVietSubExtractor(base string) (*AniVietSubExtractor, error) {

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: false,
		// Mimic Chrome's cipher suites
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

	// Create custom transport with TLS config
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	client := http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
	}

	return &AniVietSubExtractor{
		client: &client,
		domain: base,
	}, nil
}

func (ex *AniVietSubExtractor) Search(query string) ([]Movie, error) {
	api := mustJoinPath(ex.domain, SEARCH_API)
	body := url.Values{
		"ajaxSearch": {"1"},
		"keysearch":  {query},
	}
	r, err := ex.client.PostForm(api, body)
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

func (ex *AniVietSubExtractor) Get(id int) (*Movie, error) {
	url := mustJoinPath(ex.domain, "phim", fmt.Sprintf("-%d", id), "xem-phim.html")

	r, err := ex.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	return parseMovieMetadata(id, r.Body)
}

func (ex *AniVietSubExtractor) Download(e Episode, w io.Writer) error {

	playlist, err := ex.getM3UPlaylist(e)
	if err != nil {
		return err
	}

	const FAKE_PNG_HEADER_TO_SKIP = 128
	const RATELIMIT_DELAY = 500 * time.Millisecond

	lines := strings.SplitSeq(string(playlist), "\n")
	for v := range lines {
		if !strings.HasPrefix(v, "http") {
			continue
		}

		req, err := http.NewRequest(http.MethodGet, v, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Referer", ex.domain)
		req.Header.Set("User-Agent", USER_AGENT)
		r, err := ex.client.Do(req)
		if err != nil {
			return err
		}
		defer r.Body.Close()

		_, err = io.CopyN(io.Discard, r.Body, FAKE_PNG_HEADER_TO_SKIP)
		if err != nil {
			return err
		}

		_, err = io.Copy(w, r.Body)
		if err != nil {
			return err
		}

		time.Sleep(RATELIMIT_DELAY)
	}

	return nil
}

func (ex *AniVietSubExtractor) getM3UPlaylist(e Episode) ([]byte, error) {
	apiUrl := mustJoinPath(ex.domain, PLAYLIST_API)

	payload := url.Values{
		"link": {e.Hash},
		"id":   {strconv.Itoa(e.MovieId)},
	}
	body := strings.NewReader(payload.Encode())
	req, err := http.NewRequest(http.MethodPost, apiUrl, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", USER_AGENT)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r, err := ex.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	var playlist EncryptedPlaylist

	decoder := json.NewDecoder(r.Body)

	err = decoder.Decode(&playlist)
	if err != nil {
		return nil, err
	}

	file := playlist.Link[0].File

	content, err := decryptVideoSource(file)
	if err != nil {
		return nil, err
	}

	str, err := strconv.Unquote(string(content))
	if err != nil {
		return nil, err
	}

	return bytes.NewBufferString(str).Bytes(), nil
}

func decryptVideoSource(encryptedData string) ([]byte, error) {
	key := sha256.Sum256(KEY)
	dataBytes, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("error decoding base64 data: %v", err)
	}

	iv := dataBytes[:16]
	ciphertext := dataBytes[16:]

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("error creating cipher: %v", err)
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, ciphertext)

	reader := flate.NewReader(bytes.NewReader(ciphertext))
	defer reader.Close()

	return io.ReadAll(reader)
}

func extractMovies(r io.Reader) ([]Movie, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}

	var movies []Movie
	doc.Find("li:not(.ss-bottom)").Each(func(i int, s *goquery.Selection) {
		title := s.Find(".ss-title").Text()
		href := s.Find(".ss-title").AttrOr("href", "")

		movies = append(movies, Movie{
			Id:    extractLargestNumber(href),
			Title: title,
			Href:  href,
		})
	})

	return movies, nil
}

func parseMovieMetadata(movieId int, r io.Reader) (*Movie, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("error loading document: %w", err)
	}

	title := strings.TrimSpace(doc.Find("article header h1.Title").Text())
	subtitle := strings.TrimSpace(doc.Find("article header h2.SubTitle").Text())
	description := strings.TrimSpace(doc.Find("article header .Description").Text())
	accessTime := strings.TrimSpace(doc.Find("article footer p.Info span.Time").Text())

	var rating float64
	if scoreStr := doc.Find("#score_current").AttrOr("value", "0"); scoreStr != "" {
		if r, err := strconv.ParseFloat(scoreStr, 64); err == nil {
			rating = r
		}
	}

	var href string
	if metaTag := doc.Find("meta[property='og:url']"); metaTag.Length() > 0 {
		href, _ = metaTag.Attr("content")
	}

	var episodes []Episode
	selector := ".list-episode li.episode a.btn-episode"
	doc.Find(selector).Each(func(i int, s *goquery.Selection) {
		title := s.AttrOr("title", "")
		href := s.AttrOr("href", "")
		hash := s.AttrOr("data-hash", "")
		episode := Episode{
			MovieId: movieId,
			Title:   title,
			Href:    href,
			Hash:    hash,
		}
		episodes = append(episodes, episode)
	})

	movie := &Movie{
		Id:            movieId,
		Title:         title,
		Subtitle:      subtitle,
		Description:   description,
		Rating:        rating,
		Href:          href,
		TotalEpisodes: accessTime,
		Episodes:      episodes,
	}

	return movie, nil
}

func extractLargestNumber(text string) int {
	max := 0
	cur := 0
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
