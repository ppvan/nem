package main

import (
	"context"
	"log"
	"os"

	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:                  "nem",
		Usage:                 "download anime from animevietsub",
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			{
				Name:  "search",
				Usage: "search for anime by title",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "title",
						UsageText: "the title to search",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					query := cmd.StringArg("title")

					return searchHandler(query)
				},
			},
			{
				Name:  "info",
				Usage: "show anime metadata",
				Arguments: []cli.Argument{
					&cli.IntArg{
						Name:      "id",
						UsageText: "the anime id from search command",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					id := cmd.IntArg("id")

					return infoHandler(id)
				},
			},
			{
				Name:  "download",
				Usage: "download anime video to stdout",
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

					return downloadHandler(id, episode)
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}

}
