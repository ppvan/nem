package main

import (
	"strconv"

	"github.com/ppvan/nem/extractor"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/rodrigocfd/windigo/win"
)

// MaxEpisodeSlots is the number of checkbox slots pre-built in the UI.
// Windigo controls must be created up front (during WM_CREATE), so we build
// a fixed pool of checkboxes and just show/hide + relabel them once real
// episode data comes back from the extractor. Bump this if you expect
// anime with more episodes than fit here.
const MaxEpisodeSlots = 30

// Overall two-column layout, modeled after Free Manga Downloader's
// sidebar-catalog + main-workspace structure: a left sidebar for
// browsing/searching the catalog, and a right workspace for the selected
// title's details, episode selection, and download controls.
const (
	winWidth  = 970
	winHeight = 615

	sidebarX = 10
	sidebarW = 230 // sidebar controls span x=10..240

	rightX = 255 // 15px gutter after the sidebar
	rightW = 700 // 255..955, leaving a 15px right margin
)

// Grid layout for the fixed episode-checkbox pool, sized to rightW.
const (
	epColCount  = 5
	epColWidth  = rightW / epColCount // 140
	epRowHeight = 26
	epGridX     = rightX
	epGridY     = 285
)

// MyWindow holds every control plus the small amount of UI-side state.
type MyWindow struct {
	wnd *ui.Main

	// Left sidebar: catalog browsing
	edtSidebarSearch *ui.Edit
	btnSidebarSearch *ui.Button
	btnTrending      *ui.Button
	lvResults        *ui.ListView
	lblSidebarStatus *ui.Static

	// Right workspace: URL/Load row
	lblURL     *ui.Static
	edtURL     *ui.Edit
	btnLoadURL *ui.Button

	// Right workspace: info block
	thumbnail   *ui.Control // custom-painted cover image
	lblTitle    *ui.Static
	lblSubtitle *ui.Static
	lblRating   *ui.Static
	lblStats    *ui.Static // views + total episodes
	edtDesc     *ui.Edit

	// Right workspace: episodes
	lblEpisodes  *ui.Static
	btnSelectAll *ui.Button
	episodeChks  [MaxEpisodeSlots]*ui.CheckBox

	// Right workspace: destination + actions
	lblPath        *ui.Static
	edtPath        *ui.Edit
	btnBrowse      *ui.Button
	btnOpenBrowser *ui.Button
	btnDownload    *ui.Button
	lblStatus      *ui.Static

	// State
	ext             *extractor.AniVietSubExtractor
	thumbnailPixels []byte                 // decoded top-down 32bpp BGR pixels (nil = show placeholder); see thumbnail.go
	thumbnailSize   win.SIZE               // pixel dimensions matching thumbnailPixels
	currentInfo     *extractor.AnimeDetail // last loaded anime info; nil until first successful load
}

