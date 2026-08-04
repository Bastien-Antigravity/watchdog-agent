package supervisor

import (
	"os/exec"
	"runtime"
	"time"
)

// LaunchPostgresAttempt tries to start the PostgreSQL/TimescaleDB database
// if it is not already running. It returns true if successful or if it was already running.
func LaunchPostgresAttempt(addr string) bool {
	// 1. Check if already running
	if IsPortListening(addr, 500*time.Millisecond) {
		return true
	}

	LogInfo("postgres-launcher", "Postgres database at %s is offline. Attempting to start it...", addr)

	// 2. Try Docker container first (container name "timescale-db")
	if _, err := exec.LookPath("docker"); err == nil {
		LogInfo("postgres-launcher", "Docker detected. Running: docker start timescale-db")
		cmd := exec.Command("docker", "start", "timescale-db")
		if err := cmd.Run(); err == nil {
			// Wait and verify
			for i := 0; i < 10; i++ {
				if IsPortListening(addr, 500*time.Millisecond) {
					LogInfo("postgres-launcher", "Postgres started successfully via Docker.")
					return true
				}
				time.Sleep(1 * time.Second)
			}
		} else {
			LogError("postgres-launcher", "Docker start failed: %v", err)
		}
	}

	// 3. Fallback to OS-specific service management
	switch runtime.GOOS {
	case "darwin": // macOS
		if _, err := exec.LookPath("brew"); err == nil {
			LogInfo("postgres-launcher", "macOS: Attempting brew services start")
			// Try starting typical names: postgresql, postgresql@15, postgresql@16
			for _, service := range []string{"postgresql", "postgresql@15", "postgresql@16", "timescaledb"} {
				exec.Command("brew", "services", "start", service).Run()
				if IsPortListening(addr, 1*time.Second) {
					LogInfo("postgres-launcher", "Postgres started successfully via Homebrew (%s).", service)
					return true
				}
			}
		}
	case "linux": // Linux
		if _, err := exec.LookPath("systemctl"); err == nil {
			LogInfo("postgres-launcher", "Linux: Attempting systemctl start postgresql")
			exec.Command("sudo", "systemctl", "start", "postgresql").Run()
		} else if _, err := exec.LookPath("service"); err == nil {
			LogInfo("postgres-launcher", "Linux: Attempting service postgresql start")
			exec.Command("sudo", "service", "postgresql", "start").Run()
		}
	case "windows": // Windows
		LogInfo("postgres-launcher", "Windows: Attempting net start postgresql")
		// Try a few typical service names
		for _, svcName := range []string{"postgresql", "postgresql-x64-15", "postgresql-x64-16"} {
			exec.Command("net", "start", svcName).Run()
			if IsPortListening(addr, 1*time.Second) {
				LogInfo("postgres-launcher", "Postgres started successfully via Windows Service (%s).", svcName)
				return true
			}
		}
	}

	// Wait and verify one final time
	for i := 0; i < 5; i++ {
		if IsPortListening(addr, 500*time.Millisecond) {
			LogInfo("postgres-launcher", "Postgres is now online.")
			return true
		}
		time.Sleep(1 * time.Second)
	}

	LogError("postgres-launcher", "Failed to start Postgres database automatically. Please start it manually.")
	return false
}
