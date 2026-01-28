package extractor

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"regexp"
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
		domain: "https://animehay.bar",
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
	return nil, nil
}

func (ext *AnimeTvExtractor) GetM3UPlaylist(e Episode) ([]byte, error) {
	return nil, nil
}

func (ext *AnimeTvExtractor) Download(e Episode, w io.Writer) error {
	return nil
}

func (ext *AnimeTvExtractor) DownloadSegment(url string) ([]byte, error) {
	return nil, nil
}
