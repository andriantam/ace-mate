// Package pdfxtest builds small PDFs for tests: one with a text layer and one that is
// image-only, like a scanned resume.
package pdfxtest

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"strings"
)

// BuildPDF writes a valid PDF (with a correct xref table) from numbered object bodies.
func BuildPDF(objs []string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

func Stream(dict, data string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(data), data)
}

// TextPDF has a real text layer.
func TextPDF(lines []string) []byte {
	var c strings.Builder
	c.WriteString("BT /F1 14 Tf 50 740 Td 18 TL\n")
	for _, l := range lines {
		fmt.Fprintf(&c, "(%s) Tj T*\n", l)
	}
	c.WriteString("ET")
	return BuildPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		Stream("", c.String()),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	})
}

// ScanPDF is image-only (no text layer), like a scanned resume.
func ScanPDF() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for i := range img.Pix {
		img.Pix[i] = 0x20
	}
	var jb bytes.Buffer
	_ = jpeg.Encode(&jb, img, nil)
	return BuildPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /XObject << /Im0 5 0 R >> >> >>",
		Stream("", "q 500 0 0 500 50 150 cm /Im0 Do Q"),
		Stream("/Type /XObject /Subtype /Image /Width 100 /Height 100 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode", jb.String()),
	})
}

func LongLines() []string {
	var out []string
	for i := 0; i < 12; i++ {
		out = append(out, fmt.Sprintf("Senior backend engineer building distributed systems in Go and Postgres, line %d", i))
	}
	return out
}
