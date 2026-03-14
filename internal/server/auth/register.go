package auth

import (
	"net/http"
)

// Handler serves unauthenticated auth endpoints (login, OTP request).
type Handler struct {
	cfg Config
}

// NewHandler returns a handler for auth routes.
func NewHandler(cfg Config) *Handler {
	return &Handler{cfg: cfg}
}

// Register adds POST /api/otp/request and POST /api/login to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/otp/request", h.otpRequestHandler)
	mux.HandleFunc("POST /api/login", h.loginHandler)
}
