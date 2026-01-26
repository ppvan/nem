package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/ppvan/nem/extractor"
	"github.com/urfave/cli/v3"
)

func searchAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() < 1 {
		return fmt.Errorf("missing search query")
	}

	query := cmd.Args().Get(0)
	results, err := ext.Search(query)
	if err != nil {
		return err
	}

	limit := cmd.Int("limit")
	count := len(results)
	if limit < count {
		count = limit
	}

	for i := 0; i < count; i++ {
		fmt.Printf("[%d] %s\n    %s\n", results[i].Id, results[i].Title, results[i].Href)
	}
	return nil
}

func detailsAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() < 1 {
		return fmt.Errorf("missing anime ID")
	}

	id, err := strconv.Atoi(cmd.Args().Get(0))
	if err != nil {
		return fmt.Errorf("invalid ID: %w", err)
	}

	details, err := ext.GetAnimeDetails(id)
	if err != nil {
		return err
	}

	format := cmd.String("format")
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(details)
	}

	fmt.Print(details.String())
	return nil
}

func episodesAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() < 1 {
		return fmt.Errorf("missing anime ID")
	}

	id, err := strconv.Atoi(cmd.Args().Get(0))
	if err != nil {
		return fmt.Errorf("invalid ID: %w", err)
	}

	details, err := ext.GetAnimeDetails(id)
	if err != nil {
		return err
	}

	format := cmd.String("format")
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(details.Episodes)
	}

	for i, ep := range details.Episodes {
		fmt.Printf("[%d] %s\n", i+1, ep.Title)
	}
	return nil
}

func downloadAction(ctx context.Context, cmd *cli.Command) error {
	output := cmd.String("output")
	hash := cmd.String("hash")

	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()

	var episode extractor.Episode

	if hash != "" {
		episode.Hash = hash
	} else {
		if cmd.NArg() < 1 {
			return fmt.Errorf("missing anime ID")
		}

		id, err := strconv.Atoi(cmd.Args().Get(0))
		if err != nil {
			return fmt.Errorf("invalid ID: %w", err)
		}

		episodeNum := cmd.Int("episode")
		if episodeNum == 0 {
			return fmt.Errorf("--episode flag is required when not using --hash")
		}

		details, err := ext.GetAnimeDetails(id)
		if err != nil {
			return err
		}

		if episodeNum < 1 || episodeNum > len(details.Episodes) {
			return fmt.Errorf("invalid episode number: %d (available: 1-%d)", episodeNum, len(details.Episodes))
		}

		episode = details.Episodes[episodeNum-1]
	}

	if cmd.Bool("verbose") {
		fmt.Printf("Downloading to %s...\n", output)
	}

	return ext.Download(episode, file)
}

func playlistAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() < 1 {
		return fmt.Errorf("missing anime ID")
	}

	id, err := strconv.Atoi(cmd.Args().Get(0))
	if err != nil {
		return fmt.Errorf("invalid ID: %w", err)
	}

	episodeNum := cmd.Int("episode")
	output := cmd.String("output")

	details, err := ext.GetAnimeDetails(id)
	if err != nil {
		return err
	}

	if episodeNum < 1 || episodeNum > len(details.Episodes) {
		return fmt.Errorf("invalid episode number: %d (available: 1-%d)", episodeNum, len(details.Episodes))
	}

	episode := details.Episodes[episodeNum-1]
	playlist, err := ext.GetM3UPlaylist(episode)
	if err != nil {
		return err
	}

	return os.WriteFile(output, playlist, 0644)
}
