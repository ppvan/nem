package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

func main() {

	cmd := &cli.Command{
		Name:  "nem",
		Usage: "Anime downloader CLI",
		Commands: []*cli.Command{
			{
				Name:      "search",
				Usage:     "Search anime by title",
				ArgsUsage: "<query>",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:    "limit",
						Aliases: []string{"l"},
						Value:   20,
						Usage:   "Max results",
					},
				},
				Action: searchAction,
			},
			{
				Name:      "details",
				Usage:     "Get anime details",
				ArgsUsage: "<id>",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "format",
						Aliases: []string{"f"},
						Value:   "text",
						Usage:   "Output format (text|json)",
					},
				},
				Action: detailsAction,
			},
			{
				Name:      "episodes",
				Usage:     "List episodes for anime",
				ArgsUsage: "<id>",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "format",
						Aliases: []string{"f"},
						Value:   "text",
						Usage:   "Output format (text|json)",
					},
				},
				Action: episodesAction,
			},
			{
				Name:      "download",
				Usage:     "Download anime episode",
				ArgsUsage: "[id]",
				Arguments: []cli.Argument{
					&cli.IntArg{
						Name: "id",
					},
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "episode",
						Aliases:  []string{"e"},
						Usage:    "Episode number or range (i.e 5, 2-11, 1-12)",
						Required: true,
					},
					&cli.StringFlag{
						Name:      "output",
						Aliases:   []string{"o"},
						Usage:     "Output directory",
						TakesFile: true,
						Required:  true,
					},
				},
				Action: downloadAction,
			},
			{
				Name:      "playlist",
				Usage:     "Get M3U8 playlist",
				ArgsUsage: "<id>",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:     "episode",
						Aliases:  []string{"e"},
						Usage:    "Episode number",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "output",
						Aliases:  []string{"o"},
						Usage:    "Output file path",
						Required: true,
					},
				},
				Action: playlistAction,
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "source",
				Aliases:     []string{"s"},
				Usage:       "Animevietsub url",
				DefaultText: "[https://animevietsub.ee]",
				Value:       "https://animevietsub.ee",
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
