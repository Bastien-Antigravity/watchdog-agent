package core

import "context"

// ServiceStatus represents the current status of a managed process
type ServiceStatus struct {
	Name    string   `json:"name"`
	Running bool     `json:"running"`
	PID     int      `json:"pid,omitempty"`
	Deps    []string `json:"deps"`
}

// WatchdogStatusInfo represents the overall watchdog telemetry
type WatchdogStatusInfo struct {
	Healthy           bool            `json:"healthy"`
	Status            string          `json:"status"`
	Version           string          `json:"version"`
	Timestamp         int64           `json:"timestamp"`
	UptimeSeconds     int64           `json:"uptime_seconds"`
	Services          []ServiceStatus `json:"services"`
	PostgresConnected bool            `json:"postgres_connected"`
	PostgresAddr      string          `json:"postgres_addr"`
	RAGMCPConnected   bool            `json:"rag_mcp_connected"`
	RAGMCPAddr        string          `json:"rag_mcp_addr"`
}

// WatchdogController defines status and control interfaces for watchdog-agent
type WatchdogController interface {
	GetStatus(ctx context.Context) (WatchdogStatusInfo, error)
	RestartService(ctx context.Context, name string) error
	RestartAll(ctx context.Context) error
}
