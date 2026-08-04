package server

import (
	"context"
	"fmt"
	"time"

	"github.com/Bastien-Antigravity/watchdog-agent/src/core"
	"github.com/Bastien-Antigravity/watchdog-agent/src/supervisor"
	"github.com/Bastien-Antigravity/watchdog-agent/src/utils"
)

// Ensure Controller implements core.WatchdogController
var _ core.WatchdogController = (*Controller)(nil)

// Controller implements core.WatchdogController interface
type Controller struct {
	startTime    time.Time
	postgresAddr string
	ragMcpAddr   string
}

// NewController creates a new Controller instance
func NewController(postgresAddr, ragMcpAddr string) *Controller {
	return &Controller{
		startTime:    time.Now(),
		postgresAddr: postgresAddr,
		ragMcpAddr:   ragMcpAddr,
	}
}

// GetStatus retrieves the status metadata of all supervised services
func (c *Controller) GetStatus(ctx context.Context) (core.WatchdogStatusInfo, error) {
	var statuses []core.ServiceStatus
	for _, svc := range supervisor.Services {
		svc.Mu.Lock()
		pid := 0
		if svc.Running && svc.Cmd != nil && svc.Cmd.Process != nil {
			pid = svc.Cmd.Process.Pid
		}
		statuses = append(statuses, core.ServiceStatus{
			Name:    svc.Name,
			Running: svc.Running,
			PID:     pid,
			Deps:    svc.Deps,
		})
		svc.Mu.Unlock()
	}

	pgConnected := supervisor.IsPortListening(c.postgresAddr, 500*time.Millisecond)
	mcpConnected := supervisor.IsPortListening(c.ragMcpAddr, 500*time.Millisecond)

	// Append external/virtual status entries so they appear in the dashboard's service table
	statuses = append(statuses, core.ServiceStatus{
		Name:    "timescale-db",
		Running: pgConnected,
		PID:     0,
		Deps:    []string{},
	})

	statuses = append(statuses, core.ServiceStatus{
		Name:    "rag-mcp",
		Running: mcpConnected,
		PID:     0,
		Deps:    []string{},
	})

	return core.WatchdogStatusInfo{
		Healthy:           true,
		Status:            "Running",
		Version:           "1.0.0",
		Timestamp:         time.Now().Unix(),
		UptimeSeconds:     int64(time.Since(c.startTime).Seconds()),
		Services:          statuses,
		PostgresConnected: pgConnected,
		PostgresAddr:      c.postgresAddr,
		RAGMCPConnected:   mcpConnected,
		RAGMCPAddr:        c.ragMcpAddr,
	}, nil
}

// RestartService forcefully terminates the process group of a service to let it auto-restart
func (c *Controller) RestartService(ctx context.Context, name string) error {
	svc := supervisor.FindServiceByName(name)
	if svc == nil {
		return fmt.Errorf("service not found: %s", name)
	}

	svc.Mu.Lock()
	defer svc.Mu.Unlock()

	if svc.Running && svc.Cmd != nil && svc.Cmd.Process != nil {
		supervisor.LogInfo("watchdog", "Request to restart service '%s' (killing process group PID %d)...", svc.Name, svc.Cmd.Process.Pid)
		utils.KillProcessGroupByID(svc.Cmd.Process.Pid)
	}

	return nil
}

// RestartAll kills all running processes so they all restart sequentially
func (c *Controller) RestartAll(ctx context.Context) error {
	supervisor.LogInfo("watchdog", "Request to restart all managed services...")
	supervisor.KillAll()
	return nil
}
