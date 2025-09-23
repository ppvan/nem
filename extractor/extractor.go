package extractor

import (
	"io"
	"net/url"
)

type Episode struct {
	MovieId int
	Title   string
	Href    string
	Hash    string
}

type Movie struct {
	Id    int
	Title string
	Href  string
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
