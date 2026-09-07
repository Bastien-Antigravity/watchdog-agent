package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Bastien-Antigravity/watchdog-agent/src/supervisor"
)

type SymlinkTarget struct {
	Path   string
	Target string
}

// HealSymlinks heals ecosystem configuration symlinks pointing to the central standalone.yaml
func HealSymlinks(rootDir string) error {
	supervisor.LogInfo("watchdog", "Healing/verifying ecosystem configuration symlinks...")

	baseConfigPath := "docker-deployment/shared-config/native.yaml"
	if _, err := os.Stat(filepath.Join(rootDir, baseConfigPath)); os.IsNotExist(err) {
		baseConfigPath = "shared-config/native.yaml"
	}

	targets := []SymlinkTarget{
		{"standalone.yaml", baseConfigPath},
		{"config-server/standalone.yaml", "../" + baseConfigPath},
		{"config-server/cmd/config-server/standalone.yaml", "../../../" + baseConfigPath},
		{"log-server/standalone.yaml", "../" + baseConfigPath},
		{"notif-server/standalone.yaml", "../" + baseConfigPath},
		{"notif-server/cmd/notif-server/standalone.yaml", "../../../" + baseConfigPath},
		{"tele-remote/standalone.yaml", "../" + baseConfigPath},
		{"tele-remote/cmd/tele-remote/standalone.yaml", "../../../" + baseConfigPath},
		{"web-interface/standalone.yaml", "../" + baseConfigPath},
		{"web-interface/cmd/web-interface/standalone.yaml", "../../../" + baseConfigPath},
		{"watchdog-agent/standalone.yaml", "../" + baseConfigPath},
		{"obsidian-brain/08-Base-Scripts/standalone.yaml", "../../" + baseConfigPath},
		{"obsidian-brain/09-RAG-Engine/standalone.yaml", "../../" + baseConfigPath},
	}

	for _, t := range targets {
		absPath := filepath.Join(rootDir, t.Path)

		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", absPath, err)
		}

		lInfo, err := os.Lstat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				supervisor.LogInfo("watchdog", "Creating config symlink: %s -> %s", t.Path, t.Target)
				if err := os.Symlink(t.Target, absPath); err != nil {
					return fmt.Errorf("failed to create symlink %s: %w", absPath, err)
				}
				continue
			}
			return fmt.Errorf("failed to stat path %s: %w", absPath, err)
		}

		if lInfo.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(absPath)
			if err != nil {
				return fmt.Errorf("failed to read link %s: %w", absPath, err)
			}
			if linkTarget == t.Target {
				continue
			}
			supervisor.LogInfo("watchdog", "Correcting config symlink target for %s (pointing to %s, should be %s)", t.Path, linkTarget, t.Target)
			if err := os.Remove(absPath); err != nil {
				return fmt.Errorf("failed to remove incorrect symlink %s: %w", absPath, err)
			}
			if err := os.Symlink(t.Target, absPath); err != nil {
				return fmt.Errorf("failed to recreate symlink %s: %w", absPath, err)
			}
		} else {
			supervisor.LogInfo("watchdog", "Replacing regular config file at %s with symlink to %s", t.Path, t.Target)
			if err := os.RemoveAll(absPath); err != nil {
				return fmt.Errorf("failed to remove regular file %s: %w", absPath, err)
			}
			if err := os.Symlink(t.Target, absPath); err != nil {
				return fmt.Errorf("failed to create symlink %s: %w", absPath, err)
			}
		}
	}
	supervisor.LogInfo("watchdog", "Ecosystem configuration symlinks healed successfully.")
	return nil
}
