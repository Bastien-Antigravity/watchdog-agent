package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	"github.com/Bastien-Antigravity/watchdog-agent/src/utils"
)

// RegisterServices initializes the topology registry slice of managed services
func RegisterServices(rootDir, vectorDBIP, vectorDBPort string, cfg *config.AppConfig) {
	exeExt := ""
	if runtime.GOOS == "windows" {
		exeExt = ".exe"
	}

	// Resolve local NATS server configuration parameters dynamically
	natsConfigPath := cfg.Config.Get("nats_server", "config_path")
	if natsConfigPath == "" {
		natsConfigPath = filepath.Join(rootDir, "watchdog-agent", "nats", "nats-server.conf")
	} else if !filepath.IsAbs(natsConfigPath) {
		// Resolve relative path to absolute
		natsConfigPath = filepath.Join(rootDir, natsConfigPath)
	}

	// Resolve NATS server executable:
	// 1. Look in system PATH (e.g. brew, apt, winget)
	// 2. Fall back to bundled watchdog-agent/nats/nats-server strictly on macOS
	natsRunCmd := ""
	if p, err := exec.LookPath("nats-server" + exeExt); err == nil {
		natsRunCmd = p
	} else if p, err := exec.LookPath("nats-server"); err == nil {
		natsRunCmd = p
	} else if runtime.GOOS == "darwin" {
		bundled := filepath.Join(rootDir, "watchdog-agent", "nats", "nats-server")
		if _, err := os.Stat(bundled); err == nil {
			natsRunCmd = bundled
		}
	}

	Services = []*Service{
		{
			Name:     "nats-server",
			CapName:  "nats_server",
			Path:     filepath.Join(rootDir, "watchdog-agent", "nats"),
			BuildCmd: "",
			RunCmd:   natsRunCmd,
			RunArgs:  []string{"-c", natsConfigPath},
			Deps:     []string{},
			LogColor: ColorWatchdog,
		},
		{
			Name:      "log-server",
			CapName:   "log_server",
			Path:      filepath.Join(rootDir, "log-server"),
			BuildCmd:  "cargo",
			BuildArgs: []string{"build"},
			RunCmd:    filepath.Join(rootDir, "log-server", "target", "debug", "log-server"+exeExt),
			RunArgs:   []string{"--name", "log_server", "--port", "9020"},
			Deps:      []string{},
			LogColor:  ColorLogServer,
		},
		{
			Name:      "config-server",
			CapName:   "config_server",
			Path:      filepath.Join(rootDir, "config-server"),
			BuildCmd:  "go",
			BuildArgs: []string{"build", "-o", "bin/config-server" + exeExt, "./cmd/config-server"},
			RunCmd:    filepath.Join(rootDir, "config-server", "bin", "config-server"+exeExt),
			RunArgs:   []string{},
			Deps:      []string{"log-server"},
			LogColor:  ColorConfigServer,
		},
		{
			Name:      "notif-server",
			CapName:   "notif_server",
			Path:      filepath.Join(rootDir, "notif-server"),
			BuildCmd:  "go",
			BuildArgs: []string{"build", "-o", "bin/notif-server" + exeExt, "./cmd/notif-server"},
			RunCmd:    filepath.Join(rootDir, "notif-server", "bin", "notif-server"+exeExt),
			RunArgs:   []string{},
			Deps:      []string{"log-server", "config-server"},
			LogColor:  ColorNotifServer,
		},
		{
			Name:      "tele-remote",
			CapName:   "tele_remote",
			Path:      filepath.Join(rootDir, "tele-remote"),
			BuildCmd:  "go",
			BuildArgs: []string{"build", "-o", "bin/tele-remote" + exeExt, "./cmd/tele-remote"},
			RunCmd:    filepath.Join(rootDir, "tele-remote", "bin", "tele-remote"+exeExt),
			RunArgs:   []string{},
			Deps:      []string{"log-server", "config-server", "notif-server"},
			LogColor:  ColorTeleRemote,
		},
		{
			Name:      "web-interface",
			CapName:   "web_interface",
			Path:      filepath.Join(rootDir, "web-interface"),
			BuildCmd:  "go",
			BuildArgs: []string{"build", "-o", "bin/web-interface" + exeExt, "./cmd/web-interface"},
			RunCmd:    filepath.Join(rootDir, "web-interface", "bin", "web-interface"+exeExt),
			RunArgs:   []string{},
			Deps:      []string{"log-server", "config-server"},
			LogColor:  ColorWebInterface,
		},
		{
			Name:     "rag-engine",
			CapName:  "rag_engine",
			Path:     filepath.Join(rootDir, "obsidian-brain", "09-RAG-Engine"),
			BuildCmd: "",
			RunCmd:   utils.FindPythonCmd(filepath.Join(rootDir, "obsidian-brain", "09-RAG-Engine")),
			RunArgs:  []string{"main.py", "server"},
			Deps:     []string{"log-server", "config-server"},
			LogColor: ColorRagEngine,
		},
		{
			Name:     "rag-dashboard",
			CapName:  "rag_engine",
			Path:     filepath.Join(rootDir, "obsidian-brain", "09-RAG-Engine"),
			BuildCmd: "",
			RunCmd:   utils.FindPythonCmd(filepath.Join(rootDir, "obsidian-brain", "09-RAG-Engine")),
			RunArgs:  []string{"main.py", "dashboard"},
			Deps:     []string{"log-server", "config-server", "rag-engine"},
			LogColor: ColorRagDashboard,
		},
		{
			Name:     "start-squad",
			CapName:  "base_scripts",
			Path:     filepath.Join(rootDir, "obsidian-brain", "08-Base-Scripts"),
			BuildCmd: "",
			RunCmd:   utils.FindPythonCmd(filepath.Join(rootDir, "obsidian-brain", "08-Base-Scripts")),
			RunArgs:  []string{"main.py", "start-squad"},
			Deps:     []string{"log-server", "config-server", "nats-server"},
			LogColor: ColorStartSquad,
		},
	}

	// Filter and initialize addresses for services
	for _, svc := range Services {
		var defaultPort string
		switch svc.Name {
		case "log-server":
			defaultPort = "9020"
		case "config-server":
			defaultPort = "3306"
		case "notif-server":
			defaultPort = "1026"
		case "tele-remote":
			defaultPort = "1863"
		case "web-interface":
			defaultPort = "8000"
		case "rag-engine":
			defaultPort = "8090"
		case "rag-dashboard":
			defaultPort = "8082"
		case "nats-server":
			defaultPort = "4222"
		case "start-squad":
			defaultPort = "8085"
		}

		svc.IP, svc.Port = resolveServiceAddr(cfg, svc.CapName, vectorDBIP, defaultPort)
		if svc.Name == "rag-dashboard" {
			if capMap, ok := cfg.Capabilities["rag_engine"].(map[string]interface{}); ok {
				if dash, ok := capMap["dashboard"].(map[string]interface{}); ok {
					if port, exists := dash["port"]; exists {
						svc.Port = fmt.Sprintf("%v", port)
					}
				}
			}
		}
	}
}

