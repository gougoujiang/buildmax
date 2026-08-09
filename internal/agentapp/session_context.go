package agentapp

import "github.com/gougoujiang/buildmax/internal/core/session"

// SessionContext wraps a persisted session with runtime helpers.
type SessionContext struct {
	*session.Session
	selectedModel string
}

func NewSessionContext(sess *session.Session, defaultModel string) *SessionContext {
	if sess == nil {
		sess = session.NewSession("")
	}
	return &SessionContext{
		Session:       sess,
		selectedModel: defaultModel,
	}
}

// ModelName returns the selected model for this session, or the provided fallback.
func (s *SessionContext) ModelName(fallback string) string {
	if s == nil || s.Session == nil {
		return fallback
	}
	if s.selectedModel != "" {
		return s.selectedModel
	}
	return fallback
}

// SetModel updates the selected model for this session.
func (s *SessionContext) SetModel(name string) {
	if s == nil || s.Session == nil {
		return
	}
	s.selectedModel = name
}
