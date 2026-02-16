package progressbar

import (
	"fmt"
	"os"
)

// Color represents an RGB color
type Color struct {
	R, G, B uint8
}

// ProgressBar represents a terminal progress bar
type ProgressBar struct {
	total          int
	current        int
	width          int
	accentColor    Color
	supports24Bit  bool
	cursorHidden   bool
	showPercentage bool
}

// Options for configuring the progress bar
type Options struct {
	// Total number of steps (required)
	Total int

	// Width of the progress bar in characters (default: 40)
	Width int

	// Accent color in RGB (default: #847545)
	AccentColor *Color

	// Show percentage (default: true)
	ShowPercentage bool
}

// DefaultAccentColor is a brownish/tan color (#847545)
var DefaultAccentColor = Color{R: 132, G: 117, B: 69}

// New creates a new progress bar with the given options
func New(opts Options) *ProgressBar {
	if opts.Width == 0 {
		opts.Width = 40
	}

	accentColor := DefaultAccentColor
	if opts.AccentColor != nil {
		accentColor = *opts.AccentColor
	}

	// Check if terminal supports 24-bit color
	colorTerm := os.Getenv("COLORTERM")
	supports24Bit := colorTerm == "truecolor" || colorTerm == "24bit"

	return &ProgressBar{
		total:          opts.Total,
		current:        0,
		width:          opts.Width,
		accentColor:    accentColor,
		supports24Bit:  supports24Bit,
		cursorHidden:   false,
		showPercentage: opts.ShowPercentage || opts.ShowPercentage == false && opts.Total > 0, // default true
	}
}

func (pb *ProgressBar) Start() {
	if !pb.cursorHidden {
		fmt.Print("\x1b[?25l") // Hide cursor
		pb.cursorHidden = true
	}
	pb.Render()
}

func (pb *ProgressBar) Update(current int) {
	if current > pb.total {
		current = pb.total
	}
	if current < 0 {
		current = 0
	}
	pb.current = current
	pb.Render()
}

func (pb *ProgressBar) Increment() {
	pb.Update(pb.current + 1)
}
func (pb *ProgressBar) Add(delta int) {
	pb.Update(pb.current + delta)
}

func (pb *ProgressBar) Finish() {
	pb.Update(pb.total)
	if pb.cursorHidden {
		fmt.Print("\x1b[?25h") // Show cursor
		pb.cursorHidden = false
	}
	fmt.Println() // Move to next line
}

func (pb *ProgressBar) Render() {
	percentage := 0
	if pb.total > 0 {
		percentage = (pb.current * 100) / pb.total
	}
	numBlocks := 0
	if pb.total > 0 {
		numBlocks = (pb.current * pb.width) / pb.total
	}

	fmt.Print("\r")

	if numBlocks > 0 {
		if pb.supports24Bit {
			fmt.Printf("\x1b[38;2;%d;%d;%dm", pb.accentColor.R, pb.accentColor.G, pb.accentColor.B)
		} else {
			fmt.Print("\x1b[33m") // Yellow fallback
		}

		for j := 0; j < numBlocks; j++ {
			fmt.Print("█")
		}
		fmt.Print("\x1b[0m") // Reset color
	}

	if numBlocks < pb.width {
		if pb.supports24Bit {
			darkR := pb.accentColor.R * 30 / 100
			darkG := pb.accentColor.G * 30 / 100
			darkB := pb.accentColor.B * 30 / 100
			fmt.Printf("\x1b[38;2;%d;%d;%dm", darkR, darkG, darkB)
		} else {
			fmt.Print("\x1b[90m") // Dark gray fallback
		}

		for j := numBlocks; j < pb.width; j++ {
			fmt.Print("█")
		}
		fmt.Print("\x1b[0m") // Reset color
	}

	if pb.showPercentage {
		fmt.Printf(" %3d%%", percentage)
	}
}
func (pb *ProgressBar) SetTotal(total int) {
	pb.total = total
}

func (pb *ProgressBar) GetCurrent() int {
	return pb.current
}
func (pb *ProgressBar) GetTotal() int {
	return pb.total
}

func (pb *ProgressBar) IsComplete() bool {
	return pb.current >= pb.total
}
