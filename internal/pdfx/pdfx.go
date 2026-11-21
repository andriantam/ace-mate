// Package pdfx extracts plain text from PDFs for models that can't read them natively.
package pdfx

import (
	"bytes"
	"errors"
	"io"

	"github.com/ledongthuc/pdf"
)

const maxChars = 60_000

func Text(b []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", errors.New("could not read that PDF")
	}
	plain, err := r.GetPlainText()
	if err != nil {
		return "", errors.New("could not read text from that PDF")
	}
	raw, err := io.ReadAll(io.LimitReader(plain, maxChars*4))
	if err != nil {
		return "", errors.New("could not read text from that PDF")
	}
	s := string(bytes.TrimSpace(raw))
	if len(s) < 100 {
		return "", errors.New("that PDF has no readable text (is it a scan?). Try a text-based PDF")
	}
	if len(s) > maxChars {
		s = s[:maxChars]
	}
	return s, nil
}
