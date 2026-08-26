package main

import (
	"fmt"

	"github.com/ppvan/nem/extractor"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/rodrigocfd/windigo/win"
)

// Overall two-column layout, modeled after Free Manga Downloader's
// sidebar-catalog + main-workspace structure: a left sidebar for
// browsing/searching the catalog, and a right workspace for the selected
// title's details and episode list.
const (
	winWidth  = 970
	winHeight = 570

	sidebarX = 10
	sidebarW = 230 // sidebar controls span x=10..240

	rightX       = 255 // 15px gutter after the sidebar
	rightW       = 700 // 255..955, leaving a 15px right margin
	scrollBarPad = 8
)

// MyWindow holds every control plus the small amount of UI-side state.
type MyWindow struct {
	wnd *ui.Main

	// Left sidebar: catalog browsing
	edtSidebarSearch *ui.Edit
	btnSidebarSearch *ui.Button
	lvResults        *ui.ListView
	lblSidebarStatus *ui.Static

	// Right workspace: info block
	thumbnail   *ui.Control // custom-painted cover image
	lblTitle    *ui.Static
	lblSubtitle *ui.Static
	lblRating   *ui.Static
	lblStats    *ui.Static // views + total episodes
	edtDesc     *ui.Edit

	// Right workspace: episodes — double-click a row to play from there to
	// the end of the series. No checkboxes/selection: playback is the only
	// action, so there's nothing to select for.
	lblEpisodes *ui.Static
	btnOpenMPV  *ui.Button // plays the whole series, starting from episode 1
	lvEpisodes  *ui.ListView

	// Right workspace: remaining actions
	lblStatus *ui.Static

	// State
	ext             *extractor.AniVietSubExtractor
	stream          *streamServer
	thumbnailPixels []byte                 // decoded top-down 32bpp BGR pixels (nil = show placeholder); see thumbnail.go
	thumbnailSize   win.SIZE               // pixel dimensions matching thumbnailPixels
	currentInfo     *extractor.AnimeDetail // last loaded anime info; nil until first successful load
}

func ShowMainWindow(ext *extractor.AniVietSubExtractor, stream *streamServer) int {
	wnd := ui.NewMain(
		ui.OptsMain().
			Title(appTitle).
			Center(true).
			ClassIconId(42).
			Size(ui.Dpi(winWidth, winHeight)),
		// No ClassIconId() here: that requires an icon resource compiled
		// into the .exe (via a .syso resource file, like windigo's own
		// examples ship). Without one, loading it panics with "the
		// specified image file did not contain a resource section".
		// Add a .syso + ClassIconId(...) later if you want a custom icon.
	)

	// --- Left sidebar: catalog browsing -----------------------------------

	// Reserve room for the vertical scrollbar in the column width: a
	// LVS_REPORT ListView's horizontal scroll extent is driven by the sum
	// of its column widths, which is computed against the control's full
	// width, not its content width net of the vertical scrollbar. Size the
	// single column right up to that scrollbar and no further, or a
	// horizontal scrollbar appears the moment enough rows trigger a
	// vertical one — even though there's no actual horizontal overflow.
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

	// --- Right workspace: info block ---------------------------------------

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

	// --- Right workspace: episodes ------------------------------------------

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

	// --- Right workspace: remaining actions ---------------------------------

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

	// All child controls get their real HWNDs during WM_CREATE, in the
	// order they were registered above. Kick off an initial Trending()
	// load for the sidebar only once that has happened.
	wnd.On().WmCreate(func(_ ui.WmCreate) int {
		me.loadResultsList("Trending", me.ext.Trending)
		return 0
	})

	me.events()
	return wnd.RunAsMain()
}

// setEpisodes replaces lvEpisodes' rows with one per episode. A ListView's
// rows can be added/removed at any time (unlike Windigo's fixed, built-at-
// WM_CREATE controls), so setEpisodes just clears and repopulates it per
// anime — no cap on episode count.
func (me *MyWindow) setEpisodes(episodes []extractor.Episode) {
	me.lvEpisodes.DeleteAllItems()
	for i, ep := range episodes {
		me.lvEpisodes.AddItem(fmt.Sprintf("%02d", i+1), episodeLabel(ep, i+1))
	}
}

// setStatic sets a Static control's text without triggering an
// auto-resize (unlike Static.SetTextAndResize), so fixed-position labels
// keep their layout box.
func setStatic(s *ui.Static, text string) {
	s.Hwnd().SetWindowText(text)
}

func episodeLabel(ep extractor.Episode, number int) string {
	if ep.Title != "" {
		return ep.Title
	}
	return fmt.Sprintf("Episode %d", number)
}
