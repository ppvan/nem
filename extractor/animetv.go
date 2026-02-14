package extractor

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type AnimeTvExtractor struct {
	domain string
	client *http.Client
}

func NewAnimeTvExtractor() (*AnimeTvExtractor, error) {
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

	return &AnimeTvExtractor{
		client: &client,
		domain: "https://animehay.icu",
	}, nil
}

func (ext *AnimeTvExtractor) Search(query string) ([]SimpleAnime, error) {
	base := "/tim-kiem"
	re := regexp.MustCompile(`\s+`)
	processed := re.ReplaceAllString(query, "-")
	url := mustJoinPath(ext.domain, base, processed+".html")
	r, err := ext.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("network error: %s", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error: %s", r.Status)
	}
	doc, err := goquery.NewDocumentFromReader(r.Body)
	if err != nil {
		return nil, err
	}

	var animes []SimpleAnime
	doc.Find(".movie-item").Each(func(i int, s *goquery.Selection) {
		// Extract ID from the movie-item id attribute
		idAttr, _ := s.Attr("id")
		id := 0
		if idAttr != "" {
			// Extract number from "movie-id-3622"
			fmt.Sscanf(idAttr, "movie-id-%d", &id)
		}

		// Find the link with title attribute
		link := s.Find("a[title]")
		title, _ := link.Attr("title")
		href, _ := link.Attr("href")

		// Extract thumbnail
		thumbnail, _ := s.Find("img").Attr("src")

		animes = append(animes, SimpleAnime{
			Id:        id,
			Title:     title,
			Thumbnail: thumbnail,
			Href:      href,
		})
	})

	return animes, nil
}

func (ext *AnimeTvExtractor) GetAnimeDetails(id int) (*AnimeDetail, error) {
	url := mustJoinPath(ext.domain, "thong-tin-phim", fmt.Sprintf("-%d.html", id))

	r, err := ext.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	return parseAnimeTvAnimeDetails(id, r.Body)

}

func (ext *AnimeTvExtractor) GetM3UPlaylist(e Episode) ([]byte, error) {
	playlistRegex := regexp.MustCompile(`https:(?P<playlist>[a-zA-Z0-9\.\/:]+\.m3u8)`)
	url := e.Href

	r, err := ext.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	content, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	playlistHref := playlistRegex.FindString(string(content))
	r, err = ext.client.Get(playlistHref)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	content, err = io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	return content, nil
}

func (ext *AnimeTvExtractor) Download(e Episode, w io.Writer) error {

	playlist, err := ext.GetM3UPlaylist(e)
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

		segments, err := ext.DownloadSegment(v)
		if err != nil {
			return err
		}

		_, err = w.Write(segments)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ext *AnimeTvExtractor) DownloadSegment(url string) ([]byte, error) {

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", ext.domain)
	req.Header.Set("User-Agent", USER_AGENT)
	r, err := ext.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	rawContent, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	content, err := extractDataAfterIEND(rawContent)
	if err != nil {
		return nil, err
	}

	return content, nil
}

func parseAnimeTvAnimeDetails(animeId int, r io.Reader) (*AnimeDetail, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("error loading document: %w", err)
	}

	href := doc.Find("meta[property='og:url']").First().AttrOr("content", "")

	var episodes []Episode
	episodeListTag := doc.Find(".list-item-episode")

	episodeListTag.Find("a").Each(func(i int, s *goquery.Selection) {
		title := s.AttrOr("title", "")
		episodeHref := s.AttrOr("href", "")

		episode := Episode{
			MovieId: animeId,
			Title:   title,
			Href:    episodeHref,
			Hash:    "", // Hash not present in this HTML structure
		}
		episodes = append(episodes, episode)
	})

	for i, j := 0, len(episodes)-1; i < j; i, j = i+1, j-1 {
		episodes[i], episodes[j] = episodes[j], episodes[i]
	}

	infoSection := doc.Find(".info-movie")
	title := strings.TrimSpace(infoSection.Find("h1.heading_movie").Text())
	description := strings.TrimSpace(infoSection.Find(".desc p").Text())
	scoreText := strings.TrimSpace(infoSection.Find(".score div").Last().Text())
	var rating float64
	if len(scoreText) > 0 {
		parts := strings.Split(scoreText, "||")
		if len(parts) > 0 {
			ratingStr := strings.TrimSpace(parts[0])
			if r, err := strconv.ParseFloat(ratingStr, 64); err == nil {
				rating = r
			}
		}
	}

	totalEpisodes := strings.TrimSpace(infoSection.Find(".duration div").Last().Text())

	movie := &AnimeDetail{
		Id:            animeId,
		Title:         title,
		Subtitle:      title,
		Description:   description,
		Rating:        rating,
		Href:          href,
		TotalEpisodes: totalEpisodes,
		Episodes:      episodes,
	}

	return movie, nil
}
