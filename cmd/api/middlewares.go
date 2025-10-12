package main

import "net/http"

func (app *application) logRequest(handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		app.Logger.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL.Path)
		handler.ServeHTTP(w, r)
	}
}
