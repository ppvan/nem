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

type Movie struct {
	Id            int
	Title         string
	Subtitle      string
	Description   string
	Rating        float64
	Href          string
	TotalEpisodes string
	Episodes      []Episode
}

func (m *Movie) String() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Id: %d\n", m.Id))
	sb.WriteString(fmt.Sprintf("Href: %s\n", m.Href))
	sb.WriteString(fmt.Sprintf("Title: %s\n", m.Title))
	sb.WriteString(fmt.Sprintf("Subtitle: %s\n", m.Subtitle))
	sb.WriteString(fmt.Sprintf("Description: %s\n", m.Description))
	sb.WriteString(fmt.Sprintf("Rating: %.1f\n", m.Rating))
	sb.WriteString(fmt.Sprintf("Episodes: %s\n", m.TotalEpisodes))

	return sb.String()
}

type Extractor interface {
	Search(query string) ([]Movie, error)
	GetEpisodes(m Movie) ([]Episode, error)
	GetM3UPlaylist(e Episode) ([]byte, error)
	Download(e Episode) (io.Reader, error)
}

func mustJoinPath(base string, elem ...string) string {
	fullPath, err := url.JoinPath(base, elem...)
	if err != nil {
		panic(err)
	}

	return fullPath
}
