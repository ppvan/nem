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

	stride := int(sz.Cx) * 4
	buf := make([]byte, stride*int(sz.Cy))
	if err = converter.CopyPixels(nil, stride, len(buf), &buf[0]); err != nil {
		return nil, win.SIZE{}, fmt.Errorf("copy pixels: %w", err)
	}

	return buf, sz, nil
}

func coverSourceRect(srcW, srcH, dstW, dstH int32) (x, y, w, h int32) {
	if srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0 {
		return 0, 0, srcW, srcH
	}

	srcAspect := float64(srcW) / float64(srcH)
	dstAspect := float64(dstW) / float64(dstH)

	if srcAspect > dstAspect {

		w = int32(float64(srcH) * dstAspect)
		if w > srcW {
			w = srcW
		}
		h = srcH
		x = (srcW - w) / 2
		y = 0
	} else {

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
