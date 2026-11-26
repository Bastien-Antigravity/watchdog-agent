package main

// =============================================================================
// ESSENTIAL PROCESS:
// Bastien-Antigravity Watchdog Agent main entry point. Orchestrates native host
// microservices, heals ecosystem configuration symlinks, manages node single-instance
// locking, publishes NATS telemetry, and exposes the HTTP REST / OpenMFE management portal.
//
// DATA FLOW:
// 1. Initialises AppConfig and UniLog via microservice-toolbox BootstrapService.
// 2. Traverses and validates workspace root, healing 35 standalone.yaml symlinks.
// 3. Builds and spawns supervised ecosystem processes according to dependency DAG.
// 4. Starts NATS control plane telemetry, Tele-Remote bot menus, and REST server on port 9095.
// 5. Waits for OS termination signals (SIGINT/SIGTERM) to gracefully tear down process trees.
//
// KEY PARAMETERS:
// - profile: Configuration profile to load (default: standalone).
// - build: Boolean flag to trigger compilation of out-of-date binaries on startup.
// =============================================================================

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	toolbox_bootstrap "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/bootstrap"
	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	toolbox_lifecycle "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	toolbox_spec "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/spec"
	toolbox_wiretap "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/wiretap"
	watchdog_config "github.com/Bastien-Antigravity/watchdog-agent/src/config"
	"github.com/Bastien-Antigravity/watchdog-agent/src/control"
	"github.com/Bastien-Antigravity/watchdog-agent/src/rest"
	"github.com/Bastien-Antigravity/watchdog-agent/src/server"
	"github.com/Bastien-Antigravity/watchdog-agent/src/shipper"
	"github.com/Bastien-Antigravity/watchdog-agent/src/supervisor"
	"github.com/Bastien-Antigravity/watchdog-agent/src/telegram"
	"github.com/Bastien-Antigravity/watchdog-agent/src/utils"
)

// -----------------------------------------------------------------------------

