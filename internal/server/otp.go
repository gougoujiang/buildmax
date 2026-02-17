package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"buildmax/internal/storage/entity"
)

// OtpRequestRequest is the JSON body for POST /api/otp/request.
type OtpRequestRequest struct {
	Email  string `json:"email"`
	Intent string `json:"intent"` // "signup" or "login"; default "signup"
}

// OtpRequestResponse is the JSON body for a successful OTP request.
type OtpRequestResponse struct {
	Message string `json:"message"`
}

func (s *Server) otpRequestHandler(w http.ResponseWriter, r *http.Request) {
	if s.cfg.UserStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "otp not configured")
		return
	}

	var req OtpRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		writeJSONError(w, http.StatusBadRequest, "email required")
		return
	}
	intent := req.Intent
	if intent == "" {
		intent = "signup"
	}
	if intent != "signup" && intent != "login" {
		writeJSONError(w, http.StatusBadRequest, "intent must be signup or login")
		return
	}

	user, err := s.cfg.UserStore.UserByEmail(r.Context(), req.Email)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if intent == "login" {
		if user == nil {
			writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, OtpRequestResponse{Message: "otp_sent"})
		return
	}

	// intent == "signup"
	if user != nil {
		writeJSONError(w, http.StatusConflict, "email already registered")
		return
	}

	_, err = s.cfg.UserStore.CreateUser(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, entity.ErrEmailExists) {
			writeJSONError(w, http.StatusConflict, "email already registered")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, OtpRequestResponse{Message: "otp_sent"})
}