func resolveServiceAddr(cfg *config.AppConfig, capName string, defaultIP, defaultPort string) (string, string) {
	// Special Case for RAG Engine MCP address resolution
	if capName == "rag_engine" && defaultPort == "8090" {
		if capMap, ok := cfg.Capabilities["rag_engine"].(map[string]interface{}); ok {
			if mcp, ok := capMap["mcp"].(map[string]interface{}); ok {
				var resIP, resPort string
				if ip, exists := mcp["ip"]; exists {
					resIP = fmt.Sprintf("%v", ip)
				}
				if port, exists := mcp["port"]; exists {
					resPort = fmt.Sprintf("%v", port)
				}
				if resIP != "" && resPort != "" {
					return resIP, resPort
				}
			}
		}
	}

	addr, err := cfg.GetListenAddr(capName)
	if err == nil && addr != "" {
		if strings.Contains(addr, ":") {
			sp := strings.Split(addr, ":")
			if len(sp) == 2 {
				return sp[0], sp[1]
			}
		}
	}
	return defaultIP, defaultPort
}

// ValidateRegistry checks for missing dependencies and dependency cycles
func ValidateRegistry() error {
	// 1. Check for missing dependencies
	for _, svc := range Services {
		for _, depName := range svc.Deps {
			if FindServiceByName(depName) == nil {
				return fmt.Errorf("service '%s' depends on unknown service '%s'", svc.Name, depName)
			}
		}
	}

	// 2. Check for dependency cycles using DFS (color-based)
	visited := make(map[string]int) // 0: unvisited, 1: visiting, 2: visited
	var dfs func(name string) error
	dfs = func(name string) error {
		visited[name] = 1 // visiting
		svc := FindServiceByName(name)
		if svc != nil {
			for _, depName := range svc.Deps {
				state := visited[depName]
				if state == 1 {
					return fmt.Errorf("dependency cycle detected involving service '%s'", depName)
				} else if state == 0 {
					if err := dfs(depName); err != nil {
						return err
					}
				}
			}
		}
		visited[name] = 2 // fully visited
		return nil
	}

	for _, svc := range Services {
		if visited[svc.Name] == 0 {
			if err := dfs(svc.Name); err != nil {
				return err
			}
		}
	}
	return nil
}
