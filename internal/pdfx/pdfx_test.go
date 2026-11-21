package pdfx

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"strings"
	"testing"

	"github.com/andriantam/ace-mate/internal/pdfx/pdfxtest"
)

func TestTextExtraction(t *testing.T) {
	got, err := Text(pdfxtest.TextPDF(pdfxtest.LongLines()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "distributed systems in Go") {
		t.Fatalf("missing text: %q", got)
	}
}

func TestScanHasNoText(t *testing.T) {
	if _, err := Text(pdfxtest.ScanPDF()); err == nil {
		t.Fatal("a scanned PDF must report no readable text")
	}
}

func TestRenderTextAndScan(t *testing.T) {
	for name, pdf := range map[string][]byte{"text": pdfxtest.TextPDF(pdfxtest.LongLines()), "scan": pdfxtest.ScanPDF()} {
		pages, total, err := Render(context.Background(), pdf, 4)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if total != 1 || len(pages) != 1 {
			t.Fatalf("%s: pages=%d total=%d", name, len(pages), total)
		}
		img, err := jpeg.Decode(bytes.NewReader(pages[0].JPEG))
		if err != nil {
			t.Fatalf("%s: not a jpeg: %v", name, err)
		}
		if b := img.Bounds(); b.Dx() < 800 || b.Dy() < 1000 {
			t.Fatalf("%s: page too small to read: %v", name, b)
		}
		if !hasDarkPixels(img) {
			t.Fatalf("%s: rendered page is blank", name)
		}
	}
}

func TestRenderPageCap(t *testing.T) {
	pdf := pdfxtest.BuildPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 3 0 R 3 0 R] /Count 3 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
	})
	pages, total, err := Render(context.Background(), pdf, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(pages) != 2 {
		t.Fatalf("got %d of %d pages, want 2 of 3", len(pages), total)
	}
}

func TestRenderRejectsGarbage(t *testing.T) {
	if _, _, err := Render(context.Background(), []byte("%PDF-1.4 not really"), 2); err == nil {
		t.Fatal("expected an error")
	}
}

func hasDarkPixels(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 3 {
		for x := b.Min.X; x < b.Max.X; x += 3 {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r < 0x6000 && g < 0x6000 && bl < 0x6000 {
				return true
			}
		}
	}
	return false
}
