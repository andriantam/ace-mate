package pdfx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
)

// Rendering uses PDFium compiled to WebAssembly and run by wazero, so it is pure Go
// (no cgo) and the single static binary stays single and static.
//
// The PDF is untrusted input. The WebAssembly sandbox is given no file system
// (the library's default would mount the host's root), no network and no environment.

const (
	targetEdge  = 1600 // px on the page's long edge: small print stays legible and the upload stays small
	minDPI      = 72
	maxDPI      = 200
	jpegQuality = 88
)

var (
	poolOnce sync.Once
	pool     pdfium.Pool
	poolErr  error
)

func getPool() (pdfium.Pool, error) {
	poolOnce.Do(func() {
		pool, poolErr = webassembly.Init(webassembly.Config{
			MinIdle: 0, MaxIdle: 1, MaxTotal: 2,
			FSConfig: wazero.NewFSConfig(), // no mounts: the sandbox cannot see the host file system
		})
	})
	return pool, poolErr
}

// Page is one rendered page as a JPEG.
type Page struct {
	JPEG []byte
}

// Render draws up to maxPages pages of the PDF as images for a vision model.
// total is the number of pages in the document, so callers can say when pages were skipped.
func Render(ctx context.Context, pdf []byte, maxPages int) (pages []Page, total int, err error) {
	p, err := getPool()
	if err != nil {
		return nil, 0, fmt.Errorf("pdf renderer unavailable: %w", err)
	}
	inst, err := p.GetInstance(30 * time.Second)
	if err != nil {
		return nil, 0, fmt.Errorf("pdf renderer busy: %w", err)
	}
	defer inst.Close()

	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &pdf})
	if err != nil {
		return nil, 0, errors.New("could not open that PDF to render it")
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})

	pc, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil || pc.PageCount < 1 {
		return nil, 0, errors.New("that PDF has no pages")
	}
	total = pc.PageCount

	for i := 0; i < min(total, maxPages); i++ {
		if err := ctx.Err(); err != nil {
			return nil, total, err
		}
		jpg, err := renderPage(inst, doc.Document, i)
		if err != nil {
			return nil, total, err
		}
		pages = append(pages, Page{JPEG: jpg})
	}
	return pages, total, nil
}

func renderPage(inst pdfium.Pdfium, doc references.FPDF_DOCUMENT, index int) ([]byte, error) {
	page := requests.Page{ByIndex: &requests.PageByIndex{Document: doc, Index: index}}
	dpi := 130
	if sz, err := inst.GetPageSize(&requests.GetPageSize{Page: page}); err == nil && sz.Width > 0 && sz.Height > 0 {
		dpi = int(targetEdge / (max(sz.Width, sz.Height) / 72))
	}
	dpi = min(max(dpi, minDPI), maxDPI)
	r, err := inst.RenderPageInDPI(&requests.RenderPageInDPI{DPI: dpi, Page: page})
	if err != nil {
		return nil, fmt.Errorf("could not render page %d", index+1)
	}
	defer r.Cleanup()

	img := r.Result.RenderedImage
	if img == nil {
		return nil, fmt.Errorf("could not render page %d", index+1)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flatten(img), &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("could not encode page %d", index+1)
	}
	return buf.Bytes(), nil
}

// flatten copies the pixels out of the renderer's buffer (valid only until Cleanup)
// so encoding never reads freed memory.
func flatten(src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}
	return dst
}
