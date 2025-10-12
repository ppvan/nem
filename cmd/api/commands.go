package main

import (
	"fmt"
	"os"

	"github.com/ppvan/nem/extractor"
)

const DOMAIN = "https://animevietsub.show"

func searchCommand(query string) error {
	ex, err := extractor.NewAniVietSubExtractor(DOMAIN)
	if err != nil {
		return fmt.Errorf("unable to init the extractor: %s", err)
	}

	result, err := ex.Search(query)
	if err != nil {
		return fmt.Errorf("unable to search: %s", err)
	}

	for _, v := range result {
		fmt.Printf("%d - %s\n", v.Id, v.Title)
	}

	return nil
}

func infoCommand(movieId int) error {
	ex, err := extractor.NewAniVietSubExtractor(DOMAIN)
	if err != nil {
		return fmt.Errorf("unable to init the extractor: %s", err)
	}

	movie, err := ex.GetMovieMetadata(movieId)
	if err != nil {
		return err
	}

	fmt.Println(movie)

	return nil
}

func downloadCommand(movieId int, episodeId int) error {
	ex, err := extractor.NewAniVietSubExtractor(DOMAIN)
	if err != nil {
		return fmt.Errorf("unable to init the extractor: %s", err)
	}

	movie, err := ex.GetMovieMetadata(movieId)
	if err != nil {
		return err
	}

	if episodeId == 0 {
		episodeId = len(movie.Episodes)
	}

	if episodeId > len(movie.Episodes) || episodeId <= 0 {
		return fmt.Errorf("no episode %d, found %s", episodeId, movie.TotalEpisodes)
	}

	episode := movie.Episodes[episodeId-1]

	return ex.Download(episode, os.Stdout)
}

func playCommand(movieId int, episodeId int) error {
	ex, err := extractor.NewAniVietSubExtractor(DOMAIN)
	if err != nil {
		return fmt.Errorf("unable to init the extractor: %s", err)
	}

	movie, err := ex.GetMovieMetadata(movieId)
	if err != nil {
		return err
	}

	if episodeId == 0 {
		episodeId = len(movie.Episodes)
	}

	if episodeId > len(movie.Episodes) || episodeId <= 0 {
		return fmt.Errorf("no episode %d, found %s", episodeId, movie.TotalEpisodes)
	}

	episode := movie.Episodes[episodeId-1]

	return ex.Play(episode, os.Stdout)
}
