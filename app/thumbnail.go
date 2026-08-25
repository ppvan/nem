package main

import (
	"fmt"
	"runtime"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
	"github.com/rodrigocfd/windigo/x/cowic"
	"github.com/rodrigocfd/windigo/x/winsh"
	"github.com/rodrigocfd/windigo/x/winwic"
)

// decodeJpegPixels decodes JPEG bytes into a flat, top-down, 32bpp BGR
// pixel buffer ready to hand straight to HDC.StretchDIBits — no HBITMAP,
// no IPicture, no intermediate BMP re-encode.
//
// This uses WIC (Windows Imaging Component, https://github.com/rodrigocfd/windigo/tree/master/x/winwic)
// directly rather than windigo's IPicture/OleLoadPicture wrapper. That
// wrapper documents JPEG as supported, but doesn't actually render it in
// practice (see https://github.com/rodrigocfd/windigo/issues/46) — only
// BMP reliably works through it. WIC decodes JPEG (and PNG, GIF, TIFF,
// etc.) natively, so this sidesteps that limitation entirely instead of
// working around it with a Go-side JPEG-decode-then-BMP-encode step.
//
// Must be called on a COM-initialized (apartment-threaded) OS thread —
// i.e. from inside a me.wnd.UiThread(...) callback, not a bare goroutine.
// A background HTTP-fetch goroutine has no COM apartment of its own, so
// calling this there would fail with CO_E_NOTINITIALIZED (or worse, run on
// whatever OS thread the Go scheduler happens to hand it).
func decodeJpegPixels(jpegData []byte) (pixels []byte, size win.SIZE, err error) {
	if len(jpegData) == 0 {
		return nil, win.SIZE{}, fmt.Errorf("no image data")
	}

	rel := win.NewOleReleaser()
	defer rel.Release()

	var factory *winwic.IWICImagingFactory
	if err = win.CoCreateInstance(
		rel,
		&cowic.CLSID_WICImagingFactory,
		nil,
		co.CLSCTX_INPROC_SERVER,
		&factory,
	); err != nil {
		return nil, win.SIZE{}, fmt.Errorf("create WIC factory: %w", err)
	}

	// SHCreateMemStream projects the IStream directly over jpegData's
	// backing array (no copy) — keep it alive through the calls below.
	defer runtime.KeepAlive(jpegData)
	stream, err := winsh.SHCreateMemStream(rel, jpegData)
	if err != nil {
		return nil, win.SIZE{}, fmt.Errorf("create stream: %w", err)
	}

	decoder, err := factory.CreateDecoderFromStream(
		rel, stream, nil, cowic.WICDEC_METADATACACHE_OnDemand)
	if err != nil {
		return nil, win.SIZE{}, fmt.Errorf("create decoder: %w", err)
	}

	frame, err := decoder.GetFrame(rel, 0)
	if err != nil {
		return nil, win.SIZE{}, fmt.Errorf("get frame: %w", err)
	}

	// StretchDIBits wants a fixed, known pixel layout, so convert whatever
	// format the source JPEG decoded to (typically 24bpp BGR, but not
	// guaranteed) into a consistent 32bpp BGR.
	converter, err := factory.CreateFormatConverter(rel)
	if err != nil {
		return nil, win.SIZE{}, fmt.Errorf("create format converter: %w", err)
	}
	if err = converter.Initialize(
		&frame.IWICBitmapSource,
		&cowic.WIC_PIXELFORMAT_32bppBGR,
		cowic.WICBMP_DITHER_None,
		nil,
		0,
		cowic.WICBMP_PAL_Custom,
	); err != nil {
		return nil, win.SIZE{}, fmt.Errorf("convert pixel format: %w", err)
	}

	sz, err := converter.GetSize()
	if err != nil {
		return nil, win.SIZE{}, fmt.Errorf("get size: %w", err)
	}
	if sz.Cx <= 0 || sz.Cy <= 0 {
		return nil, win.SIZE{}, fmt.Errorf("invalid image size %dx%d", sz.Cx, sz.Cy)
	}

	stride := int(sz.Cx) * 4 // 32bpp = 4 bytes/pixel
	buf := make([]byte, stride*int(sz.Cy))
	if err = converter.CopyPixels(nil, stride, len(buf), &buf[0]); err != nil {
		return nil, win.SIZE{}, fmt.Errorf("copy pixels: %w", err)
	}

	return buf, sz, nil
}

// coverSourceRect computes a centered crop of a srcW x srcH source image
// that matches the aspect ratio of a dstW x dstH destination box —
// equivalent to CSS's `object-fit: cover`. Stretching that returned
// sub-rectangle to fill the destination (rather than stretching the whole
// uncropped source) fills the box completely with no distortion, at the
// cost of cropping whichever dimension overflows.
//
// Returns the full source rectangle unchanged if any input is <= 0.
func coverSourceRect(srcW, srcH, dstW, dstH int32) (x, y, w, h int32) {
	if srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0 {
		return 0, 0, srcW, srcH
	}

	srcAspect := float64(srcW) / float64(srcH)
	dstAspect := float64(dstW) / float64(dstH)

	if srcAspect > dstAspect {
		// Source is relatively wider than the destination: the full
		// height is used, crop off the left/right edges.
		w = int32(float64(srcH) * dstAspect)
		if w > srcW {
			w = srcW
		}
		h = srcH
		x = (srcW - w) / 2
		y = 0
	} else {
		// Source is relatively taller than the destination: the full
		// width is used, crop off the top/bottom edges.
		h = int32(float64(srcW) / dstAspect)
		if h > srcH {
			h = srcH
		}
		w = srcW
		x = 0
		y = (srcH - h) / 2
	}
	return x, y, w, h
}
