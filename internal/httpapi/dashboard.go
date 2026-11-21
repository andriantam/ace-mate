package httpapi

import (
	"net/http"
	"net/url"
	"time"

	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/readiness"
)

// ── dashboard ──

type jobCard struct {
	ID        string                   `json:"id"`
	Title     string                   `json:"title"`
	Company   string                   `json:"company"`
	Status    string                   `json:"status"`
	CreatedAt time.Time                `json:"createdAt"`
	Overall   float64                  `json:"overall"`
	Aspects   []readiness.AspectResult `json:"aspects"`
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request, u *db.User) {
	profile, err := s.db.GetProfile(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	list, err := s.db.ListJobs(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	cards := []jobCard{}
	var sum float64
	var ready int
	for _, j := range list {
		c := jobCard{ID: j.ID, Title: j.Title, Company: j.Company, Status: j.Status, CreatedAt: j.CreatedAt, Aspects: []readiness.AspectResult{}}
		if c.Company == "" && j.URL != "" {
			if pu, err := url.Parse(j.URL); err == nil {
				c.Company = pu.Hostname()
			}
		}
		if j.Journey != nil {
			c.Overall = readiness.Overall(*j.Journey, j.Progress)
			c.Aspects = readiness.Aspects(*j.Journey, j.Progress)
			sum += c.Overall
			ready++
		}
		cards = append(cards, c)
	}
	avg := 0.0
	if ready > 0 {
		avg = sum / float64(ready)
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": profile, "jobs": cards, "average": avg, "journeys": ready})
}
