package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ppvan/nem/extractor"
)

type SearchRequest struct {
	Query string `json:"query"`
}

func (app *application) searchHandler(w http.ResponseWriter, r *http.Request) {

	var req SearchRequest
	if err := app.readJSON(r, &req); err != nil {
		app.clientError(r, w, err)
		return
	}

	movies, err := app.Extractor.Search(req.Query)
	if err != nil {
		app.serverError(r, w, err)
		return
	}

	app.writeJSON(w, http.StatusOK, movies)
}

func (app *application) indexHandler(w http.ResponseWriter, r *http.Request) {

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		app.writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	movie, err := app.Extractor.GetMovieMetadata(id)
	if err != nil {
		app.serverError(r, w, err)
		return
	}

	fmt.Fprintln(w, "#EXTM3U")
	for _, v := range movie.Episodes {
		fmt.Fprintf(w, "#EXTINF:-1,%s\n", v.Title)
		fmt.Fprintf(w, "%s/index.m3u8\n", v.Hash)
	}

}

func (app *application) episodeHandler(w http.ResponseWriter, r *http.Request) {

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		app.writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	hash := r.PathValue("hash")
	if hash == "" {
		app.writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "empty hash",
		})
		return
	}

	content, err := app.Extractor.GetM3UPlaylist(extractor.Episode{
		MovieId: id,
		Hash:    hash,
	})

	if err != nil {
		app.writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	lines := strings.Split(string(content), "\n")
	for i, v := range lines {
		if !strings.HasPrefix(v, "http") {
			continue
		}

		chunk := base64.StdEncoding.EncodeToString([]byte(v))
		lines[i] = fmt.Sprintf("/chunks/%s.ts", chunk)
	}

	playlist := strings.Join(lines, "\n")
	w.Write([]byte(playlist))
}

func (app *application) getSegment(w http.ResponseWriter, r *http.Request) {
	encodedUrl := r.PathValue("id")
	v := encodedUrl[:len(encodedUrl)-3]
	content, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
	}

	err = app.Extractor.DownloadSegment(string(content), w)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
	}
}
