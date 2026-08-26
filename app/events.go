package main

import (
	"fmt"
	"runtime"

	"github.com/ppvan/nem/extractor"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
	"github.com/rodrigocfd/windigo/x/cosh"
	"github.com/rodrigocfd/windigo/x/winsh"
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

	// --- Select all / none episodes -----------------------------------------

	me.btnSelectAll.On().BnClicked(func() {
		n := me.lvEpisodes.ItemCount()
		if n == 0 {
			return
		}

		allChecked := true
		for i := 0; i < n; i++ {
			if !isListViewItemChecked(me.lvEpisodes, i) {
				allChecked = false
				break
			}
		}

		for i := 0; i < n; i++ {
			setListViewItemChecked(me.lvEpisodes, i, !allChecked)
		}
	})

	// --- Browse for destination folder --------------------------------

	me.btnBrowse.On().BnClicked(func() {
		rel := win.NewOleReleaser()
		defer rel.Release()

		var fod *winsh.IFileOpenDialog
		if err := win.CoCreateInstance(
			rel,
			&cosh.CLSID_FileOpenDialog,
			nil,
			co.CLSCTX_INPROC_SERVER,
			&fod,
		); err != nil {
			setStatic(me.lblStatus, "Error opening folder picker: "+err.Error())
			return
		}

		defOpts, _ := fod.GetOptions()
		_ = fod.SetOptions(defOpts | cosh.FOS_PICKFOLDERS | cosh.FOS_FORCEFILESYSTEM)

		if ok, _ := fod.Show(me.wnd.Hwnd()); ok {
			item, err := fod.GetResult(rel)
			if err != nil {
				return
			}
			path, _ := item.GetDisplayName(cosh.SIGDN_FILESYSPATH)
			me.edtPath.SetText(path)
		}
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

	// --- Play the first checked episode in mpv -----------------------------

	me.btnPlay.On().BnClicked(func() {
		if me.currentInfo == nil {
			setStatic(me.lblStatus, "Load an anime first.")
			return
		}

		idx := -1
		for i := range me.currentInfo.Episodes {
			if isListViewItemChecked(me.lvEpisodes, i) {
				idx = i
				break
			}
		}
		if idx < 0 {
			setStatic(me.lblStatus, "Check an episode to play.")
			return
		}

		ep := me.currentInfo.Episodes[idx]
		playlistURL := me.stream.NewSession(ep)

		if err := launchMPV(playlistURL); err != nil {
			setStatic(me.lblStatus, "Couldn't launch mpv: "+err.Error())
			return
		}
		setStatic(me.lblStatus, "Launched mpv: "+episodeLabel(ep, idx+1))
	})

	// --- Download selected episodes -------------------------------------

	me.btnDownload.On().BnClicked(func() {
		if me.currentInfo == nil {
			setStatic(me.lblStatus, "Load an anime first.")
			return
		}

		destDir := me.edtPath.Text()
		if destDir == "" {
			setStatic(me.lblStatus, "Please choose a destination folder first.")
			return
		}

		var selected []selectedEpisode
		for i, ep := range me.currentInfo.Episodes {
			if isListViewItemChecked(me.lvEpisodes, i) {
				selected = append(selected, selectedEpisode{number: i + 1, ep: ep})
			}
		}
		if len(selected) == 0 {
			setStatic(me.lblStatus, "Select at least one episode to download.")
			return
		}

		me.btnDownload.Hwnd().EnableWindow(false)
		me.btnDownload.SetText("Downloading...")

		go func() {
			var done, failed int
			for progress := range downloadEpisodes(me.ext, selected, destDir) {
				p := progress
				me.wnd.UiThread(func() {
					switch {
					case p.err != nil:
						failed++
						setStatic(me.lblStatus, fmt.Sprintf("%s failed: %v", p.label, p.err))
					case p.done:
						done++
						setStatic(me.lblStatus, fmt.Sprintf("Downloaded %d/%d: %s",
							done, len(selected), p.label))
					default:
						// Intermediate progress update; note this fires once per
						// callback invocation from ext.Download, so if that's very
						// chatty you may want to throttle these UI updates.
						setStatic(me.lblStatus, fmt.Sprintf("%s: %.0f%%", p.label, p.fraction*100))
					}
				})
			}

			me.wnd.UiThread(func() {
				me.btnDownload.Hwnd().EnableWindow(true)
				me.btnDownload.SetText("Download")
				setStatic(me.lblStatus, fmt.Sprintf("Done. %d succeeded, %d failed.", done, failed))
			})
		}()
	})
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
