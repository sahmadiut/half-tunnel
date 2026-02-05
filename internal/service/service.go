// Package service provides systemd service management for Half-Tunnel.
package service

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// ServiceType represents the type of service (client or server).
type ServiceType string

const (
	// ClientService represents the half-tunnel client service.
	ClientService ServiceType = "client"
	// ServerService represents the half-tunnel server service.
	ServerService ServiceType = "server"
)

// ServiceConfig holds configuration for service installation.
type ServiceConfig struct {
	Type       ServiceType
	BinaryPath string
	ConfigPath string
	User       string
	WorkingDir string
}

const serviceTemplate = `[Unit]
Description=Half-Tunnel {{.TypeTitle}}
Documentation=https://github.com/sahmadiut/half-tunnel
After=network.target
Wants=network-online.target

[Service]
Type=simple
ExecStart={{.BinaryPath}} -config {{.ConfigPath}}
Restart=always
RestartSec=5
User={{.User}}
WorkingDirectory={{.WorkingDir}}
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal
SyslogIdentifier=half-tunnel-{{.Type}}

[Install]
WantedBy=multi-user.target
`

// ServiceName returns the systemd service name for the given type.
func ServiceName(t ServiceType) string {
	return fmt.Sprintf("half-tunnel-%s", t)
}

// ServiceFilePath returns the systemd service file path for the given type.
func ServiceFilePath(t ServiceType) string {
	return fmt.Sprintf("/etc/systemd/system/%s.service", ServiceName(t))
}

