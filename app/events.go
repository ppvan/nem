package main

import (
	"fmt"
	"runtime"

	"github.com/ppvan/nem/extractor"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func (me *MyWindow) events() {

	// --- Thumbnail paint --------------------------------------------------

	me.thumbnail.On().WmPaint(func() {
		var ps win.PAINTSTRUCT
		hdc, err := me.thumbnail.Hwnd().BeginPaint(&ps)
		if err != nil {
			panic(err)
		}
		defer me.thumbnail.Hwnd().EndPaint(&ps)

		bgBrush, _ := win.GetSysColorBrush(co.COLOR_BTNFACE)
		defer bgBrush.DeleteObject()
		hdc.FillRect(&ps.RcPaint, bgBrush)

		if me.thumbnailPixels == nil {
			_, _ = hdc.SetBkMode(co.BKMODE_TRANSPARENT)
			_, _ = hdc.DrawText("No thumbnail", &ps.RcPaint,
				co.DT_CENTER|co.DT_VCENTER|co.DT_SINGLELINE)
			return
		}

		// Pure GDI from here — no COM, no HBITMAP. me.thumbnailPixels was
		// already decoded once (via WIC, see thumbnail.go) when the load
		// completed, so painting is just a raw memory blit.
		var bi win.BITMAPINFO
		bi.BmiHeader.Width = me.thumbnailSize.Cx
		bi.BmiHeader.Height = -me.thumbnailSize.Cy // negative = top-down, matching WIC's row order
		bi.BmiHeader.Planes = 1
		bi.BmiHeader.BitCount = co.BITCOUNT_32
		bi.BmiHeader.Compression = co.BI_RGB
		bi.BmiHeader.SetBiSize()

		// StretchDIBits reads directly from the slice passed via pointer,
		// so keep it alive through the syscall (see the KeepAlive note in
		// thumbnail.go for the same reasoning).
		defer runtime.KeepAlive(me.thumbnailPixels)

		// object-fit: cover — crop a centered region of the source that
		// matches the destination box's aspect ratio, then stretch that
		// crop to fill the box completely (accepting some cropping)
		// instead of stretching the whole uncropped source and distorting
		// it. bi above still describes the full decoded image; only the
		// source rectangle passed to StretchDIBits changes.
		dstW, dstH := ps.RcPaint.Right, ps.RcPaint.Bottom
		srcX, srcY, srcW, srcH := coverSourceRect(me.thumbnailSize.Cx, me.thumbnailSize.Cy, dstW, dstH)

		_, _ = hdc.StretchDIBits(
			win.POINT{},
			win.SIZE{Cx: dstW, Cy: dstH},
			win.POINT{X: srcX, Y: srcY},
			win.SIZE{Cx: srcW, Cy: srcH},
			&me.thumbnailPixels[0],
			&bi,
			co.DIB_COLORS_RGB,
			co.ROP_SRCCOPY,
		)
	})

	// --- Sidebar: search the catalog ---------------------------------------

	me.btnSidebarSearch.On().BnClicked(func() {
		query := me.edtSidebarSearch.Text()
		if query == "" {
			setStatic(me.lblSidebarStatus, "Type something to search.")
			return
		}
		me.loadResultsList(fmt.Sprintf("%q", query), func() ([]extractor.SimpleAnime, error) {
			return me.ext.Search(query)
		})
	})

	// --- Sidebar: trending -------------------------------------------------

	me.btnTrending.On().BnClicked(func() {
		me.loadResultsList("Trending", me.ext.Trending)
	})

	// --- Sidebar: double-click a result to load its details ---------------

	me.lvResults.On().NmDblClk(func(p *win.NMITEMACTIVATE) {
		if p.IItem < 0 {
			return
		}
		sa, ok := me.lvResults.Item(int(p.IItem)).Data().(extractor.SimpleAnime)
		if !ok {
			return
		}
		me.loadAnimeDetails(func() (*extractor.AnimeDetail, error) {
			return me.ext.GetAnimeDetails(sa.Id)
		})
	})

	// --- Episodes: double-click a row to play from there to the end -------

	me.lvEpisodes.On().NmDblClk(func(p *win.NMITEMACTIVATE) {
		if p.IItem < 0 {
			return
		}
		me.playFrom(int(p.IItem))
	})

	// --- Open in MPV: play the whole series from episode 1 -----------------

	me.btnOpenMPV.On().BnClicked(func() {
		me.playFrom(0)
	})

	// --- Open the current anime's page in the default browser -------------

	me.btnOpenBrowser.On().BnClicked(func() {
		if me.currentInfo == nil || me.currentInfo.Href == "" {
			setStatic(me.lblStatus, "Load an anime first.")
			return
		}
		if err := openInBrowser(me.currentInfo.Href); err != nil {
			setStatic(me.lblStatus, "Couldn't open browser: "+err.Error())
		}
	})
}

// playFrom launches mpv on a watch playlist covering
// me.currentInfo.Episodes[startIdx:] — see streamServer.NewWatchSession's
// doc comment in stream.go for how episodes after the first get queued.
// Called from either a double-click on an episode row or the Open in MPV
// button (which always passes 0).
func (me *MyWindow) playFrom(startIdx int) {
	if me.currentInfo == nil || len(me.currentInfo.Episodes) == 0 {
		setStatic(me.lblStatus, "Load an anime first.")
		return
	}
	if startIdx < 0 || startIdx >= len(me.currentInfo.Episodes) {
		return
	}

	episodes := me.currentInfo.Episodes[startIdx:]
	watchURL := me.stream.NewWatchSession(episodes)

	if err := launchMPV(watchURL); err != nil {
		setStatic(me.lblStatus, "Couldn't launch mpv: "+err.Error())
		return
	}

	label := episodeLabel(episodes[0], startIdx+1)
	setStatic(me.lblStatus, fmt.Sprintf("Playing from %s (%d episode(s) queued)", label, len(episodes)))
}

// loadResultsList runs fetch (ext.Search or ext.Trending) in the
// background and populates the sidebar list view with the results. Safe
// to call from the UI thread (e.g. a button click) — it hops to a
// goroutine itself, since the HTTP work involved is a plain network call
// with no COM/GDI, unlike loadAnimeDetails' thumbnail step below.
func (me *MyWindow) loadResultsList(label string, fetch func() ([]extractor.SimpleAnime, error)) {
	setStatic(me.lblSidebarStatus, "Loading "+label+"...")

	go func() {
		results, err := fetch()

		me.wnd.UiThread(func() {
			if err != nil {
				setStatic(me.lblSidebarStatus, "Error: "+err.Error())
				return
			}

			me.lvResults.DeleteAllItems()
			for _, sa := range results {
				me.lvResults.AddItem(sa.Title).SetData(sa)
			}
			setStatic(me.lblSidebarStatus, fmt.Sprintf("%s: %d result(s)", label, len(results)))
		})
	}()
}

// loadAnimeDetails runs fetch (ext.GetAnimeDetails, from a sidebar
// double-click) in the background, downloads the poster alongside it,
// decodes the poster via WIC once back on the UI thread (WIC needs the COM
// apartment that only this thread has — see thumbnail.go), and updates the
// whole right-hand workspace.
func (me *MyWindow) loadAnimeDetails(fetch func() (*extractor.AnimeDetail, error)) {
	setStatic(me.lblStatus, "Loading...")

	go func() {
		info, err := fetch()

		// Best-effort: fetch the poster's raw bytes alongside the details
		// themselves so they're ready by the time we hop to the UI thread.
		// Plain HTTP GET, no COM involved, so safe from a bare goroutine
		// (unlike the decode step below).
		var jpegData []byte
		if err == nil {
			jpegData, _ = fetchThumbnail(info.Thumbnail)
		}

		me.wnd.UiThread(func() {
			if err != nil {
				setStatic(me.lblStatus, "Error: "+err.Error())
				return
			}

			me.currentInfo = info

			// WIC decoding needs COM, which is only initialized on this
			// (UI) thread — that's why it happens here rather than
			// alongside the HTTP fetch above. A thumbnail failure doesn't
			// fail the load; it just leaves the "No thumbnail" placeholder up.
			me.thumbnailPixels = nil
			me.thumbnailSize = win.SIZE{}
			if pixels, sz, derr := decodeJpegPixels(jpegData); derr == nil {
				me.thumbnailPixels = pixels
				me.thumbnailSize = sz
			}
			me.thumbnail.Hwnd().RedrawWindow(nil, 0, co.RDW_INVALIDATE)

			setStatic(me.lblTitle, info.Title)
			setStatic(me.lblSubtitle, info.Subtitle)
			me.edtDesc.SetText(info.Description)
			setStatic(me.lblRating, fmt.Sprintf("Rating: %.1f", info.Rating))
			setStatic(me.lblStats, fmt.Sprintf("Episodes: %s   Views: %s", info.TotalEpisodes, info.Views))

			me.setEpisodes(info.Episodes)

			setStatic(me.lblStatus, fmt.Sprintf("Loaded. %d episode(s) found.", len(info.Episodes)))
		})
	}()
}
