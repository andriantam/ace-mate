package httpapi

import (
	"github.com/andriantam/ace-mate/internal/config"
	"github.com/andriantam/ace-mate/internal/engine"
)

const (
	toolDocs     = "docs"
	toolJourneys = "journeys"
	defaultID    = "default"
)

// Option is a model users can pick for one upload or one job application.
type Option struct {
	ID     string
	LLM    config.LLM
	Engine *engine.Engine
}

// pick returns the engine and settings for a tool and the id the user chose. An empty id
// or "default" is the site default, which is also what every task created before the
// picker existed used. ok is false for an id that is unknown or whose model isn't configured.
func (s *Server) pick(tool, id string) (*engine.Engine, config.LLM, bool) {
	def, defLLM := s.docs, s.cfg.DocsLLM
	if tool == toolJourneys {
		def, defLLM = s.journeys, s.cfg.JourneyLLM
	}
	if id == "" || id == defaultID {
		return def, defLLM, def != nil
	}
	for _, o := range s.options {
		if o.ID == id && o.Engine != nil {
			return o.Engine, o.LLM, true
		}
	}
	return nil, config.LLM{}, false
}

// engineView is one pickable model as the UI sees it.
func engineView(id string, l config.LLM, e *engine.Engine) map[string]any {
	v := modelView(l, e)
	v["id"] = id
	return v
}

// modelChoices lists what a user may pick for a tool, and which entry is pre-selected.
// It returns nothing when there is only one model, so the UI shows no picker.
func (s *Server) modelChoices(tool string) (list []map[string]any, selected string) {
	def, defLLM := s.docs, s.cfg.DocsLLM
	if tool == toolJourneys {
		def, defLLM = s.journeys, s.cfg.JourneyLLM
	}
	selected = defaultID
	for _, o := range s.options {
		if o.Engine == nil {
			continue
		}
		if def != nil && defLLM.Same(o.LLM) {
			selected = o.ID
		}
		list = append(list, engineView(o.ID, o.LLM, o.Engine))
	}
	if def != nil && selected == defaultID {
		list = append([]map[string]any{engineView(defaultID, defLLM, def)}, list...)
	}
	if len(list) < 2 {
		return nil, selected
	}
	return list, selected
}

// engineID is the id stored with a task or job: "" means the site default.
func engineID(id string) string {
	if id == defaultID {
		return ""
	}
	return id
}

func engineIDOrDefault(id string) string {
	if id == "" {
		return defaultID
	}
	return id
}
