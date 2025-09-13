package main

import (
	"fmt"
	"os"

	"github.com/ppvan/nem/extractor"
)

func main() {

	query := "phu thuy tinh lang"
	ex, err := extractor.NewAniVietSubExtractor("https://animevietsub.cam")
	if err != nil {
		panic(err)
	}

	movies, err := ex.Search(query)
	if err != nil {
		panic(err)
	}

	fmt.Println("Result")
	for _, v := range movies {
		fmt.Println("movie", v.Id, v.Title, v.Href)
	}

	fmt.Println("Select first result")
	movie := movies[0]

	eps, err := ex.GetEpisodes(movie)
	if err != nil {
		panic(err)
	}

	for _, v := range eps {
		fmt.Println("title", v.Title, "href", v.Href, "hash", v.Hash)
	}

	fmt.Println("Select last")

	ep := eps[len(eps)-1]

	p, err := ex.GetM3UPlaylist(ep)
	if err != nil {
		panic(err)
	}

	fmt.Println("eps", ep.Title)

	file, err := os.Create("result.m3u8")
	if err != nil {
		panic(err)
	}

	n, err := file.Write(p)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	fmt.Println(n)

}
