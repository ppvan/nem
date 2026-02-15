package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/urfave/cli/v3"
)

var rangeRegex = regexp.MustCompile(`^(?P<start>\d+)(?:-(?P<end>\d+))?$`)

type ProgressWriter struct {
	Writer     io.Writer
	Downloaded int64
	StartTime  time.Time
	OnProgress func(downloaded int64, speed float64)
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.Writer.Write(p)
	pw.Downloaded += int64(n)

	if pw.OnProgress != nil {
		elapsed := time.Since(pw.StartTime).Seconds()
		speed := float64(pw.Downloaded) / elapsed
		pw.OnProgress(pw.Downloaded, speed)
	}

	return n, err
}

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
	if cmd.NArg() < 1 {
		return fmt.Errorf("missing anime ID")
	}

	id, err := strconv.Atoi(cmd.Args().Get(0))
	if err != nil {
		return fmt.Errorf("invalid ID: %w", err)
	}

	episodeValue := cmd.String("episode")
	s, e, err := parseRange(episodeValue)

	if err != nil {
		return fmt.Errorf("--episode flag format invalid")
	}

	output := cmd.String("output")
	info, err := os.Stat(output)
	if errors.Is(err, os.ErrNotExist) || !info.IsDir() {
		return fmt.Errorf("directory '%s' does not exist.", output)
	}

	if !info.IsDir() {
		return fmt.Errorf("directory '%s' is not a directory.", output)
	}

	details, err := ext.GetAnimeDetails(id)
	if err != nil {
		return err
	}

	if s < 1 || e > len(details.Episodes) {
		return fmt.Errorf("invalid episode number: %s (available: 1-%d)", episodeValue, len(details.Episodes))
	}

	for _, episode := range details.Episodes[s-1 : e] {
		filename := fmt.Sprint(episode.Title, ".ts")
		episodeFilePath := filepath.Join(output, filename)

		file, err := os.Create(episodeFilePath)
		if err != nil {
			return err
		}
		defer file.Close()

		fmt.Printf("start download %s\n", episodeFilePath)

		pw := &ProgressWriter{
			Writer:    file,
			StartTime: time.Now(),
			OnProgress: func(downloaded int64, speed float64) {
				fmt.Printf("\r(%.2f MB/s)", speed/1024/1024)
			},
		}

		err = ext.Download(episode, pw)
		if err != nil {
			fmt.Printf("%s download error: %s\n", episodeFilePath, err)
		} else {
			fmt.Printf("%s downloaded sucessfully\n", episodeFilePath)
		}
	}

	return nil
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

func parseRange(input string) (int, int, error) {
	matches := rangeRegex.FindStringSubmatch(input)
	if matches == nil {
		return 0, 0, errors.New("invalid format: must be 'N' or 'N-M'")
	}

	startStr := matches[rangeRegex.SubexpIndex("start")]
	endStr := matches[rangeRegex.SubexpIndex("end")]

	start, err := parsePositiveInt32(startStr)
	if err != nil {
		return 0, 0, fmt.Errorf("start value error: %v", err)
	}

	// If end is empty (single number case), end = start
	if endStr == "" {
		return start, start, nil
	}

	end, err := parsePositiveInt32(endStr)
	if err != nil {
		return 0, 0, fmt.Errorf("end value error: %v", err)
	}

	return start, end, nil
}

func parsePositiveInt32(s string) (int, error) {
	val, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, errors.New("number out of range or invalid")
	}
	if val <= 0 {
		return 0, errors.New("number must be positive (> 0)")
	}
	return int(val), nil
}
