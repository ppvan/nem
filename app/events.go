package main

import (
	"fmt"
	"runtime"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
	"github.com/rodrigocfd/windigo/x/cosh"
	"github.com/rodrigocfd/windigo/x/winaut"
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

		if me.thumbnailData == nil {
			_, _ = hdc.SetBkMode(co.BKMODE_TRANSPARENT)
			_, _ = hdc.DrawText("No thumbnail", &ps.RcPaint,
				co.DT_CENTER|co.DT_VCENTER|co.DT_SINGLELINE)
			return
		}

		rel := win.NewOleReleaser()
		defer rel.Release() // important: release your COM resources to avoid leaks

		// SHCreateMemStream projects the IStream directly over
		// me.thumbnailData's backing array (no copy), so per windigo's own
		// docs the slice must stay reachable for the syscall's duration.
		// It's already referenced by the me struct so the GC won't collect
		// it mid-call, but KeepAlive makes that guarantee explicit.
		defer runtime.KeepAlive(me.thumbnailData)
		stream, err := winsh.SHCreateMemStream(rel, me.thumbnailData)
		if err != nil {
			_, _ = hdc.SetBkMode(co.BKMODE_TRANSPARENT)
			_, _ = hdc.DrawText("Thumbnail error", &ps.RcPaint,
				co.DT_CENTER|co.DT_VCENTER|co.DT_SINGLELINE)
			return
		}

		// windigo's OleLoadPicture documents JPEG as supported, but it
		// doesn't actually render JPEGs in practice — see
		// https://github.com/rodrigocfd/windigo/issues/46. me.thumbnailData
		// is therefore already BMP-encoded by the time it gets here (see
		// fetchThumbnailBitmap in backend.go), not the raw downloaded JPEG.
		pic, err := winaut.OleLoadPicture(rel, stream, 0, true)
		if err != nil {
			_, _ = hdc.SetBkMode(co.BKMODE_TRANSPARENT)
			_, _ = hdc.DrawText("Thumbnail error", &ps.RcPaint,
				co.DT_CENTER|co.DT_VCENTER|co.DT_SINGLELINE)
			return
		}

		// Stretches to fill the control, same as vye's QR code rendering.
		// Posters are usually portrait (~2:3) and the thumbnail box here
		// is 160x200 (4:5), so there's a small amount of visible stretch —
		// good enough for a first pass; letterboxing to preserve aspect
		// ratio would be the next improvement.
		sz, _ := pic.Size()
		_, _ = pic.Render(hdc,
			win.POINT{},
			win.SIZE{Cx: ps.RcPaint.Right, Cy: ps.RcPaint.Bottom},
			win.POINT{X: 0, Y: sz.Cy},
			win.SIZE{Cx: sz.Cx, Cy: -sz.Cy},
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

			// Best-effort: fetch + convert the poster alongside the search
			// itself so it's ready by the time we update the UI. A
			// thumbnail failure shouldn't fail the whole search — just
			// falls back to the "No thumbnail" placeholder.
			var thumbData []byte
			if err == nil {
				thumbData, _ = fetchThumbnailBitmap(info.Thumbnail)
			}

			me.wnd.UiThread(func() {
				me.btnSearch.Hwnd().EnableWindow(true)
				me.btnSearch.SetText("Search")

				if err != nil {
					setStatic(me.lblStatus, "Error: "+err.Error())
					return
				}

				me.currentInfo = info
				me.thumbnailData = thumbData
				me.thumbnail.Hwnd().RedrawWindow(nil, 0, co.RDW_INVALIDATE)

				title := info.Title
				if info.Subtitle != "" {
					title += " - " + info.Subtitle
				}
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
