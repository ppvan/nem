package main

import (
	"fmt"

	"github.com/ppvan/nem/extractor"
)

const HOST = "https://stream.googleapiscdn.com"

// func main() {
// 	rawPlaylistBytes, err := os.ReadFile(`C:\Users\phucl\Downloads\response.txt`)
// 	if err != nil {
// 		panic(err)
// 	}

// 	expectedBytes, err := os.ReadFile(`C:\Users\phucl\Downloads\decrypted.m3u8`)
// 	if err != nil {
// 		panic(err)
// 	}

// 	headersBytes, err := os.ReadFile(`C:\Users\phucl\Downloads\headers.json`)
// 	if err != nil {
// 		panic(err)
// 	}

// 	tokenBytes, err := os.ReadFile(`C:\Users\phucl\Downloads\token.txt`)
// 	if err != nil {
// 		panic(err)
// 	}

// 	rawPlaylist := string(rawPlaylistBytes)
// 	expected := string(expectedBytes)
// 	_ = expected // remove if unused

// 	token := string(tokenBytes)

// 	var headers map[string]string
// 	if err := json.Unmarshal(headersBytes, &headers); err != nil {
// 		panic(err)
// 	}

// 	envelope := extractor.ExtractEnvelope(headers)

// 	actual, err := extractor.DecryptPlaylist([]byte(rawPlaylist), &envelope, token, HOST)
// 	if err != nil {
// 		panic(err)
// 	}

// 	if err := os.WriteFile("./final.m3u8", []byte(actual), 0644); err != nil {
// 		panic(err)
// 	}

// 	fmt.Println("Written final.m3u8")
// }

func main() {
	ex, _ := extractor.NewAniVietSubExtractor("https://animevietsub.pl")

	ep := extractor.Episode{
		MovieId: 5103,
		Title:   "T 1",
		Href:    "https://animevietsub.pl/phim/otonari-no-tenshi-sama-ni-itsunomanika-dame-ningen-ni-sareteita-ken-2nd-season-a5103/tap-01-112692.html",
		Hash:    "HI",
	}

	playlist, err := ex.GetM3UPlaylist(ep)

	if err != nil {
		panic(err)
	}

	fmt.Println(string(playlist))

}
