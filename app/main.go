package main

import (
	"runtime"

	"github.com/ppvan/nem/extractor"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const appTitle = "Kyoa"

func main() {
	runtime.LockOSThread()

	_, _ = win.CoInitializeEx(co.COINIT_APARTMENTTHREADED | co.COINIT_DISABLE_OLE1DDE)
	defer win.CoUninitialize()

	var (
		ext *extractor.AniVietSubExtractor
		err error
	)
	for _, domain := range animeDomainCandidates() {
		if ext, err = extractor.NewAniVietSubExtractor(domain); err == nil {
			break
		}
	}

	if ext == nil {

		win.HWND(0).MessageBox(
			"Failed to initialize extractor:\n"+err.Error(),
			appTitle, co.MB_ICONERROR)
		return
	}

	stream := newStreamServer(ext)
	if _, err := stream.Start(); err != nil {
		win.HWND(0).MessageBox(
			"Failed to start local stream server:\n"+err.Error(),
			appTitle, co.MB_ICONERROR)
		return
	}

	ShowMainWindow(ext, stream)
}
