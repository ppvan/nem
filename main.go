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

	cmd := &cli.Command{
		Name:        "nem",
		Description: "download anime from animevietsub",
		Commands: []*cli.Command{
			{
				Name: "search",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "title",
						UsageText: "the title to search",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					query := cmd.StringArg("title")

					return search(query)
				},
			},
			{
				Name: "info",
				Arguments: []cli.Argument{
					&cli.IntArg{
						Name:      "id",
						UsageText: "the anime id from search command",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					id := cmd.IntArg("id")

					return info(id)
				},
			},
			{
				Name: "download",
				Arguments: []cli.Argument{
					&cli.IntArg{
						Name:      "id",
						UsageText: "the anime id from search command",
					},
				},
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:    "episode",
						Aliases: []string{"e"},
						Value:   0,
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					id := cmd.IntArg("id")
					episode := cmd.Int("episode")

					return download(id, episode)
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}

}

func search(query string) error {
	ex, err := extractor.NewAniVietSubExtractor("https://animevietsub.show")
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

func info(movieId int) error {
	ex, err := extractor.NewAniVietSubExtractor("https://animevietsub.show")
	if err != nil {
		return fmt.Errorf("unable to init the extractor: %s", err)
	}

	movie, err := ex.Get(movieId)
	if err != nil {
		return err
	}

	fmt.Println(movie)

	return nil
}

func download(movieId int, episodeId int) error {
	ex, err := extractor.NewAniVietSubExtractor("https://animevietsub.show")
	if err != nil {
		return fmt.Errorf("unable to init the extractor: %s", err)
	}

	movie, err := ex.Get(movieId)
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
