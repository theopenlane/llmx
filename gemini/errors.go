package gemini

import "errors"

var (
	// ErrConfigInvalid is returned when the config lacks an api key, or a project and location on Vertex AI
	ErrConfigInvalid = errors.New("gemini: api key, or project and location for vertex, are required")
	// ErrPromptBlocked is returned when Gemini refuses the prompt outright and sends no response
	ErrPromptBlocked = errors.New("gemini: prompt blocked")
	// ErrForeignDocument is returned when a request carries a document uploaded through another provider
	ErrForeignDocument = errors.New("gemini: document was not uploaded through this provider")
)