// Install installs the systemd service.
func Install(cfg *ServiceConfig) error {
	// Check if systemd is available
	if !isSystemdAvailable() {
		return fmt.Errorf("systemd is not available on this system")
	}

	// Validate binary exists
	if _, err := os.Stat(cfg.BinaryPath); os.IsNotExist(err) {
		return fmt.Errorf("binary not found: %s", cfg.BinaryPath)
	}

	// Validate config exists
	if _, err := os.Stat(cfg.ConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", cfg.ConfigPath)
	}

	// Set defaults
	if cfg.User == "" {
		cfg.User = "root"
	}
	if cfg.WorkingDir == "" {
		cfg.WorkingDir = filepath.Dir(cfg.ConfigPath)
	}

	// Generate service file content
	tmpl, err := template.New("service").Parse(serviceTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse service template: %w", err)
	}

	data := struct {
		Type       ServiceType
		TypeTitle  string
		BinaryPath string
		ConfigPath string
		User       string
		WorkingDir string
	}{
		Type:       cfg.Type,
		TypeTitle:  toTitleCase(string(cfg.Type)),
		BinaryPath: cfg.BinaryPath,
		ConfigPath: cfg.ConfigPath,
		User:       cfg.User,
		WorkingDir: cfg.WorkingDir,
	}

	// Create service file
	servicePath := ServiceFilePath(cfg.Type)
	f, err := os.Create(servicePath)
	if err != nil {
		return fmt.Errorf("failed to create service file: %w (try running with sudo)", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd
	if err := runSystemctl("daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	return nil
}

// Uninstall removes the systemd service.
func Uninstall(t ServiceType) error {
	if !isSystemdAvailable() {
		return fmt.Errorf("systemd is not available on this system")
	}

	serviceName := ServiceName(t)

	// Stop the service if running
	_ = runSystemctl("stop", serviceName)

	// Disable the service
	_ = runSystemctl("disable", serviceName)

	// Remove service file
	servicePath := ServiceFilePath(t)
	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	// Reload systemd
	if err := runSystemctl("daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	return nil
}

// Start starts the systemd service.
func Start(t ServiceType) error {
	return runSystemctl("start", ServiceName(t))
}

// Stop stops the systemd service.
func Stop(t ServiceType) error {
	return runSystemctl("stop", ServiceName(t))
}

// Restart restarts the systemd service.
func Restart(t ServiceType) error {
	return runSystemctl("restart", ServiceName(t))
}

// Enable enables the systemd service to start on boot.
func Enable(t ServiceType) error {
	return runSystemctl("enable", ServiceName(t))
}

// Disable disables the systemd service from starting on boot.
func Disable(t ServiceType) error {
	return runSystemctl("disable", ServiceName(t))
}

// Status returns the status of the systemd service.
func Status(t ServiceType) (string, error) {
	cmd := exec.Command("systemctl", "status", ServiceName(t))
	output, err := cmd.CombinedOutput()
	// systemctl status returns non-zero for inactive services
	return string(output), err
}

// IsInstalled checks if the service is installed.
func IsInstalled(t ServiceType) bool {
	_, err := os.Stat(ServiceFilePath(t))
	return err == nil
}

// IsRunning checks if the service is currently running.
func IsRunning(t ServiceType) bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", ServiceName(t))
	return cmd.Run() == nil
}

// Logs streams logs for the service.
// If follow is true, it follows the log output (like tail -f).
// If lines is > 0, it shows only the last N lines.
func Logs(t ServiceType, follow bool, lines int) error {
	args := []string{"-u", ServiceName(t)}

	if lines > 0 {
		args = append(args, "-n", fmt.Sprintf("%d", lines))
	} else {
		args = append(args, "-n", "100") // default to last 100 lines
	}

	if follow {
		args = append(args, "-f")
	}

	cmd := exec.Command("journalctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LogsOutput returns logs as a string.
func LogsOutput(t ServiceType, lines int) (string, error) {
	args := []string{"-u", ServiceName(t), "--no-pager"}

	if lines > 0 {
		args = append(args, "-n", fmt.Sprintf("%d", lines))
	}

	cmd := exec.Command("journalctl", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// runSystemctl runs a systemctl command.
func runSystemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// isSystemdAvailable checks if systemd is available.
func isSystemdAvailable() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

// toTitleCase converts a string to title case (first letter uppercase).
func toTitleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// GetDefaultBinaryPath returns the default binary path for the given service type.
func GetDefaultBinaryPath(t ServiceType) string {
	switch t {
	case ClientService:
		return "/usr/local/bin/ht-client"
	case ServerService:
		return "/usr/local/bin/ht-server"
	default:
		return ""
	}
}

// GetDefaultConfigPath returns the default config path for the given service type.
func GetDefaultConfigPath(t ServiceType) string {
	switch t {
	case ClientService:
		return "/etc/half-tunnel/client.yml"
	case ServerService:
		return "/etc/half-tunnel/server.yml"
	default:
		return ""
	}
}

// EnsureConfigDir ensures the config directory exists.
func EnsureConfigDir() error {
	return os.MkdirAll("/etc/half-tunnel", 0755)
}

// PrintServiceInfo prints information about the installed service.
func PrintServiceInfo(t ServiceType) {
	serviceName := ServiceName(t)
	fmt.Printf("\nService: %s\n", serviceName)
	fmt.Printf("Service file: %s\n", ServiceFilePath(t))
	fmt.Printf("Installed: %v\n", IsInstalled(t))
	fmt.Printf("Running: %v\n", IsRunning(t))
	fmt.Println("\nUseful commands:")
	fmt.Printf("  Start:   sudo systemctl start %s\n", serviceName)
	fmt.Printf("  Stop:    sudo systemctl stop %s\n", serviceName)
	fmt.Printf("  Restart: sudo systemctl restart %s\n", serviceName)
	fmt.Printf("  Status:  sudo systemctl status %s\n", serviceName)
	fmt.Printf("  Logs:    journalctl -u %s -f\n", serviceName)
}

// InteractiveInstall performs an interactive installation prompting for paths.
func InteractiveInstall(t ServiceType) error {
	reader := bufio.NewReader(os.Stdin)

	// Get binary path
	defaultBinary := GetDefaultBinaryPath(t)
	fmt.Printf("Binary path [%s]: ", defaultBinary)
	binaryPath, _ := reader.ReadString('\n')
	binaryPath = strings.TrimSpace(binaryPath)
	if binaryPath == "" {
		binaryPath = defaultBinary
	}

	// Get config path
	defaultConfig := GetDefaultConfigPath(t)
	fmt.Printf("Config path [%s]: ", defaultConfig)
	configPath, _ := reader.ReadString('\n')
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		configPath = defaultConfig
	}

	cfg := &ServiceConfig{
		Type:       t,
		BinaryPath: binaryPath,
		ConfigPath: configPath,
	}

	return Install(cfg)
}
