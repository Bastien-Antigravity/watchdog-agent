package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/Bastien-Antigravity/watchdog-agent/src/core"

	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	toolbox_lifecycle "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	toolbox_teleclient "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/teleremote"
	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

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
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("🤖 Watchdog Agent (Uptime: %ds)\n\n", status.UptimeSeconds))
			for _, svc := range status.Services {
				state := "🔴 OFFLINE"
				if svc.Running {
					state = fmt.Sprintf("🟢 RUNNING (PID %d)", svc.PID)
				}
				sb.WriteString(fmt.Sprintf("• *%s*: %s\n", svc.Name, state))
			}
			m.logger.Info(sb.String())
			return nil
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

	// 3. Restart All
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
	port := 50051
	if teleCap.Port != "" {
		fmt.Sscanf(teleCap.Port, "%d", &port)
	}

	ip := "127.0.0.1"
	if teleCap.IP != "" {
		ip = teleCap.IP
	}

	teleClient := toolbox_teleclient.NewTeleClient("Watchdog Agent", ip, port, logger)
	mgr := NewMenuManager(teleClient, controller, logger)

	// Initial menu build
	mgr.RebuildMenu()

	teleClient.Start()

	lm.Register("TeleClient", func() error {
		teleClient.Close()
		return nil
	})
}
