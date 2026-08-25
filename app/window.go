package main

import (
	"fmt"
	"unsafe"

	"github.com/ppvan/nem/extractor"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/rodrigocfd/windigo/win"
)

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

// MyWindow holds every control plus the small amount of UI-side state.
type MyWindow struct {
	wnd *ui.Main

	// Left sidebar: catalog browsing
	edtSidebarSearch *ui.Edit
	btnSidebarSearch *ui.Button
	btnTrending      *ui.Button
	lvResults        *ui.ListView
	lblSidebarStatus *ui.Static

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
	lvEpisodes   *ui.ListView

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

	// Reserve room for the vertical scrollbar in the column width: a
	// LVS_REPORT ListView's horizontal scroll extent is driven by the sum
	// of its column widths, which is computed against the control's full
	// width, not its content width net of the vertical scrollbar. Size the
	// single column right up to that scrollbar and no further, or a
	// horizontal scrollbar appears the moment enough rows trigger a
	// vertical one — even though there's no actual horizontal overflow.
	vScrollW := int(win.GetSystemMetrics(co.SM_CXVSCROLL))

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
		Column("Title", ui.DpiX(sidebarW)-vScrollW-2),
	)
	lblSidebarStatus := ui.NewStatic(wnd, ui.OptsStatic().
		Text("").
		Position(ui.Dpi(sidebarX, 580)).
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
		Text("Episodes:").
		Position(ui.Dpi(rightX, 222)).
		Size(ui.Dpi(150, 20)),
	)
	btnSelectAll := ui.NewButton(wnd, ui.OptsButton().
		Text("Select All").
		Position(ui.Dpi(rightX+rightW-100, 219)).
		Width(ui.DpiX(100)).
		Height(ui.DpiY(24)),
	)
	epNumColW := ui.DpiX(50)
	lvEpisodes := ui.NewListView(wnd, ui.OptsListView().
		Position(ui.Dpi(rightX, 248)).
		Size(ui.Dpi(rightW, 192)).
		CtrlStyle(co.LVS_REPORT|co.LVS_NOCOLUMNHEADER|co.LVS_SHOWSELALWAYS).
		CtrlExStyle(co.LVS_EX_FULLROWSELECT|co.LVS_EX_CHECKBOXES).
		Column("#", epNumColW).
		Column("Title", ui.DpiX(rightW)-epNumColW-vScrollW-8),
	)

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

		thumbnail:   thumbnail,
		lblTitle:    lblTitle,
		lblSubtitle: lblSubtitle,
		lblRating:   lblRating,
		lblStats:    lblStats,
		edtDesc:     edtDesc,

		lblEpisodes:  lblEpisodes,
		btnSelectAll: btnSelectAll,
		lvEpisodes:   lvEpisodes,

		lblPath:        lblPath,
		edtPath:        edtPath,
		btnBrowse:      btnBrowse,
		btnOpenBrowser: btnOpenBrowser,
		btnDownload:    btnDownload,
		lblStatus:      lblStatus,
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

// setEpisodes replaces lvEpisodes' rows with one per episode, each
// starting unchecked. Unlike the fixed-checkbox-pool approach this
// replaced, a ListView's rows can be added/removed at runtime, so there's
// no fixed cap on episode count anymore.
func (me *MyWindow) setEpisodes(episodes []extractor.Episode) {
	me.lvEpisodes.DeleteAllItems()
	for i, ep := range episodes {
		item := me.lvEpisodes.AddItem(fmt.Sprintf("%02d", i+1), episodeLabel(ep, i+1))
		setListViewItemChecked(me.lvEpisodes, item.Index(), false)
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

// --- LVS_EX_CHECKBOXES state helpers ---------------------------------------
//
// Windigo's ui.ListView/ListViewItem wrapper doesn't expose the per-row
// checkbox state added by the LVS_EX_CHECKBOXES extended style, so these
// send the underlying LVM_SETITEMSTATE/LVM_GETITEMSTATE messages directly
// — the same pattern ui.ListViewItem.Select()/IsSelected() use internally
// for LVIS_SELECTED, just with the state-image bits (LVIS_STATEIMAGEMASK)
// that LVS_EX_CHECKBOXES repurposes for the checkbox: state image index 1
// is unchecked, index 2 is checked (both encoded as index<<12).

const (
	lvisUnchecked = co.LVIS(0x1000) // INDEXTOSTATEIMAGEMASK(1)
	lvisChecked   = co.LVIS(0x2000) // INDEXTOSTATEIMAGEMASK(2)
)

func setListViewItemChecked(lv *ui.ListView, index int, checked bool) {
	state := lvisUnchecked
	if checked {
		state = lvisChecked
	}
	lvi := win.LVITEM{
		State:     state,
		StateMask: co.LVIS_STATEIMAGEMASK,
	}
	_, _ = lv.Hwnd().SendMessage(co.LVM_SETITEMSTATE,
		win.WPARAM(int32(index)), win.LPARAM(unsafe.Pointer(&lvi)))
}

func isListViewItemChecked(lv *ui.ListView, index int) bool {
	ret, _ := lv.Hwnd().SendMessage(co.LVM_GETITEMSTATE,
		win.WPARAM(int32(index)), win.LPARAM(co.LVIS_STATEIMAGEMASK))
	return co.LVIS(ret) == lvisChecked
}
