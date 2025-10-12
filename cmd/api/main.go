package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ppvan/nem/extractor"
)

type application struct {
	Extractor extractor.Extractor
	Logger    *log.Logger
}

type envelope map[string]interface{}

func main() {

	logger := log.New(os.Stdout, "[nem] ", log.Flags())
	addr := ":8000"

	ex, err := extractor.NewAniVietSubExtractor(DOMAIN)
	if err != nil {
		logger.Fatal(err)
	}

	app := application{
		Extractor: ex,
		Logger:    logger,
	}

	server := http.Server{
		Addr:         addr,
		Handler:      app.routes(),
		ErrorLog:     app.Logger,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	app.Logger.Printf("Listening on %s\n", server.Addr)
	err = server.ListenAndServe()
	if err != nil {
		app.Logger.Fatal(err)
	}
}
