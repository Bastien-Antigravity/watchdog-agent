package utils

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FindWorkspaceRoot locates the workspace root containing shared-config
func FindWorkspaceRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "shared-config")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	exe, err := os.Executable()
	if err == nil {
		dir = filepath.Dir(exe)
		for {
			if _, err := os.Stat(filepath.Join(dir, "shared-config")); err == nil {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	return "", fmt.Errorf("could not find workspace root (shared-config directory not found in parent directories)")
}

// GetLocalIPs returns local IPv4 and IPv6 loopback and interface addresses
func GetLocalIPs() (map[string]bool, error) {
	ips := map[string]bool{
		"127.0.0.1": true,
		"localhost": true,
		"::1":       true,
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			ips[ipnet.IP.String()] = true
		}
	}
	return ips, nil
}

// IsLocal checks if an IP is local to the machine
func IsLocal(ip string, localIPs map[string]bool) bool {
	if ip == "0.0.0.0" || ip == "" {
		return true
	}
	return localIPs[ip]
}

// FindPythonCmd locates python3 or python command in virtual environment or system fallback
func FindPythonCmd(dir string) string {
	venvPy := filepath.Join(dir, ".venv", "bin", "python3")
	if _, err := os.Stat(venvPy); err == nil {
		return venvPy
	}
	venvPyLocal := filepath.Join(dir, ".venv", "bin", "python")
	if _, err := os.Stat(venvPyLocal); err == nil {
		return venvPyLocal
	}
	return "python3"
}

// KillProcessOnPort forcefully terminates any process listening on a port
func KillProcessOnPort(port string) {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti :%s | xargs kill -9", port))
	_ = cmd.Run()
}

// IsOccupantFleetService checks if the process listening on a port is a base/fleet service.
func IsOccupantFleetService(port string) (bool, error) {
	// 1. Run lsof to get PIDs
	cmd := exec.Command("lsof", "-t", "-i", fmt.Sprintf(":%s", port), "-sTCP:LISTEN")
	output, err := cmd.Output()
	if err != nil {
		// If lsof fails or returns empty, assume not listening or not a fleet service
		return false, nil
	}

	pidsStr := strings.TrimSpace(string(output))
	if pidsStr == "" {
		return false, nil
	}

	// Split by newline to get all PIDs
	pids := strings.Split(pidsStr, "\n")
	for _, pid := range pids {
		pid = strings.TrimSpace(pid)
		if pid == "" {
			continue
		}

		// 2. Get process command line using ps
		psCmd := exec.Command("ps", "-p", pid, "-o", "command=")
		psOutput, err := psCmd.Output()
		if err != nil {
			continue
		}

		commandLine := strings.ToLower(string(psOutput))

		// Check against base/fleet services keywords
		fleetKeywords := []string{
			"watchdog-agent",
			"web-interface",
			"notif-server",
			"log-server",
			"config-server",
			"tele-remote",
			"main.py",
			"nats-server",
		}

		for _, kw := range fleetKeywords {
			if strings.Contains(commandLine, kw) {
				return true, nil
			}
		}
	}

	return false, nil
}
