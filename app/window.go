package main

import (
	"fmt"

	"github.com/ppvan/nem/extractor"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/rodrigocfd/windigo/win"
)

const (
	winWidth  = 970
	winHeight = 570

	sidebarX = 10
	sidebarW = 230

	rightX       = 255
	rightW       = 700
	scrollBarPad = 8
)

type MyWindow struct {
	wnd *ui.Main

	edtSidebarSearch *ui.Edit
	btnSidebarSearch *ui.Button
	lvResults        *ui.ListView
	lblSidebarStatus *ui.Static

	thumbnail   *ui.Control
	lblTitle    *ui.Static
	lblSubtitle *ui.Static
	lblRating   *ui.Static
	lblStats    *ui.Static
	edtDesc     *ui.Edit

	lblEpisodes *ui.Static
	btnOpenMPV  *ui.Button
	lvEpisodes  *ui.ListView

	lblStatus *ui.Static

	ext             *extractor.AniVietSubExtractor
	stream          *streamServer
	thumbnailPixels []byte
	thumbnailSize   win.SIZE
	currentInfo     *extractor.AnimeDetail
}

func ShowMainWindow(ext *extractor.AniVietSubExtractor, stream *streamServer) int {
	wnd := ui.NewMain(
		ui.OptsMain().
			Title(appTitle).
			Center(true).
			ClassIconId(42).
			Size(ui.Dpi(winWidth, winHeight)),
	)

	vScrollW := int(win.GetSystemMetrics(co.SM_CXVSCROLL))

	edtSidebarSearch := ui.NewEdit(wnd, ui.OptsEdit().
		Position(ui.Dpi(sidebarX, 13)).
		Width(ui.DpiX(150)),
	)
	btnSidebarSearch := ui.NewButton(wnd, ui.OptsButton().
		Text("Search").
		Position(ui.Dpi(sidebarX+160, 12)).
		Width(ui.DpiX(70)).
		Height(ui.DpiY(26)),
	)
	lvResults := ui.NewListView(wnd, ui.OptsListView().
		Position(ui.Dpi(sidebarX, 45)).
		Size(ui.Dpi(sidebarW, 480)).
		CtrlStyle(co.LVS_REPORT|co.LVS_NOCOLUMNHEADER|co.LVS_SINGLESEL|co.LVS_SHOWSELALWAYS).
		CtrlExStyle(co.LVS_EX_FULLROWSELECT).
		Column("Title", ui.DpiX(sidebarW)-vScrollW-scrollBarPad),
	)
	lblSidebarStatus := ui.NewStatic(wnd, ui.OptsStatic().
		Text("").
		Position(ui.Dpi(sidebarX, 540)).
		Size(ui.Dpi(sidebarW, 20)),
	)

	thumbnail := ui.NewControl(wnd, ui.OptsControl().
		Position(ui.Dpi(rightX, 12)).
		Size(ui.Dpi(160, 200)),
	)

	metaX := rightX + 175
	metaW := rightW - 175

	lblTitle := ui.NewStatic(wnd, ui.OptsStatic().
		Text("Select or search for a title to get started").
		Position(ui.Dpi(metaX, 12)).
		Size(ui.Dpi(metaW, 22)),
	)
	lblSubtitle := ui.NewStatic(wnd, ui.OptsStatic().
		Text("").
		Position(ui.Dpi(metaX, 36)).
		Size(ui.Dpi(metaW, 20)),
	)
	lblRating := ui.NewStatic(wnd, ui.OptsStatic().
		Text("Rating: —").
		Position(ui.Dpi(metaX, 58)).
		Size(ui.Dpi(metaW, 20)),
	)
	lblStats := ui.NewStatic(wnd, ui.OptsStatic().
		Text("").
		Position(ui.Dpi(metaX, 80)).
		Size(ui.Dpi(metaW, 20)),
	)
	edtDesc := ui.NewEdit(wnd, ui.OptsEdit().
		Position(ui.Dpi(metaX, 104)).
		Width(ui.DpiX(metaW)).
		Height(ui.DpiY(108)).
		CtrlStyle(co.ES_MULTILINE|co.ES_LEFT|co.ES_AUTOVSCROLL|co.ES_READONLY),
	)

	lblEpisodes := ui.NewStatic(wnd, ui.OptsStatic().
		Text("Episodes (double-click to play from there):").
		Position(ui.Dpi(rightX, 222)).
		Size(ui.Dpi(rightW-160, 20)),
	)
	btnOpenMPV := ui.NewButton(wnd, ui.OptsButton().
		Text("Open in MPV").
		Position(ui.Dpi(rightX+rightW-150, 219)).
		Width(ui.DpiX(150)).
		Height(ui.DpiY(24)),
	)
	epNumColW := ui.DpiX(50)
	lvEpisodes := ui.NewListView(wnd, ui.OptsListView().
		Position(ui.Dpi(rightX, 252)).
		Size(ui.Dpi(rightW, 272)).
		CtrlStyle(co.LVS_REPORT|co.LVS_NOCOLUMNHEADER|co.LVS_SINGLESEL|co.LVS_SHOWSELALWAYS).
		CtrlExStyle(co.LVS_EX_FULLROWSELECT).
		Column("#", epNumColW).
		Column("Title", ui.DpiX(rightW)-epNumColW-vScrollW-scrollBarPad),
	)

	lblStatus := ui.NewStatic(wnd, ui.OptsStatic().
		Text("").
		Position(ui.Dpi(rightX, 540)).
		Size(ui.Dpi(rightW, 20)),
	)

	me := &MyWindow{
		wnd:    wnd,
		ext:    ext,
		stream: stream,

		edtSidebarSearch: edtSidebarSearch,
		btnSidebarSearch: btnSidebarSearch,
		lvResults:        lvResults,
		lblSidebarStatus: lblSidebarStatus,

		thumbnail:   thumbnail,
		lblTitle:    lblTitle,
		lblSubtitle: lblSubtitle,
		lblRating:   lblRating,
		lblStats:    lblStats,
		edtDesc:     edtDesc,

		lblEpisodes: lblEpisodes,
		btnOpenMPV:  btnOpenMPV,
		lvEpisodes:  lvEpisodes,

		lblStatus: lblStatus,
	}

	wnd.On().WmCreate(func(_ ui.WmCreate) int {
		me.loadResultsList("Trending", me.ext.Trending)
		return 0
	})

	me.events()
	return wnd.RunAsMain()
}

func (me *MyWindow) setEpisodes(episodes []extractor.Episode) {
	me.lvEpisodes.DeleteAllItems()
	for i, ep := range episodes {
		me.lvEpisodes.AddItem(fmt.Sprintf("%02d", i+1), episodeLabel(ep, i+1))
	}
}

func setStatic(s *ui.Static, text string) {
	s.Hwnd().SetWindowText(text)
}

func episodeLabel(ep extractor.Episode, number int) string {
	if ep.Title != "" {
		return ep.Title
	}
	return fmt.Sprintf("Episode %d", number)
}
