package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Rate limits are kept in memory, so with several server instances each one limits on
// its own. The daily journey limit is counted in the database and holds across instances.

type limiter struct {
	mu    sync.Mutex
	every rate.Limit
	burst int
	m     map[string]*entry
}

type entry struct {
	l    *rate.Limiter
	seen time.Time
}

func newLimiter(every rate.Limit, burst int) *limiter {
	l := &limiter{every: every, burst: burst, m: map[string]*entry{}}
	go func() {
		for range time.Tick(5 * time.Minute) {
			l.mu.Lock()
			for k, e := range l.m {
				if time.Since(e.seen) > 10*time.Minute {
					delete(l.m, k)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.m[key]
	if !ok {
		e = &entry{l: rate.NewLimiter(l.every, l.burst)}
		l.m[key] = e
	}
	e.seen = time.Now()
	return e.l.Allow()
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if f := r.Header.Get("Fly-Client-IP"); f != "" {
			return f
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) limitAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authLimit.allow(s.clientIP(r)) {
			writeErr(w, http.StatusTooManyRequests, "Too many attempts. Slow down.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
