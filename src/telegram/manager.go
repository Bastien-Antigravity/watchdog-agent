package telegram

// =============================================================================
// ESSENTIAL PROCESS:
// Telegram bot UI integration and menu management for watchdog-agent.
// Dynamically constructs and binds interactive buttons, node telemetry status,
// and process restart callbacks into the tele-remote bot interface.
//
// DATA FLOW:
// 1. Reads tele-remote capability gRPC listen address from AppConfig.
// 2. Connects TeleClient facade and constructs hierarchical menu tree.
// 3. Registers button callbacks to restart individual processes or the full stack.
// 4. Pushes menu schema updates asynchronously to tele-remote.
//
// KEY PARAMETERS:
// - appConfig: Distributed configuration facade for tele_remote address resolution.
// - controller: core.WatchdogController interface for querying status and triggers.
// =============================================================================

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Bastien-Antigravity/watchdog-agent/src/core"
	"github.com/Bastien-Antigravity/watchdog-agent/src/supervisor"

	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	toolbox_lifecycle "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	toolbox_teleclient "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/teleremote"
	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// -----------------------------------------------------------------------------

// MenuManager orchestrates the rebuild operations of the Telegram interactive menus.
type MenuManager struct {
	tc         *toolbox_teleclient.TeleClient
	controller core.WatchdogController
	logger     unilog_ifaces.Logger
}

// NewMenuManager creates a new MenuManager.
func NewMenuManager(tc *toolbox_teleclient.TeleClient, controller core.WatchdogController, logger unilog_ifaces.Logger) *MenuManager {
	return &MenuManager{
		tc:         tc,
		controller: controller,
		logger:     logger,
	}
}

// RebuildMenu dynamically pulls status updates and registers actions
func (m *MenuManager) RebuildMenu() {
	status, err := m.controller.GetStatus(context.Background())
	if err != nil {
		m.logger.Error("Telegram Menu: failed to get status: %v", err)
		return
	}

	var actions []toolbox_teleclient.Action

	// 1. Health status check
	actions = append(actions, toolbox_teleclient.Action{
		Label: "📊 Fleet Status",
		Callback: func(input string) error {
			// Query current live status rather than boot-time closure capture [WD-06]
			currentStatus, err := m.controller.GetStatus(context.Background())
			if err != nil {
				return err
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("🤖 Watchdog Agent (Uptime: %ds)\n\n", currentStatus.UptimeSeconds))
			for _, svc := range currentStatus.Services {
				state := "🔴 OFFLINE"
				if svc.Paused {
					if svc.InCrashLoop {
						state = "🚨 CRASH LOOP (PAUSED)"
					} else {
						state = "⏸️ PAUSED"
					}
				} else if svc.Running {
					state = fmt.Sprintf("🟢 RUNNING (PID %d)", svc.PID)
				}
				sb.WriteString(fmt.Sprintf("• *%s*: %s\n", svc.Name, state))
			}
			return m.tc.SendTelemetry(sb.String())
		},
	})

	// 2. Restart Specific Service submenu
	var restartServiceActions []toolbox_teleclient.Action
	for _, svc := range status.Services {
		name := svc.Name
		restartServiceActions = append(restartServiceActions, toolbox_teleclient.Action{
			Label: fmt.Sprintf("🔄 Restart %s", name),
			Callback: func(input string) error {
				m.logger.Info("Telegram trigger: restarting service '%s'...", name)
				err := m.controller.RestartService(context.Background(), name)
				if err != nil {
					return err
				}
				// Refresh the menu since process stats updated
				m.RebuildMenu()
				return nil
			},
		})
	}
	actions = append(actions, toolbox_teleclient.Action{
		Label:   "🔌 Restart Service",
		SubMenu: restartServiceActions,
	})

	// 3. Pause / Resume Supervision submenu
	var supervisionActions []toolbox_teleclient.Action
	for _, svc := range status.Services {
		if svc.Name == "rag-mcp" {
			continue // virtual service
		}
		name := svc.Name
		isPaused := svc.Paused
		if isPaused {
			supervisionActions = append(supervisionActions, toolbox_teleclient.Action{
				Label: fmt.Sprintf("▶️ Resume %s", name),
				Callback: func(input string) error {
					m.logger.Info("Telegram trigger: resuming service '%s'...", name)
					err := m.controller.ResumeService(context.Background(), name)
					if err != nil {
						return err
					}
					m.RebuildMenu()
					return nil
				},
			})
		} else {
			supervisionActions = append(supervisionActions, toolbox_teleclient.Action{
				Label: fmt.Sprintf("⏸️ Pause %s", name),
				Callback: func(input string) error {
					m.logger.Info("Telegram trigger: pausing service '%s'...", name)
					err := m.controller.PauseService(context.Background(), name)
					if err != nil {
						return err
					}
					m.RebuildMenu()
					return nil
				},
			})
		}
	}
	actions = append(actions, toolbox_teleclient.Action{
		Label:   "⏸️ Pause / Resume",
		SubMenu: supervisionActions,
	})

	// 4. Restart All
	actions = append(actions, toolbox_teleclient.Action{
		Label: "⚠️ Restart All Services",
		Callback: func(input string) error {
			m.logger.Info("Telegram trigger: restarting all managed services...")
			err := m.controller.RestartAll(context.Background())
			if err != nil {
				return err
			}
			m.RebuildMenu()
			return nil
		},
	})

	m.tc.UpdateActions(actions)
	m.tc.PushMenuUpdate()
}

// SetupTelegram initializes the Tele-Remote client, binds dynamic updates, and registers with Lifecycle Manager.
func SetupTelegram(appConfig *toolbox_config.AppConfig, controller core.WatchdogController, logger unilog_ifaces.Logger, lm *toolbox_lifecycle.Manager) {
	var teleCap struct {
		IP   string `json:"ip"`
		Port string `json:"port"`
	}
	if err := appConfig.GetCapability("tele_remote", &teleCap); err != nil {
		logger.Warning("Tele-Remote capability not found or configured: %v", err)
		return
	}
	port := 1863
	if teleCap.Port != "" {
		fmt.Sscanf(teleCap.Port, "%d", &port)
	}

	ip := "127.0.0.1"
	if teleCap.IP != "" {
		ip = teleCap.IP
	}

	teleClient := toolbox_teleclient.NewTeleClient("Watchdog Agent", ip, port, logger)
	mgr := NewMenuManager(teleClient, controller, logger)

	// Hook supervisor circuit breaker alerts to Telegram
	supervisor.OnCircuitBreakerTripped = func(svc *supervisor.Service, crashCount int, window time.Duration) {
		mgr.RebuildMenu()
		_ = teleClient.SendTelemetry(fmt.Sprintf("🚨 *CRITICAL ALERT*: Service *%s* crashed %d times within %v!\nSupervisor tripped circuit breaker to *PAUSED (CrashLoopBackoff)*.\nUse Telegram menu or Web UI to inspect and resume.", svc.Name, crashCount, window))
	}

	// Initial menu build
	mgr.RebuildMenu()

	teleClient.Start()

	lm.Register("TeleClient", func() error {
		teleClient.Close()
		return nil
	})
}
