package api

import (
	"encoding/json"
	"net/http"

	"github.com/nazarioz/oidc-radius-bridge/internal/auth"
	"github.com/nazarioz/oidc-radius-bridge/pkg/logger"
)

// Handler manages HTTP requests from FreeRADIUS and implements the http.Handler interface
type Handler struct {
	authService auth.Service
	logger      *logger.Logger
	mux         *http.ServeMux
}

// AuthRequest represents the authentication request payload from FreeRADIUS
type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResponse represents the authentication response payload for FreeRADIUS
type AuthResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// NewHandler creates a new HTTP handler with the given dependencies
func NewHandler(authService auth.Service, logger *logger.Logger) *Handler {
	h := &Handler{
		authService: authService,
		logger:      logger,
		mux:         http.NewServeMux(),
	}
	h.RegisterRoutes(h.mux)
	return h
}

// RegisterRoutes sets up the HTTP routes for the handler
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/auth", h.handleAuth)
}

// ServeHTTP implements the http.Handler interface
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// handleAuth processes authentication requests from FreeRADIUS
func (h *Handler) handleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate Content-Type
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
		return
	}

	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate inputs
	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	// Authenticate with OIDC provider
	if err := h.authService.Authenticate(r.Context(), req.Username, req.Password); err != nil {
		h.logger.Error("Authentication failed for user %s: %v", req.Username, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(AuthResponse{
			Success: false,
			Message: "Authentication failed",
		})
		return
	}

	// Log success (without sensitive information)
	h.logger.Info("Successfully authenticated user: %s", req.Username)

	// Return success response to FreeRADIUS
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthResponse{
		Success: true,
		Message: "Authentication successful",
	})
}
