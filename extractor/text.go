package extractor

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

func extractPlaylistLink(htmlContent string) (*url.URL, error) {
	playerLinkRe := regexp.MustCompile(`PLAYER_DATA.+("link":)"(https[^"]+)"`)
	match := playerLinkRe.FindStringSubmatch(htmlContent)
	if match == nil {
		return nil, fmt.Errorf("no match found")
	}
	rawLink := strings.ReplaceAll(match[2], `\/`, `/`)

	playerLink, err := url.Parse(rawLink)
	if err != nil {
		return nil, fmt.Errorf("invalid player url: %w", err)
	}

	return playerLink, nil
}

type PlayerData struct {
	VideoID  string
	AVSToken string
	Host     string
}

func extractPlayerData(playerHTML string) (*PlayerData, error) {
	idRegex := regexp.MustCompile(`const id\s*=\s*"([^"]+)"`)
	tokenRegex := regexp.MustCompile(`const avsToken\s*=\s*"([^"]+)"`)

	idMatch := idRegex.FindStringSubmatch(playerHTML)
	if idMatch == nil {
		return nil, fmt.Errorf("failed to extract id from player page")
	}

	tokenMatch := tokenRegex.FindStringSubmatch(playerHTML)
	if tokenMatch == nil {
		return nil, fmt.Errorf("failed to extract avsToken from player page")
	}

	return &PlayerData{
		VideoID:  idMatch[1],
		AVSToken: tokenMatch[1],
	}, nil
}
