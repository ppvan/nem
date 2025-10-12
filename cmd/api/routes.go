package main

import "net/http"

func (app *application) routes() http.Handler {
	router := http.NewServeMux()

	router.HandleFunc("POST /search/", app.searchHandler)
	router.HandleFunc("GET /movies/{id}/index.m3u8", app.indexHandler)
	router.HandleFunc("GET /movies/{id}/{hash}/index.m3u8", app.episodeHandler)

	router.HandleFunc("GET /chunks/{id}", app.getSegment)

	return app.logRequest(router)
}
