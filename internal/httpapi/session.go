package httpapi

import (
	"net/http"

	"github.com/andriantam/ace-mate/internal/config"
	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/engine"
	"github.com/andriantam/ace-mate/internal/legal"
	"github.com/andriantam/ace-mate/internal/version"
)

// ── session ──

type providerView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	providers := []providerView{}
	for _, p := range s.auth.Providers {
		providers = append(providers, providerView{p.ID, p.Name})
	}
	lg := s.cfg.Legal
	resp := map[string]any{
		"user":      nil,
		"providers": providers,
		"devLogin":  s.cfg.DevLogin,
		"version":   version.Version,
		"models": map[string]any{
			"documents": s.toolView(toolDocs),
			"journeys":  s.toolView(toolJourneys),
		},
		"legal": map[string]any{
			"version": legal.Version, "effective": legal.Effective, "operator": lg.Operator, "contact": lg.Contact,
			"hosting": lg.Hosting, "jurisdiction": lg.Jurisdiction, "minAge": lg.MinAge,
			"birthday": s.cfg.GoogleBirthday,
		},
	}
	if u := s.auth.CurrentUser(r); u != nil {
		resp["user"] = map[string]any{
			"email": u.Email, "name": u.Name, "givenName": u.GivenName, "image": u.Image,
			"termsAccepted": u.TermsVersion == legal.Version, "marketingOptIn": u.MarketingOptIn,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// toolView is the default model of a tool plus, when the operator configured several,
// the models a user can pick from.
func (s *Server) toolView(tool string) map[string]any {
	l, e := s.cfg.DocsLLM, s.docs
	if tool == toolJourneys {
		l, e = s.cfg.JourneyLLM, s.journeys
	}
	v := modelView(l, e)
	if list, sel := s.modelChoices(tool); list != nil {
		v["options"], v["defaultId"] = list, sel
	}
	return v
}

// modelView tells users which model a tool uses and where their data goes.
func modelView(l config.LLM, e *engine.Engine) map[string]any {
	if e == nil {
		return map[string]any{"enabled": false}
	}
	return map[string]any{
		"enabled": true, "provider": e.P.Name(), "model": l.Model, "displayName": l.DisplayName,
		"host": l.Host(), "privacy": l.Privacy, "vision": e.P.Caps().Vision,
	}
}

// ── consent ──

type consentReq struct {
	Accept    bool   `json:"accept"`
	Version   string `json:"version"`
	Marketing bool   `json:"marketing"`
}

func (s *Server) acceptTerms(w http.ResponseWriter, r *http.Request, u *db.User) {
	var req consentReq
	if !readJSON(w, r, &req) {
		return
	}
	if !req.Accept {
		writeErr(w, http.StatusBadRequest, "You need to accept the Terms of Service and Privacy Policy to use the app.")
		return
	}
	if req.Version != legal.Version {
		writeErr(w, http.StatusConflict, "The Terms changed while this page was open. Reload and review them again.")
		return
	}
	if err := s.db.AcceptTerms(r.Context(), u.ID, legal.Version, req.Marketing); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) setMarketing(w http.ResponseWriter, r *http.Request, u *db.User) {
	var req struct {
		OptIn bool `json:"optIn"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.db.SetMarketing(r.Context(), u.ID, req.OptIn); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
