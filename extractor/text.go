package extractor

import (
	"bytes"
	"encoding/binary"
	"errors"
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

func extractLargestNumber(text string) int {
	max, cur := 0, 0
	for i := 0; i < len(text); i++ {
		if '0' <= text[i] && text[i] <= '9' {
			cur = 10*cur + int(text[i]-'0')
			if cur >= max {
				max = cur
			}
		} else {
			cur = 0
		}
	}
	return max
}

func mustJoinPath(base string, elem ...string) string {
	fullPath, err := url.JoinPath(base, elem...)
	if err != nil {
		panic(err)
	}

	return fullPath
}

func extractDataAfterIEND(raw []byte) ([]byte, error) {
	// PNG signature
	pngSignature := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	// Verify PNG signature
	if len(raw) < len(pngSignature) {
		return nil, errors.New("not a valid PNG file (missing PNG signature)")
	}

	pos := len(pngSignature)

	for pos < len(raw) {
		if pos+8 > len(raw) {
			return nil, errors.New("incomplete chunk header")
		}

		chunkLength := binary.BigEndian.Uint32(raw[pos : pos+4])

		chunkType := raw[pos+4 : pos+8]

		chunkSize := 4 + 4 + int(chunkLength) + 4

		if bytes.Equal(chunkType, []byte("IEND")) {
			iendEnd := pos + chunkSize

			if iendEnd >= len(raw) {
				return []byte{}, nil
			}

			return raw[iendEnd:], nil
		}

		pos += chunkSize
	}

	return nil, errors.New("IEND chunk not found in PNG file")
}
