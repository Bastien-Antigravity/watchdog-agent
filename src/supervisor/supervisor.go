package supervisor

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/Bastien-Antigravity/watchdog-agent/src/utils"
)

var (
	activeCmds   []*exec.Cmd
	activeCmdsMu sync.Mutex
	Services     []*Service
)

// LogInfo writes a formatted message with watchdog prefix to stdout/logger
func LogInfo(svc, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if Logger != nil {
		Logger.Info("[%s] %s", svc, msg)
	}
	fmt.Printf("%s[watchdog:%s]%s %s\n", ColorWatchdog, svc, ColorReset, msg)
}

// LogError writes a formatted error message with watchdog prefix to stderr/logger
func LogError(svc, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if Logger != nil {
		Logger.Error("[%s:ERROR] %s", svc, msg)
	}
	fmt.Fprintf(os.Stderr, "%s[watchdog:%s:ERROR]%s %s\n", ColorWatchdog, svc, ColorReset, msg)
}

// RegisterCmd adds a command to the active list for signal cleanup
func RegisterCmd(cmd *exec.Cmd) {
	activeCmdsMu.Lock()
	defer activeCmdsMu.Unlock()
	activeCmds = append(activeCmds, cmd)
}

// UnregisterCmd removes a command from the active list
func UnregisterCmd(cmd *exec.Cmd) {
	activeCmdsMu.Lock()
	defer activeCmdsMu.Unlock()
	for i, c := range activeCmds {
		if c == cmd {
			activeCmds = append(activeCmds[:i], activeCmds[i+1:]...)
			break
		}
	}
}

// KillAll kills all supervised command processes clean
func KillAll() {
	activeCmdsMu.Lock()
	cmds := make([]*exec.Cmd, len(activeCmds))
	copy(cmds, activeCmds)
	activeCmdsMu.Unlock()

	for _, cmd := range cmds {
		if cmd != nil && cmd.Process != nil {
			utils.KillProcessGroup(cmd)
		}
	}
}

// FindServiceByName searches the topology registry for a service by name
func FindServiceByName(name string) *Service {
	for _, s := range Services {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// PipeOutput routes logs from subprocess stdout/stderr with service prefixes
func PipeOutput(name, colorCode string, reader io.Reader) {
	r := bufio.NewReader(reader)
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			// strip trailing newline if present
			if line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			}
			fmt.Printf("%s[%s]%s %s\n", colorCode, name, ColorReset, line)
		}
		if err != nil {
			break
		}
	}
}

// IsPortListening checks if TCP port listens
func IsPortListening(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// MonitorAndSupervise is the supervisor runner loop
func MonitorAndSupervise(svc *Service, baseEnv []string, localIPs map[string]bool) {
	for {
		// 1. Wait for dependencies to be healthy
		for _, depName := range svc.Deps {
			depSvc := FindServiceByName(depName)
			if depSvc == nil {
				continue
			}

			depAddr := net.JoinHostPort(depSvc.IP, depSvc.Port)
			LogInfo(svc.Name, "Waiting for dependency '%s' on %s...", depName, depAddr)
			lastLogTime := time.Now()
			for {
				if IsPortListening(depAddr, 500*time.Millisecond) {
					break
				}
				if time.Since(lastLogTime) >= 10*time.Second {
					LogInfo(svc.Name, "Still waiting for dependency '%s' on %s...", depName, depAddr)
					lastLogTime = time.Now()
				}
				time.Sleep(1 * time.Second)
			}
			LogInfo(svc.Name, "Dependency '%s' is ready.", depName)
		}

		// 2. Clear port just in case it is occupied by an orphaned instance
		if svc.Port != "" {
			checkHost := svc.IP
			if checkHost == "" {
				checkHost = "127.0.0.1"
			}
			addr := net.JoinHostPort(checkHost, svc.Port)
			for {
				if !IsPortListening(addr, 100*time.Millisecond) {
					break
				}
				isFleet, err := utils.IsOccupantFleetService(svc.Port)
				if err != nil {
					LogError(svc.Name, "Failed to inspect port occupant: %v", err)
				}
				if isFleet {
					LogError(svc.Name, "Port %s is occupied by a fleet service! Attempting to clean up orphaned process...", svc.Port)
					utils.KillProcessOnPort(svc.Port)
					time.Sleep(500 * time.Millisecond)
				} else {
					LogError(svc.Name, "Port %s is occupied by a non-fleet service! Please free the port to proceed. Retrying in 5 seconds...", svc.Port)
					time.Sleep(5 * time.Second)
				}
			}
		}

		// 3. Start the subprocess
		LogInfo(svc.Name, "Launching process...")
		cmd := exec.Command(svc.RunCmd, svc.RunArgs...)
		cmd.Dir = svc.Path
		cmd.Env = baseEnv

		// Set process group attributes cross-platform to allow clean child process tree termination
		utils.SetSysProcAttrGroup(cmd)

		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			LogError(svc.Name, "Failed to create stdout pipe: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			LogError(svc.Name, "Failed to create stderr pipe: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if err := cmd.Start(); err != nil {
			LogError(svc.Name, "Failed to start process: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		RegisterCmd(cmd)
		svc.Mu.Lock()
		svc.Cmd = cmd
		svc.Running = true
		svc.Mu.Unlock()

		LogInfo(svc.Name, "Process started with PID %d", cmd.Process.Pid)

		// Pipe outputs in goroutines
		go PipeOutput(svc.Name, svc.LogColor, stdoutPipe)
		go PipeOutput(svc.Name, svc.LogColor, stderrPipe)

		// Block until process exits
		waitDone := make(chan error, 1)
		go func() {
			waitDone <- cmd.Wait()
		}()

		err = <-waitDone
		UnregisterCmd(cmd)

		svc.Mu.Lock()
		svc.Running = false
		svc.Cmd = nil

		svc.Mu.Unlock()

		LogError(svc.Name, "Process exited: %v. Restarting in 3 seconds...", err)
		time.Sleep(3 * time.Second)
	}
}
