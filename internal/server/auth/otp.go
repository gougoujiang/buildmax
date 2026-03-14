package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"buildmax/internal/server/httputil"
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

func (h *Handler) otpRequestHandler(w http.ResponseWriter, r *http.Request) {
	if h.cfg.UserStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "otp not configured")
		return
	}

	var req OtpRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "email required")
		return
	}
	intent := req.Intent
	if intent == "" {
		intent = "signup"
	}
	if intent != "signup" && intent != "login" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "intent must be signup or login")
		return
	}

	user, err := h.cfg.UserStore.UserByEmail(r.Context(), req.Email)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if intent == "login" {
		if user == nil {
			httputil.WriteJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, OtpRequestResponse{Message: "otp_sent"})
		return
	}

	// intent == "signup"
	if user != nil {
		httputil.WriteJSONError(w, http.StatusConflict, "email already registered")
		return
	}

	_, err = h.cfg.UserStore.CreateUser(r.Context(), req.Email, h.cfg.DefaultQuotaTier)
	if err != nil {
		if errors.Is(err, entity.ErrEmailExists) {
			httputil.WriteJSONError(w, http.StatusConflict, "email already registered")
			return
		}
		httputil.WriteJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, OtpRequestResponse{Message: "otp_sent"})
}