func ShowMainWindow(ext *extractor.AniVietSubExtractor) int {
	wnd := ui.NewMain(
		ui.OptsMain().
			Title(appTitle).
			Center(true).
			Size(ui.Dpi(winWidth, winHeight)),
		// No ClassIconId() here: that requires an icon resource compiled
		// into the .exe (via a .syso resource file, like windigo's own
		// examples ship). Without one, loading it panics with "the
		// specified image file did not contain a resource section".
		// Add a .syso + ClassIconId(...) later if you want a custom icon.
	)

	// --- Left sidebar: catalog browsing -----------------------------------

	edtSidebarSearch := ui.NewEdit(wnd, ui.OptsEdit().
		Position(ui.Dpi(sidebarX, 35)).
		Width(ui.DpiX(150)),
	)
	btnSidebarSearch := ui.NewButton(wnd, ui.OptsButton().
		Text("Search").
		Position(ui.Dpi(sidebarX+160, 34)).
		Width(ui.DpiX(70)).
		Height(ui.DpiY(26)),
	)
	btnTrending := ui.NewButton(wnd, ui.OptsButton().
		Text("Trending").
		Position(ui.Dpi(sidebarX, 65)).
		Width(ui.DpiX(sidebarW)).
		Height(ui.DpiY(24)),
	)
	lvResults := ui.NewListView(wnd, ui.OptsListView().
		Position(ui.Dpi(sidebarX, 95)).
		Size(ui.Dpi(sidebarW, 480)).
		CtrlStyle(co.LVS_REPORT|co.LVS_NOCOLUMNHEADER|co.LVS_SINGLESEL|co.LVS_SHOWSELALWAYS).
		CtrlExStyle(co.LVS_EX_FULLROWSELECT).
		Column("Title", ui.DpiX(sidebarW-4)),
	)
	lblSidebarStatus := ui.NewStatic(wnd, ui.OptsStatic().
		Text("").
		Position(ui.Dpi(sidebarX, 580)).
		Size(ui.Dpi(sidebarW, 20)),
	)

	// --- Right workspace: URL/Load row -------------------------------------

	lblURL := ui.NewStatic(wnd, ui.OptsStatic().
		Text("URL:").
		Position(ui.Dpi(rightX, 15)).
		Size(ui.Dpi(35, 20)),
	)
	edtURL := ui.NewEdit(wnd, ui.OptsEdit().
		Position(ui.Dpi(rightX+40, 12)).
		Width(ui.DpiX(565)),
	)
	btnLoadURL := ui.NewButton(wnd, ui.OptsButton().
		Text("Load").
		Position(ui.Dpi(rightX+615, 11)).
		Width(ui.DpiX(85)).
		Height(ui.DpiY(26)),
	)

	// --- Right workspace: info block ---------------------------------------

	thumbnail := ui.NewControl(wnd, ui.OptsControl().
		Position(ui.Dpi(rightX, 50)).
		Size(ui.Dpi(160, 200)),
	)

	metaX := rightX + 175
	metaW := rightW - 175

	lblTitle := ui.NewStatic(wnd, ui.OptsStatic().
		Text("Title will appear here").
		Position(ui.Dpi(metaX, 50)).
		Size(ui.Dpi(metaW, 22)),
	)
	lblSubtitle := ui.NewStatic(wnd, ui.OptsStatic().
		Text("").
		Position(ui.Dpi(metaX, 74)).
		Size(ui.Dpi(metaW, 20)),
	)
	lblRating := ui.NewStatic(wnd, ui.OptsStatic().
		Text("Rating: —").
		Position(ui.Dpi(metaX, 96)).
		Size(ui.Dpi(metaW, 20)),
	)
	lblStats := ui.NewStatic(wnd, ui.OptsStatic().
		Text("").
		Position(ui.Dpi(metaX, 118)).
		Size(ui.Dpi(metaW, 20)),
	)
	edtDesc := ui.NewEdit(wnd, ui.OptsEdit().
		Position(ui.Dpi(metaX, 142)).
		Width(ui.DpiX(metaW)).
		Height(ui.DpiY(108)).
		CtrlStyle(co.ES_MULTILINE|co.ES_LEFT|co.ES_AUTOVSCROLL|co.ES_READONLY),
	)

	// --- Right workspace: episodes ------------------------------------------

	lblEpisodes := ui.NewStatic(wnd, ui.OptsStatic().
		Text("Episodes:").
		Position(ui.Dpi(rightX, 260)).
		Size(ui.Dpi(150, 20)),
	)
	btnSelectAll := ui.NewButton(wnd, ui.OptsButton().
		Text("Select All").
		Position(ui.Dpi(rightX+rightW-100, 257)).
		Width(ui.DpiX(100)).
		Height(ui.DpiY(24)),
	)

	var episodeChks [MaxEpisodeSlots]*ui.CheckBox
	for i := 0; i < MaxEpisodeSlots; i++ {
		row := i / epColCount
		col := i % epColCount
		x := epGridX + col*epColWidth
		y := epGridY + row*epRowHeight

		chk := ui.NewCheckBox(wnd, ui.OptsCheckBox().
			Text("Ep. --").
			Position(ui.Dpi(x, y)).
			Size(ui.Dpi(epColWidth-10, 20)),
		)
		episodeChks[i] = chk
	}

	// --- Right workspace: destination + actions -----------------------------

	lblPath := ui.NewStatic(wnd, ui.OptsStatic().
		Text("Save to:").
		Position(ui.Dpi(rightX, 453)).
		Size(ui.Dpi(60, 20)),
	)
	edtPath := ui.NewEdit(wnd, ui.OptsEdit().
		Position(ui.Dpi(rightX+65, 450)).
		Width(ui.DpiX(540)),
	)
	btnBrowse := ui.NewButton(wnd, ui.OptsButton().
		Text("Browse...").
		Position(ui.Dpi(rightX+610, 449)).
		Width(ui.DpiX(85)).
		Height(ui.DpiY(26)),
	)
	btnOpenBrowser := ui.NewButton(wnd, ui.OptsButton().
		Text("Open in Browser").
		Position(ui.Dpi(rightX, 484)).
		Width(ui.DpiX(150)).
		Height(ui.DpiY(28)),
	)
	btnDownload := ui.NewButton(wnd, ui.OptsButton().
		Text("Download").
		Position(ui.Dpi(rightX+rightW-150, 484)).
		Width(ui.DpiX(150)).
		Height(ui.DpiY(28)),
	)
	lblStatus := ui.NewStatic(wnd, ui.OptsStatic().
		Text("").
		Position(ui.Dpi(rightX, 522)).
		Size(ui.Dpi(rightW, 20)),
	)

	me := &MyWindow{
		wnd: wnd,
		ext: ext,

		edtSidebarSearch: edtSidebarSearch,
		btnSidebarSearch: btnSidebarSearch,
		btnTrending:      btnTrending,
		lvResults:        lvResults,
		lblSidebarStatus: lblSidebarStatus,

		lblURL:     lblURL,
		edtURL:     edtURL,
		btnLoadURL: btnLoadURL,

		thumbnail:   thumbnail,
		lblTitle:    lblTitle,
		lblSubtitle: lblSubtitle,
		lblRating:   lblRating,
		lblStats:    lblStats,
		edtDesc:     edtDesc,

		lblEpisodes:  lblEpisodes,
		btnSelectAll: btnSelectAll,
		episodeChks:  episodeChks,

		lblPath:        lblPath,
		edtPath:        edtPath,
		btnBrowse:      btnBrowse,
		btnOpenBrowser: btnOpenBrowser,
		btnDownload:    btnDownload,
		lblStatus:      lblStatus,
	}

	// All child controls get their real HWNDs during WM_CREATE, in the
	// order they were registered above. Hide the unused episode checkbox
	// slots, and kick off an initial Trending() load for the sidebar, only
	// once that has happened.
	wnd.On().WmCreate(func(_ ui.WmCreate) int {
		me.setEpisodeCount(0)
		me.loadResultsList("Trending", me.ext.Trending)
		return 0
	})

	me.events()
	return wnd.RunAsMain()
}

// setEpisodeCount shows the first n episode checkboxes, labeled from
// me.currentInfo.Episodes[i].Title (falling back to a plain position number
// if the title is empty), and hides the rest.
func (me *MyWindow) setEpisodeCount(n int) {
	for i, chk := range me.episodeChks {
		if i < n {
			ep := me.currentInfo.Episodes[i]
			chk.SetTextAndResize(episodeLabel(ep, i+1))
			chk.SetCheck(false)
			chk.Hwnd().ShowWindow(co.SW_SHOW)
		} else {
			chk.Hwnd().ShowWindow(co.SW_HIDE)
		}
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
	return "Ep. " + strconv.Itoa(number)
}
