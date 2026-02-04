// Package main provides the entry point for the Half-Tunnel CLI.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/pflag"
)

const (
	defaultServiceInstallDir = "/usr/local/bin"
	serviceNameClient        = "half-tunnel-client"
	serviceNameServer        = "half-tunnel-server"
)

func runServiceCommand(args []string) {
	if len(args) == 0 {
		printServiceUsage()
		os.Exit(0)
	}

	switch args[0] {
	case "install":
		runServiceInstall(args[1:])
	case "start":
		runServiceAction(args[1:], "start")
	case "stop":
		runServiceAction(args[1:], "stop")
	case "status":
		runServiceAction(args[1:], "status")
	case "logs":
		runServiceLogs(args[1:])
	case "help", "--help", "-h":
		printServiceUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown service subcommand: %s\n", args[0])
		printServiceUsage()
		os.Exit(1)
	}
}

func printServiceUsage() {
	fmt.Println(`Manage Half-Tunnel systemd services

Usage:
  half-tunnel service <subcommand> [options]

Subcommands:
  install   Install systemd service files
  start     Start a service
  stop      Stop a service
  status    Show service status
  logs      Show service logs

Use "half-tunnel service <subcommand> --help" for more information.`)
}

func runServiceInstall(args []string) {
	fs := pflag.NewFlagSet("install", pflag.ExitOnError)
	serviceType := fs.String("type", "", "Service type: 'client' or 'server' (required)")
	installDir := fs.String("install-dir", defaultServiceInstallDir, "Install directory for binaries")
	configPath := fs.String("config", "", "Config file path (default: /etc/half-tunnel/<type>.yml)")
	user := fs.String("user", "root", "User to run the service")
	force := fs.Bool("force", false, "Overwrite existing service file if it exists")

	fs.Usage = func() {
		fmt.Println(`Install systemd service files

Usage:
  half-tunnel service install --type <client|server> [options]

Options:`)
		fs.PrintDefaults()
		fmt.Println(`
Examples:
  half-tunnel service install --type server
  half-tunnel service install --type client --config /etc/half-tunnel/client.yml
  half-tunnel service install --type server --install-dir /opt/half-tunnel/bin
  half-tunnel service install --type client --force`)
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *serviceType == "" {
		fmt.Fprintln(os.Stderr, "Error: --type is required")
		fs.Usage()
		os.Exit(1)
	}

	serviceTypeValue := strings.ToLower(strings.TrimSpace(*serviceType))
	if serviceTypeValue != "client" && serviceTypeValue != "server" {
		fmt.Fprintln(os.Stderr, "Error: --type must be 'client' or 'server'")
		os.Exit(1)
	}

	installDirValue := strings.TrimSpace(*installDir)
	if installDirValue == "" {
		installDirValue = defaultServiceInstallDir
	}

	configValue := strings.TrimSpace(*configPath)
	if configValue == "" {
		configValue = fmt.Sprintf("/etc/half-tunnel/%s.yml", serviceTypeValue)
	}

	serviceName := serviceNameServer
	executable := "ht-server"
	description := "Half-Tunnel Server"
	if serviceTypeValue == "client" {
		serviceName = serviceNameClient
		executable = "ht-client"
		description = "Half-Tunnel Client"
	}

	serviceContent := renderServiceFile(description, installDirValue, executable, configValue, *user)
	servicePath := fmt.Sprintf("/etc/systemd/system/%s.service", serviceName)

	if err := writeServiceFile(servicePath, serviceContent, *force); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing service file: %v\n", err)
		os.Exit(1)
	}

	if err := runSystemctl("daemon-reload"); err != nil {
		fmt.Fprintf(os.Stderr, "Error reloading systemd: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Service installed: %s\n", servicePath)
	fmt.Println("Next steps:")
	fmt.Printf("  sudo systemctl enable %s\n", serviceName)
	fmt.Printf("  sudo systemctl start %s\n", serviceName)
}

func runServiceAction(args []string, action string) {
	fs := pflag.NewFlagSet(action, pflag.ExitOnError)
	serviceType := fs.String("type", "", "Service type: 'client' or 'server' (required)")

	fs.Usage = func() {
		fmt.Printf(`%s a systemd service

Usage:
  half-tunnel service %s --type <client|server>

Options:
`, formatActionTitle(action), action)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	serviceName, err := parseServiceName(*serviceType)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fs.Usage()
		os.Exit(1)
	}

	if err := runSystemctl(action, serviceName); err != nil {
		fmt.Fprintf(os.Stderr, "Error running systemctl %s: %v\n", action, err)
		os.Exit(1)
	}
}

func runServiceLogs(args []string) {
	fs := pflag.NewFlagSet("logs", pflag.ExitOnError)
	serviceType := fs.String("type", "", "Service type: 'client' or 'server' (required)")
	tail := fs.Int("tail", 100, "Number of log lines to show")

	fs.Usage = func() {
		fmt.Println(`Show systemd service logs

Usage:
  half-tunnel service logs --type <client|server> [options]

Options:`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	serviceName, err := parseServiceName(*serviceType)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fs.Usage()
		os.Exit(1)
	}

	tailCount := *tail
	if tailCount <= 0 {
		tailCount = 100
	}

	argsList := []string{"-u", serviceName, "--no-pager", fmt.Sprintf("--lines=%d", tailCount)}
	cmd := exec.Command("journalctl", argsList...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running journalctl: %v\n", err)
		os.Exit(1)
	}
}

func parseServiceName(serviceType string) (string, error) {
	serviceTypeValue := strings.ToLower(strings.TrimSpace(serviceType))
	switch serviceTypeValue {
	case "client":
		return serviceNameClient, nil
	case "server":
		return serviceNameServer, nil
	default:
		return "", fmt.Errorf("Error: --type must be 'client' or 'server'")
	}
}

func renderServiceFile(description, installDir, executable, configPath, user string) string {
	return fmt.Sprintf(`[Unit]
Description=%s
Documentation=https://github.com/sahmadiut/half-tunnel
After=network.target

[Service]
Type=simple
ExecStart=%s/%s -config %s
Restart=always
RestartSec=5
User=%s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
`, description, installDir, executable, configPath, user)
}

func writeServiceFile(path, content string, overwrite bool) error {
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("service file already exists (use --force to overwrite): %s", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func runSystemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func formatActionTitle(action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return ""
	}
	return strings.ToUpper(action[:1]) + action[1:]
}
