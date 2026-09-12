package utils

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
		if _, err := os.Stat(filepath.Join(dir, "docker-deployment", "shared-config")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("shared-config directory not found in parent hierarchy")
}

// GetLocalIPs returns a set of local IP addresses for this machine
func GetLocalIPs() (map[string]bool, error) {
	ips := make(map[string]bool)
	ips["127.0.0.1"] = true
	ips["127.0.0.2"] = true
	ips["localhost"] = true

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok {
			ips[ipNet.IP.String()] = true
		}
	}
	return ips, nil
}

// IsLocal checks if an IP belongs to the local machine
func IsLocal(ip string, localIPs map[string]bool) bool {
	if ip == "" || ip == "0.0.0.0" {
		return true
	}
	return localIPs[ip]
}

// IsLocalIP is an alias for IsLocal
func IsLocalIP(ip string, localIPs map[string]bool) bool {
	return IsLocal(ip, localIPs)
}

// FindPythonCmd locates python3 or python command in virtual environment or system fallback
func FindPythonCmd(dir string) string {
	// 1. Unix standard venv
	venvPy := filepath.Join(dir, ".venv", "bin", "python3")
	if _, err := os.Stat(venvPy); err == nil {
		return venvPy
	}
	venvPyLocal := filepath.Join(dir, ".venv", "bin", "python")
	if _, err := os.Stat(venvPyLocal); err == nil {
		return venvPyLocal
	}
	// 2. Windows standard venv
	venvPyWin := filepath.Join(dir, ".venv", "Scripts", "python.exe")
	if _, err := os.Stat(venvPyWin); err == nil {
		return venvPyWin
	}
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("python.exe"); err == nil {
			return p
		}
		if p, err := exec.LookPath("python"); err == nil {
			return p
		}
		return "python"
	}
	if p, err := exec.LookPath("python3"); err == nil {
		return p
	}
	return "python3"
}

// KillProcessOnPort forcefully terminates any process listening on a port
func KillProcessOnPort(port string) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", fmt.Sprintf("for /f \"tokens=5\" %%a in ('netstat -aon ^| findstr :%s ^| findstr LISTENING') do taskkill /F /PID %%a", port))
		_ = cmd.Run()
		return
	}
	cmd := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti :%s | xargs kill -9", port))
	_ = cmd.Run()
}

// IsOccupantFleetService checks if the process listening on a port is a base/fleet service.
func IsOccupantFleetService(port string) (bool, error) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", fmt.Sprintf("netstat -aon | findstr :%s | findstr LISTENING", port))
		output, err := cmd.Output()
		if err != nil || len(strings.TrimSpace(string(output))) == 0 {
			return false, nil
		}
		return true, nil
	}

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
			"docker",
			"docker-proxy",
			"com.docker",
			"postgres",
			"timescaledb",
		}

		for _, kw := range fleetKeywords {
			if strings.Contains(commandLine, kw) {
				return true, nil
			}
		}
	}

	return false, nil
}
