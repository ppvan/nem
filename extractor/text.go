package extractor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func extractPlaylistLink(htmlContent string) (*url.URL, error) {
	playerLinkRe := regexp.MustCompile(`PLAYER_DATA.+("link":)"(https[^"]+)"`)
	match := playerLinkRe.FindStringSubmatch(htmlContent)
	if match == nil {
		return nil, fmt.Errorf("no match found")
	}
	rawLink := strings.ReplaceAll(match[2], `\/`, `/`)

	playerLink, err := url.Parse(rawLink)
	if err != nil {
		return nil, fmt.Errorf("invalid player url: %w", err)
	}

	return playerLink, nil
}

type PlayerData struct {
	VideoID  string
	AVSToken string
	Host     string
}

func extractPlayerData(playerHTML string) (*PlayerData, error) {
	idRegex := regexp.MustCompile(`const id\s*=\s*"([^"]+)"`)
	tokenRegex := regexp.MustCompile(`const avsToken\s*=\s*"([^"]+)"`)

	idMatch := idRegex.FindStringSubmatch(playerHTML)
	if idMatch == nil {
		return nil, fmt.Errorf("failed to extract id from player page")
	}

	tokenMatch := tokenRegex.FindStringSubmatch(playerHTML)
	if tokenMatch == nil {
		return nil, fmt.Errorf("failed to extract avsToken from player page")
	}

	return &PlayerData{
		VideoID:  idMatch[1],
		AVSToken: tokenMatch[1],
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

func mustJoinPath(base string, elem ...string) string {
	fullPath, err := url.JoinPath(base, elem...)
	if err != nil {
		panic(err)
	}

	return fullPath
}

func extractDataAfterIEND(raw []byte) ([]byte, error) {

	pngSignature := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	if len(raw) < len(pngSignature) {
		return nil, errors.New("not a valid PNG file (missing PNG signature)")
	}

	pos := len(pngSignature)

	for pos < len(raw) {
		if pos+8 > len(raw) {
			return nil, errors.New("incomplete chunk header")
		}

		chunkLength := binary.BigEndian.Uint32(raw[pos : pos+4])

		chunkType := raw[pos+4 : pos+8]

		chunkSize := 4 + 4 + int(chunkLength) + 4

		if bytes.Equal(chunkType, []byte("IEND")) {
			iendEnd := pos + chunkSize

			if iendEnd >= len(raw) {
				return []byte{}, nil
			}

			return raw[iendEnd:], nil
		}

		pos += chunkSize
	}

	return nil, errors.New("IEND chunk not found in PNG file")
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
	doc.Find("li").Each(func(i int, s *goquery.Selection) {
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
			Id:      s.AttrOr("data-id", ""),
			Title:   s.AttrOr("title", ""),
			Href:    s.AttrOr("href", ""),
			Hash:    s.AttrOr("data-hash", ""),
		})
	})
	slices.SortFunc(episodes, func(e1, e2 Episode) int {
		id1, err1 := strconv.ParseInt(e1.Id, 10, 64)
		id2, err2 := strconv.ParseInt(e2.Id, 10, 64)

		if err1 != nil || err2 != nil {
			return 0
		}

		switch {
		case id1 < id2:
			return -1
		case id1 > id2:
			return 1
		default:
			return 0
		}
	})

	articleTag := doc.Find("article.TPost")
	title := strings.TrimSpace(articleTag.Find("h1.Title").Text())
	subtitle := strings.TrimSpace(articleTag.Find("h2.SubTitle").Text())
	description := strings.TrimSpace(articleTag.Find("div.Description").Text())
	accessTime := strings.TrimSpace(articleTag.Find("span.Time").Text())
	views := strings.TrimSpace(strings.SplitN(articleTag.Find("span.View").Text(), " ", 2)[0])
	thumbnail := strings.TrimSpace(articleTag.Find("div.Image img").AttrOr("src", "N/A"))

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
		Thumbnail:     thumbnail,
		Rating:        rating,
		Href:          href,
		TotalEpisodes: accessTime,
		Episodes:      episodes,
		Views:         views,
	}, nil
}