func main() {
	profileFlag := flag.String("profile", "standalone", "Configuration profile to load (e.g. standalone, production)")
	buildFlag := flag.Bool("build", true, "Automatically build binaries on startup")
	modeFlag := flag.String("mode", "", "Operational mode: 'all' (default), 'supervisor', 'shipper', 'telemetry'")
	superviseFlag := flag.Bool("supervise", true, "Enable process supervision and auto-restart (default false in Docker)")
	shipperFlag := flag.Bool("shipper", true, "Enable autonomous error log shipper")
	restFlag := flag.Bool("rest", true, "Enable HTTP REST API and OpenMFE dashboard")
	flag.Parse()

	// Resolve operational switches and environment overrides
	opts := watchdog_config.ResolveRuntimeOptions(
		*modeFlag,
		*superviseFlag,
		*shipperFlag,
		*restFlag,
		*buildFlag,
		*profileFlag,
	)

	// 1. Resolve workspace root dynamically (with fallback for standalone container execution)
	rootDir, err := utils.FindWorkspaceRoot()
	if err != nil {
		if !opts.Supervise || opts.IsDocker {
			rootDir, _ = os.Getwd()
		} else {
			supervisor.LogError("watchdog", "Failed to find workspace root: %v", err)
			os.Exit(1)
		}
	}

	// 2. Heal config symlinks before loading AppConfig (native supervisor mode only)
	if opts.Supervise && !opts.IsDocker {
		if err := watchdog_config.HealSymlinks(rootDir); err != nil {
			supervisor.LogError("watchdog", "Failed to heal ecosystem config symlinks: %v", err)
			os.Exit(1)
		}
	}

	// 3. Initialize Service via Unified Ecosystem Bootstrapper
	cfg, appLogger := toolbox_bootstrap.BootstrapService("watchdog-agent")
	if cfg == nil || appLogger == nil {
		_, _ = fmt.Fprintln(os.Stderr, "CRITICAL: Failed to bootstrap watchdog-agent: cfg and appLogger must not be nil")
		os.Exit(1)
	}
	defer appLogger.Close()
	supervisor.Logger = appLogger

	version := cfg.GetVersion()
	appLogger.Info("Starting Bastien-Antigravity Watchdog Agent v%s (Profile: %s, Mode: %s)...", version, opts.Profile, opts.Mode)
	appLogger.Info("Active Capabilities -> Supervise: %t, Shipper: %t, REST/Dashboard: %t", opts.Supervise, opts.Shipper, opts.REST)
	appLogger.Info("Resolved workspace root: %s", rootDir)

	cfg.OnLiveConfUpdate(func(_ map[string]interface{}) {
		toolbox_wiretap.ConfigureFromConfig("watchdog-agent", cfg)
		appLogger.Info("Watchdog Agent live configuration hot-reload applied (profile: %s)", cfg.Profile)
	})

	// Detect local host IPs to determine local service assignments
	localIPs, err := utils.GetLocalIPs()
	if err != nil {
		appLogger.Critical("Failed to detect local IPs: %v", err)
		os.Exit(1)
	}

	// Single instance locking
	lockDir := rootDir
	if opts.IsDocker || !opts.Supervise {
		lockDir = os.TempDir()
	}
	lockPath := filepath.Join(lockDir, ".watchdog-agent.lock")
	lockFile, err := utils.AcquireLock(lockPath)
	if err != nil {
		appLogger.Critical("Another instance of watchdog-agent is already running (failed to acquire lock %s). Exiting.", lockPath)
		os.Exit(1)
	}
	defer lockFile.Close()

	if opts.Supervise {
		defaultHost := os.Getenv("HOST_IP")
		if defaultHost == "" {
			defaultHost = cfg.Common.PublicIP
		}
		if defaultHost == "" {
			appLogger.Critical("Failed to resolve default host IP from AppConfig Common.PublicIP or HOST_IP")
			os.Exit(1)
		}

		// Initialize the topologies registry of services
		supervisor.RegisterServices(rootDir, defaultHost, cfg)

		// Validate registry topologies
		if err := supervisor.ValidateRegistry(); err != nil {
			appLogger.Critical("Ecosystem registry validation failed: %v", err)
			os.Exit(1)
		}

		// Base environment to inject to children (filter out watchdog-specific overrides)
		var filteredEnv []string
		for _, envVar := range os.Environ() {
			if !strings.HasPrefix(envVar, "LOGGER_PROFILE=") {
				filteredEnv = append(filteredEnv, envVar)
			}
		}
		baseEnv := filteredEnv

		// Compile local binaries if required (e.g. if --build=true)
		if opts.Build {
			for _, svc := range supervisor.Services {
				if svc.BuildCmd != "" {
					appLogger.Info("Building service '%s'...", svc.Name)
					cmd := exec.Command(svc.BuildCmd, svc.BuildArgs...)
					cmd.Dir = svc.Path
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					if err := cmd.Run(); err != nil {
						appLogger.Error("Build failed for '%s': %v", svc.Name, err)
						os.Exit(1)
					}
					appLogger.Info("Service '%s' compiled successfully.", svc.Name)
				}
			}
		}

		// Launch each service in its own monitor goroutine
		for _, svc := range supervisor.Services {
			// Only run if the service IP is resolved to this local node
			if utils.IsLocal(svc.IP, localIPs) {
				// Clone baseEnv slice to prevent concurrent slice modification races
				svcEnv := make([]string, len(baseEnv))
				copy(svcEnv, baseEnv)
				go supervisor.MonitorAndSupervise(svc, svcEnv, localIPs)
			} else {
				appLogger.Info("Skipping non-local service '%s' assigned to node %s", svc.Name, svc.IP)
			}
		}
	} else {
		appLogger.Info("Child process supervision disabled (process lifecycle managed by Docker or external orchestrator).")
	}

	// Initialize Lifecycle Manager
	lm := toolbox_lifecycle.NewManagerWithLogger(appLogger)

	// Initialize shared NATS bus for REST telemetry heartbeats and/or Shipper fleet incident publishing
	brainID := shipper.ResolveBrainID(cfg)
	var incidentNotifier shipper.IncidentPublisher
	if opts.REST || opts.Shipper {
		busClient, busErr := control.NewBusClient(cfg, appLogger)
		if busErr != nil {
			appLogger.Warning("NATS Control Plane unavailable (%v); continuing offline", busErr)
		} else {
			busCtx, busCancel := context.WithCancel(context.Background())
			go busClient.Start(busCtx, opts.REST)
			lm.Register("StopNATSControlPlane", func() error {
				busCancel()
				return nil
			})
			if opts.Shipper {
				incidentNotifier = shipper.NewNatsIncidentPublisher(busClient.Publish, busClient.SubjectPrefix(), brainID)
			}
		}
	}

	if opts.REST {
		// Resolve timescale_db address using capability configuration
		postgresAddr, _ := cfg.GetListenAddr("timescale_db")

		// Resolve RAG MCP address using capability configuration
		ragMcpAddr := ""
		var ragCap struct {
			MCP struct {
				IP   string `json:"ip"`
				Port string `json:"port"`
			} `json:"mcp"`
		}
		if err := cfg.GetCapability("rag_engine", &ragCap); err == nil {
			if ragCap.MCP.IP != "" && ragCap.MCP.Port != "" {
				ragMcpAddr = net.JoinHostPort(ragCap.MCP.IP, ragCap.MCP.Port)
			}
		}

		// Initialize Watchdog Controller
		ctrl := server.NewController(postgresAddr, ragMcpAddr)

		// Resolve REST address verbatim from AppConfig (zero 0.0.0.0 IP rewriting)
		restAddr, err := cfg.GetRESTAddr("watchdog_agent")
		if err == nil && restAddr != "" {
			_, portStr, splitErr := net.SplitHostPort(restAddr)
			if splitErr == nil {
				var restPort int
				if _, err := fmt.Sscanf(portStr, "%d", &restPort); err == nil {
					restHandler := rest.NewRESTHandler(ctrl, appLogger)
					restHandler.SetAppConfig(cfg)
					restServer := &http.Server{
						Addr:         restAddr,
						Handler:      restHandler.Handler(),
						ReadTimeout:  5 * time.Second,
						WriteTimeout: 10 * time.Second,
					}
					go func() {
						appLogger.Info("Starting REST management server on %s", restAddr)
						if sErr := restServer.ListenAndServe(); sErr != nil && sErr != http.ErrServerClosed {
							appLogger.Error("REST server failed to start: %v", sErr)
						}
					}()
					lm.Register("WatchdogRESTServer", func() error {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						return restServer.Shutdown(ctx)
					})

					mfeUrl := buildMFEURL(cfg, restAddr)
					_ = toolbox_spec.PublishWithLifecycle(cfg, appLogger, lm, "watchdog_agent", toolbox_spec.ServiceSpec{
						Name:        "Watchdog Agent",
						Tag:         "watchdog-agent-mfe",
						URL:         mfeUrl,
						NavTitle:    "Watchdog Agent",
						Layer:       "L1",
						Category:    "Base Fleet",
						Priority:    10,
						Icon:        "fa-shield",
						Description: "Local node supervisor, process orchestrator, health monitor, and configuration symlink healer.",
						Protocol:    "HTTP/1.1 REST",
						Framing:     fmt.Sprintf("Port %d REST", restPort),
						DocLink:     "obsidian-brain/06-Microservices/Watchdog-Agent-Hub.md",
						Contracts:   rest.BuildExchangeContracts(cfg, nil),
					})
				}
			}
		}

		// Setup Telegram client
		telegram.SetupTelegram(cfg, ctrl, appLogger, lm)
	}

	// Initialize and start Autonomous Fleet Error Log Shipper
	if opts.Shipper {
		dbCfg, dbErr := shipper.ResolveDBConfigWithAppConfig(cfg)
		if dbErr != nil {
			appLogger.Critical("Failed to validate shipper database config at startup: %v", dbErr)
			os.Exit(1)
		}

		checkpointPath := os.Getenv("SHIPPER_CHECKPOINT_PATH")
		if checkpointPath == "" {
			if opts.IsDocker {
				checkpointPath = "/watchdog-agent/state/.shipper_offsets.json"
			} else {
				checkpointPath = filepath.Join(rootDir, ".shipper_offsets.json")
			}
		}

		shipperCfg := shipper.Config{
			RootDir:        rootDir,
			ScanDir:        os.Getenv("LOG_SCAN_DIRS"),
			CheckpointPath: checkpointPath,
			PollInterval:   1 * time.Second,
			MaxBatchSize:   500,
			BrainID:        brainID,
			DBConfig:       dbCfg,
			Notifier:       incidentNotifier,
			Logger:         appLogger,
		}
		if logShipper, err := shipper.NewLogShipper(shipperCfg); err != nil {
			appLogger.Critical("Failed to initialize LogShipper: %v", err)
			os.Exit(1)
		} else {
			appLogger.Info("Autonomous Fleet Error Log Shipper initialized (Brain: '%s', DB: '%s:%d/%s', ScanDir: '%s', Checkpoint: '%s')",
				brainID, dbCfg.Host, dbCfg.Port, dbCfg.DBName, logShipper.ScanDir(), checkpointPath)
			shipperCtx, shipperCancel := context.WithCancel(context.Background())
			logShipper.Start(shipperCtx)
			lm.Register("StopLogShipper", func() error {
				shipperCancel()
				return nil
			})
		}
	} else {
		appLogger.Info("Autonomous Fleet Error Log Shipper disabled.")
	}

	// Register cleanup hooks
	if opts.Supervise {
		lm.Register("StopServices", func() error {
			appLogger.Info("Shutdown signal received. Terminating all managed processes...")
			supervisor.KillAll()
			return nil
		})
	}

	lm.Wait(context.Background())
}

// -----------------------------------------------------------------------------

func buildMFEURL(cfg *toolbox_config.AppConfig, restAddr string) string {
	return toolbox_spec.ResolveMFEURL(cfg, fmt.Sprintf("http://%s/static/mfe.js", restAddr))
}
