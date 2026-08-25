package main

import (
	"fmt"
	"runtime"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
	"github.com/rodrigocfd/windigo/x/cosh"
	"github.com/rodrigocfd/windigo/x/winsh"
)

func (me *MyWindow) events() {

	// --- Thumbnail placeholder paint ------------------------------------

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
		// already decoded once (via WIC, see thumbnail.go) when the search
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

	// --- Search ------------------------------------------------------------

	me.btnSearch.On().BnClicked(func() {
		url := me.edtURL.Text()

		me.btnSearch.Hwnd().EnableWindow(false)
		me.btnSearch.SetText("Searching...")
		setStatic(me.lblStatus, "")

		go func() {
			info, err := me.ext.GetAnimeDetailsHref(url)

			// Best-effort: fetch the poster's raw bytes alongside the
			// search itself so they're ready by the time we hop to the UI
			// thread. Plain HTTP GET, no COM involved, so this is safe to
			// do from a bare goroutine (unlike the decode step below).
			var jpegData []byte
			if err == nil {
				jpegData, _ = fetchThumbnail(info.Thumbnail)
			}

			me.wnd.UiThread(func() {
				me.btnSearch.Hwnd().EnableWindow(true)
				me.btnSearch.SetText("Search")

				if err != nil {
					setStatic(me.lblStatus, "Error: "+err.Error())
					return
				}

				me.currentInfo = info

				// WIC decoding needs COM, which is only initialized on
				// this (UI) thread — that's why it happens here rather
				// than alongside the HTTP fetch above. A thumbnail
				// failure doesn't fail the search; it just leaves the
				// "No thumbnail" placeholder up.
				me.thumbnailPixels = nil
				me.thumbnailSize = win.SIZE{}
				if pixels, sz, derr := decodeJpegPixels(jpegData); derr == nil {
					me.thumbnailPixels = pixels
					me.thumbnailSize = sz
				}
				me.thumbnail.Hwnd().RedrawWindow(nil, 0, co.RDW_INVALIDATE)

				title := info.Title
				setStatic(me.lblTitle, title)
				me.edtDesc.SetText(info.Description)
				setStatic(me.lblRating, fmt.Sprintf("Rating: %.1f", info.Rating))

				n := len(info.Episodes)
				if n > MaxEpisodeSlots {
					n = MaxEpisodeSlots
				}
				me.setEpisodeCount(n)

				setStatic(me.lblStatus, fmt.Sprintf("Found %d episode(s). Views: %s",
					len(info.Episodes), info.Views))
			})
		}()
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

	// --- Download selected episodes -------------------------------------

	me.btnDownload.On().BnClicked(func() {
		if me.currentInfo == nil {
			setStatic(me.lblStatus, "Search for an anime first.")
			return
		}

		destDir := me.edtPath.Text()
		if destDir == "" {
			setStatic(me.lblStatus, "Please choose a destination folder first.")
			return
		}

		var selected []selectedEpisode
		for i, chk := range me.episodeChks {
			if i >= len(me.currentInfo.Episodes) {
				break
			}
			if chk.IsChecked() {
				selected = append(selected, selectedEpisode{
					number: i + 1,
					ep:     me.currentInfo.Episodes[i],
				})
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
