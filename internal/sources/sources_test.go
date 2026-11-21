package sources

import (
	"context"
	"testing"
)

func TestRejectsUnsafeLinks(t *testing.T) {
	for _, raw := range []string{"http://github.com/x", "https://127.0.0.1/", "https://localhost/me", "https://example.com:8443/", "ftp://example.com"} {
		if _, err := Read(context.Background(), raw); err == nil {
			t.Errorf("%s should be refused", raw)
		}
	}
}
