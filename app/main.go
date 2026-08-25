package main

import (
	"fmt"
	"runtime"

	"github.com/ppvan/nem/extractor"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const appTitle = "Anime Downloader"

func main() {
	runtime.LockOSThread() // Windows GUI must run on the OS thread

	// COM is needed for the "Browse..." folder picker (IFileOpenDialog).
	_, _ = win.CoInitializeEx(co.COINIT_APARTMENTTHREADED | co.COINIT_DISABLE_OLE1DDE)
	defer win.CoUninitialize()

	ext, err := extractor.NewAniVietSubExtractor("https://animevietsub.work")
	if err != nil {
		msg := fmt.Errorf("extractor init: %w", err)
		panic(msg)
	}

	ShowMainWindow(ext)
}
