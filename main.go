package main

import (
	"context"
	"log"
	"os"

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

					return searchHandler(query)
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

					return infoHandler(id)
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

					return downloadHandler(id, episode)
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}

}
