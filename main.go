package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ppvan/nem/extractor"
	"github.com/urfave/cli/v3"
)

func main() {

	// ./nem -s "Title" -e 1 --latest
	cmd := &cli.Command{
		Name:        "nem",
		Description: "download anime from animevietsub",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "search",
				UsageText: "the anime title to search",
			},
		},
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "episode",
				Value: 0,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			query := cmd.StringArg("search")
			episode := cmd.Int("episode")

			return search(query, episode)
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}

}

func search(query string, episode int) error {
	ex, err := extractor.NewAniVietSubExtractor("https://animevietsub.show")
	if err != nil {
		return fmt.Errorf("unable to init the extractor: %s", err)
	}

	result, err := ex.Search(query)
	if err != nil {
		return fmt.Errorf("unable to search: %s", err)
	}

	if len(result) > 1 {
		for _, v := range result {
			fmt.Println(v.Title)
		}
		return nil
	}

	movie := result[0]
	episodes, err := ex.GetEpisodes(movie)
	if err != nil {
		return err
	}

	targetEpisode := episodes[episode]

	os.Mkdir(movie.Title, 0700)
	path := fmt.Sprintf("%v/%v", movie.Title, targetEpisode.Title)
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("unable to create file: %s", err)
	}
	defer file.Close()

	fmt.Printf("Download %v-%v to %v", movie.Title, targetEpisode.Title, path)
	return ex.Download(targetEpisode, file)

}
