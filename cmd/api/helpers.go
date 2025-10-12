package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (app *application) clientError(r *http.Request, w http.ResponseWriter, err error) {

	app.writeJSON(w, http.StatusBadRequest, envelope{"error": err.Error()})

}

func (app *application) serverError(r *http.Request, w http.ResponseWriter, err error) {

	app.writeJSON(w, http.StatusBadRequest, envelope{"error": err.Error()})

}

func (app *application) readJSON(r *http.Request, dst interface{}) error {
	// Limit the request body size to prevent abuse (e.g., 1MB)
	maxBytes := 1_048_576
	r.Body = http.MaxBytesReader(nil, r.Body, int64(maxBytes))

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(dst)
	if err != nil {
		return fmt.Errorf("failed to decode JSON: %w", err)
	}

	// Check if there's more data after the first JSON object
	if dec.More() {
		return fmt.Errorf("body must only contain a single JSON object")
	}

	return nil
}

func (app *application) writeJSON(w http.ResponseWriter, status int, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ") // Optional: pretty print

	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}
