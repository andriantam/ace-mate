// Package docx extracts plain text from Word (.docx) files. Old binary .doc files are
// not supported.
package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

const (
	maxXML   = 20 << 20 // uncompressed document.xml cap (zip-bomb guard)
	maxChars = 60_000
)

// IsDocx reports whether b looks like a Word document (a zip with word/document.xml).
func IsDocx(b []byte) bool {
	if !bytes.HasPrefix(b, []byte("PK\x03\x04")) {
		return false
	}
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return false
	}
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			return true
		}
	}
	return false
}

// Text returns the document's text, one paragraph per line.
func Text(b []byte) (string, error) {
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", errors.New("could not open that Word document")
	}
	var doc *zip.File
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			doc = f
		}
	}
	if doc == nil {
		return "", errors.New("that file isn't a Word document")
	}
	rc, err := doc.Open()
	if err != nil {
		return "", errors.New("could not open that Word document")
	}
	defer rc.Close()

	var sb strings.Builder
	dec := xml.NewDecoder(io.LimitReader(rc, maxXML))
	inText := false
	for sb.Len() < maxChars*2 {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", errors.New("could not read that Word document")
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "tab":
				sb.WriteByte('\t')
			case "br", "cr":
				sb.WriteByte('\n')
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				sb.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		}
	}
	s := strings.TrimSpace(sb.String())
	if len(s) < 100 {
		return "", errors.New("that Word document has almost no text")
	}
	if len(s) > maxChars {
		s = s[:maxChars]
	}
	return s, nil
}
