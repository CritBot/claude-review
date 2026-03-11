package memory

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const daemonPIDFile = "daemon.pid"

// DaemonPaths holds paths used by the daemon.
type DaemonPaths struct {
	HomeDir string // ~/.claude-review/
	PIDFile string
	LogFile string
}

// DefaultDaemonPaths returns the default paths for daemon files.
func DefaultDaemonPaths() (DaemonPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return DaemonPaths{}, err
	}
	dir := filepath.Join(home, ".claude-review")
	return DaemonPaths{
		HomeDir: dir,
		PIDFile: filepath.Join(dir, daemonPIDFile),
		LogFile: filepath.Join(dir, "daemon.log"),
	}, nil
}

// DaemonStatus holds the current status of the background daemon.
type DaemonStatus struct {
	Running           bool
	PID               int
	Uptime            time.Duration
	LastConsolidation time.Time
	TotalFindings     int
}

// GetDaemonStatus returns the current daemon status.
func GetDaemonStatus(paths DaemonPaths) DaemonStatus {
	data, err := os.ReadFile(paths.PIDFile)
	if err != nil {
		return DaemonStatus{Running: false}
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return DaemonStatus{Running: false}
	}

	// Check if process is actually running
	process, err := os.FindProcess(pid)
	if err != nil {
		return DaemonStatus{Running: false}
	}
	if err := process.Signal(os.Signal(nil)); err != nil {
		// Process not running
		os.Remove(paths.PIDFile)
		return DaemonStatus{Running: false}
	}

	return DaemonStatus{Running: true, PID: pid}
}

// StartDaemon launches the consolidation daemon as a background process.
// The daemon watches the memory.db and runs consolidation when triggered.
func StartDaemon(paths DaemonPaths) error {
	status := GetDaemonStatus(paths)
	if status.Running {
		return fmt.Errorf("daemon already running (PID %d)", status.PID)
	}

	if err := os.MkdirAll(paths.HomeDir, 0755); err != nil {
		return err
	}

	// Self-invoke with the hidden --daemon flag
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	logFile, err := os.OpenFile(paths.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "memory", "--daemon-loop")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}

	pid := strconv.Itoa(cmd.Process.Pid)
	if err := os.WriteFile(paths.PIDFile, []byte(pid), 0644); err != nil {
		return err
	}

	fmt.Printf("✓ claude-review memory daemon started (PID %s)\n", pid)
	return nil
}

// StopDaemon terminates the background daemon.
func StopDaemon(paths DaemonPaths) error {
	status := GetDaemonStatus(paths)
	if !status.Running {
		return fmt.Errorf("daemon is not running")
	}

	process, err := os.FindProcess(status.PID)
	if err != nil {
		return err
	}
	if err := process.Kill(); err != nil {
		return fmt.Errorf("stopping daemon: %w", err)
	}
	os.Remove(paths.PIDFile)
	fmt.Println("✓ claude-review memory daemon stopped")
	return nil
}

// InstallAutostart writes a launchd (macOS) or systemd (Linux) unit to auto-start the daemon on login.
func InstallAutostart(paths DaemonPaths) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(exe, paths)
	case "linux":
		return installSystemd(exe, paths)
	default:
		return fmt.Errorf("autostart not supported on %s — run 'claude-review memory start' manually", runtime.GOOS)
	}
}

func installLaunchd(exe string, paths DaemonPaths) error {
	plistPath := filepath.Join(paths.HomeDir, "com.critbot.claude-review.plist")
	launchAgentsDir := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return err
	}
	symlinkPath := filepath.Join(launchAgentsDir, "com.critbot.claude-review.plist")

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.critbot.claude-review</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>memory</string>
    <string>--daemon-loop</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>`, exe, paths.LogFile, paths.LogFile)

	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return err
	}
	os.Remove(symlinkPath)
	if err := os.Symlink(plistPath, symlinkPath); err != nil {
		return err
	}
	exec.Command("launchctl", "load", symlinkPath).Run()
	fmt.Printf("✓ launchd agent installed at %s\n", symlinkPath)
	return nil
}

func installSystemd(exe string, paths DaemonPaths) error {
	unitDir := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		return err
	}
	unitPath := filepath.Join(unitDir, "claude-review-memory.service")

	unit := fmt.Sprintf(`[Unit]
Description=claude-review memory consolidation daemon
After=network.target

[Service]
ExecStart=%s memory --daemon-loop
Restart=on-failure
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=default.target
`, exe, paths.LogFile, paths.LogFile)

	if err := os.WriteFile(unitPath, []byte(unit), 0644); err != nil {
		return err
	}
	exec.Command("systemctl", "--user", "enable", "--now", "claude-review-memory").Run()
	fmt.Printf("✓ systemd user service installed at %s\n", unitPath)
	return nil
}
