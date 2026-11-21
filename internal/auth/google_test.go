package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

type rewrite struct{ to string }

func (rw rewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Scheme, r2.URL.Host = "http", strings.TrimPrefix(rw.to, "http://")
	return http.DefaultTransport.RoundTrip(r2)
}

func TestGoogleBirthday(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"full date":    {200, `{"birthdays":[{"metadata":{"primary":true},"date":{"year":1990,"month":4,"day":7}}]}`, "1990-04-07"},
		"no year":      {200, `{"birthdays":[{"metadata":{"primary":true},"date":{"month":12,"day":31}}]}`, "--12-31"},
		"prefers year": {200, `{"birthdays":[{"date":{"month":4,"day":7}},{"date":{"year":1990,"month":4,"day":7}}]}`, "1990-04-07"},
		"none":         {200, `{}`, ""},
		"refused":      {403, `{}`, ""},
		"bad month":    {200, `{"birthdays":[{"date":{"year":1990,"month":13,"day":7}}]}`, ""},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				t.Errorf("%s: missing bearer token", name)
			}
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		got, err := googleBirthday(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "tok"}), &http.Client{Transport: rewrite{srv.URL}})
		srv.Close()
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q err %v, want %q", name, got, err, tc.want)
		}
	}
}
