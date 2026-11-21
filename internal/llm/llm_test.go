package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                 `{"a":1}`,
		"```json\n{\"a\":1}\n```": `{"a":1}`,
		"<think>hmm {not json}</think>\n{\"a\":1}":  `{"a":1}`,
		"Sure! Here you go: {\"a\":{\"b\":2}} done": `{"a":{"b":2}}`,
	}
	for in, want := range cases {
		if got := ExtractJSON(in); got != want {
			t.Errorf("ExtractJSON(%q) = %q, want %q", in, got, want)
		}
	}
}

func fakeServer(t *testing.T, reply string, finish string, gotBody *map[string]any, hits *atomic.Int32, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if gotBody != nil {
			_ = json.Unmarshal(b, gotBody)
		}
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": reply}, "finish_reason": finish}}})
	}))
}

func TestOpenAIStructuredModes(t *testing.T) {
	schema := map[string]any{"type": "object"}
	for _, tc := range []struct {
		mode     StructuredMode
		wantType string // response_format.type, "" means absent
	}{
		{StructuredSchema, "json_schema"},
		{StructuredObject, "json_object"},
		{StructuredPrompt, ""},
	} {
		var body map[string]any
		srv := fakeServer(t, `{"ok":true}`, "stop", &body, nil, 0)
		p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL + "/v1", Model: "m", Structured: tc.mode, APIKey: "k"})
		out, err := p.Complete(context.Background(), Request{System: "sys", Parts: []Part{{Text: "hello"}}, Schema: schema, MaxTokens: 100})
		srv.Close()
		if err != nil || out != `{"ok":true}` {
			t.Fatalf("%s: %q %v", tc.mode, out, err)
		}
		rf, _ := body["response_format"].(map[string]any)
		got, _ := rf["type"].(string)
		if got != tc.wantType {
			t.Errorf("%s: response_format.type = %q, want %q", tc.mode, got, tc.wantType)
		}
		msgs := body["messages"].([]any)
		if msgs[0].(map[string]any)["role"] != "system" || msgs[1].(map[string]any)["content"] != "hello" {
			t.Errorf("%s: messages = %v", tc.mode, msgs)
		}
	}
}

func TestOpenAIErrorsAreCategorised(t *testing.T) {
	for _, tc := range []struct {
		status int
		kind   Kind
	}{{401, KindAuth}, {400, KindBadRequest}} {
		srv := fakeServer(t, "", "", nil, nil, tc.status)
		p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL + "/v1", Model: "m", Structured: StructuredPrompt})
		_, err := p.Complete(context.Background(), Request{Parts: []Part{{Text: "x"}}})
		srv.Close()
		if KindOf(err) != tc.kind {
			t.Errorf("status %d: kind = %v, want %v (%v)", tc.status, KindOf(err), tc.kind, err)
		}
	}
	srv := fakeServer(t, `{"a":`, "length", nil, nil, 0)
	defer srv.Close()
	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL + "/v1", Model: "m", Structured: StructuredPrompt})
	if _, err := p.Complete(context.Background(), Request{Parts: []Part{{Text: "x"}}}); KindOf(err) != KindTruncated {
		t.Errorf("finish_reason=length should be KindTruncated, got %v", err)
	}
}

func TestOpenAIRetriesRateLimit(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(429)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": "{}"}, "finish_reason": "stop"}}})
	}))
	defer srv.Close()
	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL + "/v1", Model: "m", Structured: StructuredPrompt})
	if _, err := p.Complete(context.Background(), Request{Parts: []Part{{Text: "x"}}}); err != nil {
		t.Fatalf("expected a retry to succeed: %v", err)
	}
	if hits.Load() != 2 {
		t.Errorf("hits = %d, want 2", hits.Load())
	}
}

func TestNoAuthHeaderWithoutKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("Authorization header must be omitted when no key is set (local servers)")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": "{}"}, "finish_reason": "stop"}}})
	}))
	defer srv.Close()
	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL + "/v1", Model: "m", Structured: StructuredPrompt})
	_, _ = p.Complete(context.Background(), Request{Parts: []Part{{Text: strings.Repeat("x", 3)}}})
}

func TestOpenAISendsImagesAsImageURL(t *testing.T) {
	var body map[string]any
	srv := fakeServer(t, `{"ok":true}`, "stop", &body, nil, 0)
	defer srv.Close()
	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL + "/v1", Model: "vl", Structured: StructuredPrompt, Vision: true})
	if !p.Caps().Vision {
		t.Fatal("Vision should be reported")
	}
	if _, err := p.Complete(context.Background(), Request{System: "s", Parts: []Part{{Text: "read this"}, {Image: []byte("jpegbytes")}}}); err != nil {
		t.Fatal(err)
	}
	msgs := body["messages"].([]any)
	content := msgs[1].(map[string]any)["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("content parts = %d", len(content))
	}
	img := content[1].(map[string]any)
	url := img["image_url"].(map[string]any)["url"].(string)
	if img["type"] != "image_url" || !strings.HasPrefix(url, "data:image/jpeg;base64,") {
		t.Fatalf("image part = %v", img)
	}
}
