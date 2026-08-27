package main

import (
	"fmt"
	"runtime"

	"github.com/ppvan/nem/extractor"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func (me *MyWindow) events() {

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

		var bi win.BITMAPINFO
		bi.BmiHeader.Width = me.thumbnailSize.Cx
		bi.BmiHeader.Height = -me.thumbnailSize.Cy
		bi.BmiHeader.Planes = 1
		bi.BmiHeader.BitCount = co.BITCOUNT_32
		bi.BmiHeader.Compression = co.BI_RGB
		bi.BmiHeader.SetBiSize()

		defer runtime.KeepAlive(me.thumbnailPixels)

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

	me.lvEpisodes.On().NmDblClk(func(p *win.NMITEMACTIVATE) {
		if p.IItem < 0 {
			return
		}
		me.playFrom(int(p.IItem))
	})

	me.btnOpenMPV.On().BnClicked(func() {
		me.playFrom(0)
	})

}

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

func (me *MyWindow) loadAnimeDetails(fetch func() (*extractor.AnimeDetail, error)) {
	setStatic(me.lblStatus, "Loading...")

	go func() {
		info, err := fetch()

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
