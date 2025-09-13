package extractor

import (
	"bytes"
	"compress/flate"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36"
const KEY = "dm_thang_suc_vat_get_link_an_dbt"
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

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	client := http.Client{
		Timeout: 5 * time.Second,
		Jar:     jar,
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
		return nil, err
	}

	defer r.Body.Close()

	return extractMovies(r.Body)

}

func (ex *AniVietSubExtractor) GetEpisodes(m Movie) ([]Episode, error) {
	url := mustJoinPath(m.Href, "xem-phim.html")

	r, err := ex.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	return parseMovieEpisodes(m.Id, r.Body)
}

func (ex *AniVietSubExtractor) GetM3UPlaylist(e Episode) ([]byte, error) {
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

	return []byte(str), nil
}

func decryptVideoSource(encryptedData string) ([]byte, error) {
	key := sha256.Sum256([]byte(KEY))
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

	result := make([]Movie, 0, 16)

	doc.Find("li:not(.ss-bottom)").Each(func(i int, s *goquery.Selection) {
		title := s.Find(".ss-title").Text()
		href := s.Find(".ss-title").AttrOr("href", "")

		result = append(result, Movie{
			Id:    extractLargestNumber(href),
			Title: title,
			Href:  href,
		})
	})

	return result, nil
}

func parseMovieEpisodes(movieId int, r io.Reader) ([]Episode, error) {
	var episodes []Episode
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("error loading document: %w", err)
	}

	selector := ".list-episode li.episode a.btn-episode"

	doc.Find(selector).Each(func(i int, s *goquery.Selection) {
		title := s.AttrOr("title", "")
		href := s.AttrOr("href", "")
		hash := s.AttrOr("data-hash", "")
		episode := Episode{
			movieId,
			title,
			href,
			hash,
		}
		episodes = append(episodes, episode)
	})

	return episodes, nil
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
