// internal/models/registry.go
package models

import (
	"roundtable/internal/config"
)

// Registry holds all available models
type Registry struct {
	models map[string]Model
	order  []string // Preserve order for consistent display
}

// NewRegistry creates a registry from config
func NewRegistry(cfg *config.Config) *Registry {
	r := &Registry{
		models: make(map[string]Model),
		order:  []string{},
	}

	// Add Claude if enabled
	if cfg.Models.Claude.Enabled {
		claude := NewClaude(cfg.Models.Claude.CLIPath, cfg.Models.Claude.DefaultModel)
		r.models["claude"] = claude
		r.order = append(r.order, "claude")
	}

	// Add Gemini if enabled
	if cfg.Models.Gemini.Enabled {
		gemini := NewGemini(cfg.Models.Gemini.CLIPath)
		r.models["gemini"] = gemini
		r.order = append(r.order, "gemini")
	}

	// Add GPT if enabled and has API key
	if cfg.Models.GPT.Enabled && cfg.Models.GPT.APIKey != "" {
		gpt := NewGPT(cfg.Models.GPT.APIKey, cfg.Models.GPT.DefaultModel)
		r.models["gpt"] = gpt
		r.order = append(r.order, "gpt")
	}

	// Add Grok if enabled and has API key
	if cfg.Models.Grok.Enabled && cfg.Models.Grok.APIKey != "" {
		grok := NewGrok(cfg.Models.Grok.APIKey, cfg.Models.Grok.DefaultModel)
		r.models["grok"] = grok
		r.order = append(r.order, "grok")
	}

	return r
}

// Get returns a model by ID
func (r *Registry) Get(id string) Model {
	return r.models[id]
}

// All returns all models in order
func (r *Registry) All() []Model {
	result := make([]Model, 0, len(r.order))
	for _, id := range r.order {
		if m, ok := r.models[id]; ok {
			result = append(result, m)
		}
	}
	return result
}

// Enabled returns IDs of all enabled models
func (r *Registry) Enabled() []string {
	return r.order
}

// Count returns number of enabled models
func (r *Registry) Count() int {
	return len(r.order)
}
