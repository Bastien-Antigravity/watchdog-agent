package rest

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	unilog_interfaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
	"github.com/Bastien-Antigravity/watchdog-agent/src/core"
	"github.com/Bastien-Antigravity/watchdog-agent/src/supervisor"
)

//go:embed mfe.js
var mfeJS string

// RESTHandler handles HTTP management requests for watchdog
type RESTHandler struct {
	logger  unilog_interfaces.Logger
	control core.WatchdogController
}

// NewRESTHandler creates a new RESTHandler instance
func NewRESTHandler(control core.WatchdogController, logger unilog_interfaces.Logger) *RESTHandler {
	return &RESTHandler{
		logger:  logger,
		control: control,
	}
}

// RegisterRoutes registers the REST routes to the provided mux
func (h *RESTHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/status", h.handleStatus)
	mux.HandleFunc("/api/v1/watchdog/restart", h.handleRestartService)
	mux.HandleFunc("/api/v1/watchdog/restart_all", h.handleRestartAll)
	mux.HandleFunc("/api/v1/postgres/start", h.handleStartPostgres)

	// Serve embedded static files (OpenMFE Web Component bundles) with cache-busting headers
	mfeHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Write([]byte(mfeJS))
	}
	mux.HandleFunc("/static/js/mfe-loader.js", mfeHandler)
	mux.HandleFunc("/static/mfe.js", mfeHandler)
}

type httpStatusResponse struct {
	Healthy           bool                 `json:"healthy"`
	Status            string               `json:"status"`
	Version           string               `json:"version"`
	Timestamp         int64                `json:"timestamp"`
	UptimeSeconds     int64                `json:"uptime_seconds"`
	Services          []core.ServiceStatus `json:"services"`
	PostgresConnected bool                 `json:"postgres_connected"`
	PostgresAddr      string               `json:"postgres_addr"`
	RAGMCPConnected   bool                 `json:"rag_mcp_connected"`
	RAGMCPAddr        string               `json:"rag_mcp_addr"`
}

func (h *RESTHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	statusInfo, err := h.control.GetStatus(r.Context())
	if err != nil {
		h.sendJSON(w, httpStatusResponse{
			Healthy:   false,
			Status:    "Error",
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.sendJSON(w, httpStatusResponse{
		Healthy:           statusInfo.Healthy,
		Status:            statusInfo.Status,
		Version:           statusInfo.Version,
		Timestamp:         statusInfo.Timestamp,
		UptimeSeconds:     statusInfo.UptimeSeconds,
		Services:          statusInfo.Services,
		PostgresConnected: statusInfo.PostgresConnected,
		PostgresAddr:      statusInfo.PostgresAddr,
		RAGMCPConnected:   statusInfo.RAGMCPConnected,
		RAGMCPAddr:        statusInfo.RAGMCPAddr,
	})
}

type httpRestartRequest struct {
	Name string `json:"name"`
}

type httpControlResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

func (h *RESTHandler) handleRestartService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req httpRestartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := h.control.RestartService(r.Context(), req.Name)
	if err != nil {
		h.sendJSON(w, httpControlResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to restart service: %v", err),
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.sendJSON(w, httpControlResponse{
		Success:   true,
		Message:   fmt.Sprintf("Service '%s' restart triggered successfully", req.Name),
		Timestamp: time.Now().Unix(),
	})
}

func (h *RESTHandler) handleRestartAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := h.control.RestartAll(r.Context())
	if err != nil {
		h.sendJSON(w, httpControlResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to restart all services: %v", err),
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.sendJSON(w, httpControlResponse{
		Success:   true,
		Message:   "Restart all services triggered successfully",
		Timestamp: time.Now().Unix(),
	})
}

func (h *RESTHandler) sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// Handler returns HTTP multiplexer wrapped in CORS
func (h *RESTHandler) Handler() http.Handler {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (h *RESTHandler) handleStartPostgres(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	statusInfo, err := h.control.GetStatus(r.Context())
	if err != nil || statusInfo.PostgresAddr == "" {
		h.sendJSON(w, httpControlResponse{
			Success:   false,
			Message:   "Postgres address not resolved from capability configurations",
			Timestamp: time.Now().Unix(),
		})
		return
	}

	go supervisor.LaunchPostgresAttempt(statusInfo.PostgresAddr)

	h.sendJSON(w, httpControlResponse{
		Success:   true,
		Message:   "Postgres launch process triggered in background",
		Timestamp: time.Now().Unix(),
	})
}

// StartServer launches HTTP REST API server
func (h *RESTHandler) StartServer(port int) error {
	addr := fmt.Sprintf(":%d", port)
	h.logger.Info("Starting REST management server on %s", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      h.Handler(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return server.ListenAndServe()
}
