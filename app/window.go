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

// Grid layout for the fixed episode-checkbox pool.
const (
	epColCount  = 5
	epColWidth  = 148
	epRowHeight = 26
	epGridX     = 15
	epGridY     = 285
)

// MyWindow holds every control plus the small amount of UI-side state.
type MyWindow struct {
	wnd *ui.Main

	// Search bar
	lblURL    *ui.Static
	edtURL    *ui.Edit
	btnSearch *ui.Button

	// Info block
	thumbnail *ui.Control // custom-painted placeholder for the cover image
	lblTitle  *ui.Static
	edtDesc   *ui.Edit
	lblRating *ui.Static

	// Episodes
	lblEpisodes *ui.Static
	episodeChks [MaxEpisodeSlots]*ui.CheckBox

	// Bottom bar
	lblPath     *ui.Static
	edtPath     *ui.Edit
	btnBrowse   *ui.Button
	btnDownload *ui.Button
	lblStatus   *ui.Static

	// State
	ext             *extractor.AniVietSubExtractor
	thumbnailPixels []byte                 // decoded top-down 32bpp BGR pixels (nil = show placeholder); see thumbnail.go
	thumbnailSize   win.SIZE               // pixel dimensions matching thumbnailPixels
	currentInfo     *extractor.AnimeDetail // last fetched anime info; nil until first successful search
}

func ShowMainWindow(ext *extractor.AniVietSubExtractor) int {
	wnd := ui.NewMain(
		ui.OptsMain().
			Title(appTitle).
			Center(true).
			Size(ui.Dpi(780, 560)),
		// No ClassIconId() here: that requires an icon resource compiled
		// into the .exe (via a .syso resource file, like windigo's own
		// examples ship). Without one, loading it panics with "the
		// specified image file did not contain a resource section".
		// Add a .syso + ClassIconId(...) later if you want a custom icon.
	)

	// --- Search bar -----------------------------------------------------

	lblURL := ui.NewStatic(wnd, ui.OptsStatic().
		Text("URL:").
		Position(ui.Dpi(15, 18)).
		Size(ui.Dpi(35, 20)),
	)
	edtURL := ui.NewEdit(wnd, ui.OptsEdit().
		Position(ui.Dpi(55, 15)).
		Width(ui.DpiX(595)),
	)
	btnSearch := ui.NewButton(wnd, ui.OptsButton().
		Text("Search").
		Position(ui.Dpi(655, 14)).
		Width(ui.DpiX(90)).
		Height(ui.DpiY(26)),
	)

	// --- Info block: left = thumbnail, right = title/description/rating -

	thumbnail := ui.NewControl(wnd, ui.OptsControl().
		Position(ui.Dpi(15, 50)).
		Size(ui.Dpi(160, 200)),
	)

	lblTitle := ui.NewStatic(wnd, ui.OptsStatic().
		Text("Title will appear here").
		Position(ui.Dpi(190, 50)).
		Size(ui.Dpi(555, 24)),
	)
	edtDesc := ui.NewEdit(wnd, ui.OptsEdit().
		Position(ui.Dpi(190, 80)).
		Width(ui.DpiX(555)).
		Height(ui.DpiY(130)).
		CtrlStyle(co.ES_MULTILINE|co.ES_LEFT|co.ES_AUTOVSCROLL|co.ES_READONLY),
	)
	lblRating := ui.NewStatic(wnd, ui.OptsStatic().
		Text("Rating: —").
		Position(ui.Dpi(190, 218)).
		Size(ui.Dpi(555, 20)),
	)

	// --- Episodes ---------------------------------------------------------

	lblEpisodes := ui.NewStatic(wnd, ui.OptsStatic().
		Text("Episodes:").
		Position(ui.Dpi(15, 260)).
		Size(ui.Dpi(200, 20)),
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

	// --- Bottom bar: destination path + download -------------------------

	lblPath := ui.NewStatic(wnd, ui.OptsStatic().
		Text("Save to:").
		Position(ui.Dpi(15, 478)).
		Size(ui.Dpi(60, 20)),
	)
	edtPath := ui.NewEdit(wnd, ui.OptsEdit().
		Position(ui.Dpi(80, 475)).
		Width(ui.DpiX(490)),
	)
	btnBrowse := ui.NewButton(wnd, ui.OptsButton().
		Text("Browse...").
		Position(ui.Dpi(580, 474)).
		Width(ui.DpiX(85)).
		Height(ui.DpiY(26)),
	)
	btnDownload := ui.NewButton(wnd, ui.OptsButton().
		Text("Download").
		Position(ui.Dpi(670, 474)).
		Width(ui.DpiX(85)).
		Height(ui.DpiY(26)),
	)
	lblStatus := ui.NewStatic(wnd, ui.OptsStatic().
		Text("").
		Position(ui.Dpi(15, 510)).
		Size(ui.Dpi(740, 20)),
	)

	me := &MyWindow{
		wnd: wnd,
		ext: ext,

		lblURL:    lblURL,
		edtURL:    edtURL,
		btnSearch: btnSearch,

		thumbnail: thumbnail,
		lblTitle:  lblTitle,
		edtDesc:   edtDesc,
		lblRating: lblRating,

		lblEpisodes: lblEpisodes,
		episodeChks: episodeChks,

		lblPath:     lblPath,
		edtPath:     edtPath,
		btnBrowse:   btnBrowse,
		btnDownload: btnDownload,
		lblStatus:   lblStatus,
	}

	// All child controls get their real HWNDs during WM_CREATE, in the
	// order they were registered above. Hide the unused episode checkbox
	// slots only once that has happened.
	wnd.On().WmCreate(func(_ ui.WmCreate) int {
		me.setEpisodeCount(0)
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
