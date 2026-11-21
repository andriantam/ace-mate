package docx

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func build(t *testing.T, body string) []byte {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.Create("word/document.xml")
	_, _ = w.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body + `</w:body></w:document>`))
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func para(s string) string { return `<w:p><w:r><w:t>` + s + `</w:t></w:r></w:p>` }

func TestText(t *testing.T) {
	long := strings.Repeat("Built payment services in Go and Postgres. ", 4)
	doc := build(t, para("Senior Engineer")+para(long)+`<w:p><w:r><w:t>Skills:</w:t><w:tab/><w:t>Go &amp; Kafka</w:t></w:r></w:p>`)
	if !IsDocx(doc) {
		t.Fatal("should be detected as docx")
	}
	got, err := Text(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Senior Engineer\n", "Skills:\tGo & Kafka"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestRejects(t *testing.T) {
	if IsDocx([]byte("%PDF-1.4")) {
		t.Error("a PDF is not a docx")
	}
	if _, err := Text(build(t, para("hi"))); err == nil {
		t.Error("an almost empty document should be rejected")
	}
}
