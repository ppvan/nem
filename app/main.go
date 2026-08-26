package main

import (
	"runtime"

	"github.com/ppvan/nem/extractor"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const appTitle = "Anime Downloader"

// TODO: make this configurable (a settings field, env var, flag, etc.)
// instead of hardcoding a single site.
const animeDomain = "https://animevietsub.work"

func main() {
	runtime.LockOSThread() // Windows GUI must run on the OS thread

	// COM is needed for the "Browse..." folder picker (IFileOpenDialog).
	_, _ = win.CoInitializeEx(co.COINIT_APARTMENTTHREADED | co.COINIT_DISABLE_OLE1DDE)
	defer win.CoUninitialize()

	// Built once and reused for the whole app session (rather than per
	// search) so we don't pay for TLS client setup / warmUp() on every
	// click.
	ext, err := extractor.NewAniVietSubExtractor(animeDomain)
	if err != nil {
		// A windowsgui build has no console attached, so surface startup
		// failures with a message box instead of printing to stderr.
		win.HWND(0).MessageBox(
			"Failed to initialize extractor:\n"+err.Error(),
			appTitle, co.MB_ICONERROR)
		return
	}

	// Local loopback proxy that lets an external mpv process play an
	// episode through ext's own fetch/decrypt logic — see stream.go.
	stream := newStreamServer(ext)
	if _, err := stream.Start(); err != nil {
		win.HWND(0).MessageBox(
			"Failed to start local stream server:\n"+err.Error(),
			appTitle, co.MB_ICONERROR)
		return
	}

	ShowMainWindow(ext, stream)
}
