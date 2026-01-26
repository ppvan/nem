package extractor

import (
	"fmt"
	"io"
	"net/url"
	"strings"
)

type Episode struct {
	MovieId int
	Title   string
	Href    string
	Hash    string
}

type SimpleAnime struct {
	Id        int    `json:"id"`
	Title     string `json:"title"`
	Thumbnail string `json:"thumbnail"`
	Href      string `json:"href"`
}

type AnimeDetail struct {
	Id            int       `json:"id"`
	Title         string    `json:"title"`
	Subtitle      string    `json:"subtitle"`
	Description   string    `json:"description"`
	Rating        float64   `json:"rating"`
	Href          string    `json:"href"`
	TotalEpisodes string    `json:"total_episodes"`
	Episodes      []Episode `json:"episodes"`
}

func (m *AnimeDetail) String() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Id: %d\n", m.Id)
	fmt.Fprintf(&sb, "Href: %s\n", m.Href)
	fmt.Fprintf(&sb, "Title: %s\n", m.Title)
	fmt.Fprintf(&sb, "Subtitle: %s\n", m.Subtitle)
	fmt.Fprintf(&sb, "Description: %s\n", m.Description)
	fmt.Fprintf(&sb, "Rating: %.1f\n", m.Rating)
	fmt.Fprintf(&sb, "Episodes: %s\n", m.TotalEpisodes)

	return sb.String()
}

type Extractor interface {
	Search(query string) ([]SimpleAnime, error)
	GetM3UPlaylist(e Episode) ([]byte, error)
	Download(e Episode, w io.Writer) error
	DownloadSegment(url string, w io.Writer) error
	GetAnimeDetails(id int) (*AnimeDetail, error)
}

func mustJoinPath(base string, elem ...string) string {
	fullPath, err := url.JoinPath(base, elem...)
	if err != nil {
		panic(err)
	}

	return fullPath
}
