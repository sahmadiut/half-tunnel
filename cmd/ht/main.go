// Package main provides the entry point for the ht CLI - a unified service management tool.
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/sahmadiut/half-tunnel/internal/service"
	"github.com/spf13/pflag"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	// Handle version flag early
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("ht %s (commit: %s, built: %s)\n", version, commit, buildDate)
		os.Exit(0)
	}

	// Handle help for no arguments
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	// Route to subcommand
	switch os.Args[1] {
	case "client", "c":
		runClientCommand(os.Args[2:])
	case "server", "s":
		runServerCommand(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`ht - Half-Tunnel Service Manager

Usage:
  ht <service> <command> [options]

Services:
  client, c    Manage the client service
  server, s    Manage the server service

Commands:
  install      Install the systemd service
  uninstall    Remove the systemd service
  start        Start the service
  stop         Stop the service
  restart      Restart the service
  enable       Enable service autostart on boot
  disable      Disable service autostart
  status       Show service status
  logs         View service logs (default: follow mode)

Flags:
  -v, --version    Show version information
  -h, --help       Show this help message

Examples:
  ht c install --config /etc/half-tunnel/client.yml
  ht s start
  ht client logs
  ht server logs -n 50
  ht c restart

Use "ht <service> <command> --help" for more information.`)
}

func runClientCommand(args []string) {
	runServiceCommand(service.ClientService, args)
}

func runServerCommand(args []string) {
	runServiceCommand(service.ServerService, args)
}

func runServiceCommand(svcType service.ServiceType, args []string) {
	if len(args) == 0 {
		printServiceUsage(svcType)
		os.Exit(0)
	}

	switch args[0] {
	case "install":
		runInstall(svcType, args[1:])
	case "uninstall":
		runUninstall(svcType, args[1:])
	case "start":
		runStart(svcType)
	case "stop":
		runStop(svcType)
	case "restart":
		runRestart(svcType)
	case "enable":
		runEnable(svcType)
	case "disable":
		runDisable(svcType)
	case "status":
		runStatus(svcType)
	case "logs", "log", "l":
		runLogs(svcType, args[1:])
	case "help", "--help", "-h":
		printServiceUsage(svcType)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		printServiceUsage(svcType)
		os.Exit(1)
	}
}

func printServiceUsage(svcType service.ServiceType) {
	fmt.Printf(`Manage the %s service

Usage:
  ht %s <command> [options]

Commands:
  install      Install the systemd service
  uninstall    Remove the systemd service
  start        Start the service
  stop         Stop the service
  restart      Restart the service
  enable       Enable service autostart on boot
  disable      Disable service autostart
  status       Show service status
  logs, log, l View service logs

Install Options:
  --binary, -b   Path to the binary (default: %s)
  --config, -c   Path to the config file (default: %s)
  --user, -u     User to run the service as (default: root)

Logs Options:
  -f, --follow   Follow log output (default: true)
  -n, --lines    Number of lines to show (default: 100)
  --no-follow    Disable follow mode

Examples:
  ht %s install --config /etc/half-tunnel/%s.yml
  ht %s start
  ht %s logs
  ht %s logs -n 50 --no-follow
`, svcType, svcType,
		service.GetDefaultBinaryPath(svcType),
		service.GetDefaultConfigPath(svcType),
		svcType, svcType, svcType, svcType, svcType)
}

func runInstall(svcType service.ServiceType, args []string) {
	fs := pflag.NewFlagSet("install", pflag.ExitOnError)

	binaryPath := fs.StringP("binary", "b", service.GetDefaultBinaryPath(svcType), "Path to the binary")
	configPath := fs.StringP("config", "c", service.GetDefaultConfigPath(svcType), "Path to the config file")
	user := fs.StringP("user", "u", "root", "User to run the service as")

	fs.Usage = func() {
		fmt.Printf(`Install the %s systemd service

Usage:
  ht %s install [options]

Options:
`, svcType, svcType)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	// Ensure config directory exists
	if err := service.EnsureConfigDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not create config directory: %v\n", err)
	}

	cfg := &service.ServiceConfig{
		Type:       svcType,
		BinaryPath: *binaryPath,
		ConfigPath: *configPath,
		User:       *user,
	}

	if err := service.Install(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to install service: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Service %s installed successfully!\n", service.ServiceName(svcType))
	service.PrintServiceInfo(svcType)
}

func runUninstall(svcType service.ServiceType, args []string) {
	fs := pflag.NewFlagSet("uninstall", pflag.ExitOnError)
	force := fs.BoolP("force", "f", false, "Force uninstall without confirmation")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if !*force {
		fmt.Printf("Are you sure you want to uninstall %s? [y/N] ", service.ServiceName(svcType))
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Cancelled.")
			return
		}
	}

	if err := service.Uninstall(svcType); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to uninstall service: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Service %s uninstalled successfully!\n", service.ServiceName(svcType))
}

func runStart(svcType service.ServiceType) {
	if !service.IsInstalled(svcType) {
		fmt.Fprintf(os.Stderr, "❌ Service %s is not installed. Run 'ht %s install' first.\n",
			service.ServiceName(svcType), svcType)
		os.Exit(1)
	}

	if err := service.Start(svcType); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to start service: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Service %s started!\n", service.ServiceName(svcType))
}

func runStop(svcType service.ServiceType) {
	if err := service.Stop(svcType); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to stop service: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Service %s stopped!\n", service.ServiceName(svcType))
}

func runRestart(svcType service.ServiceType) {
	if !service.IsInstalled(svcType) {
		fmt.Fprintf(os.Stderr, "❌ Service %s is not installed. Run 'ht %s install' first.\n",
			service.ServiceName(svcType), svcType)
		os.Exit(1)
	}

	if err := service.Restart(svcType); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to restart service: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Service %s restarted!\n", service.ServiceName(svcType))
}

func runEnable(svcType service.ServiceType) {
	if !service.IsInstalled(svcType) {
		fmt.Fprintf(os.Stderr, "❌ Service %s is not installed. Run 'ht %s install' first.\n",
			service.ServiceName(svcType), svcType)
		os.Exit(1)
	}

	if err := service.Enable(svcType); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to enable service: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Service %s enabled for autostart!\n", service.ServiceName(svcType))
}

func runDisable(svcType service.ServiceType) {
	if err := service.Disable(svcType); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to disable service: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Service %s disabled from autostart!\n", service.ServiceName(svcType))
}

func runStatus(svcType service.ServiceType) {
	if !service.IsInstalled(svcType) {
		fmt.Printf("Service %s is not installed.\n", service.ServiceName(svcType))
		return
	}

	status, _ := service.Status(svcType)
	fmt.Println(status)
}

func runLogs(svcType service.ServiceType, args []string) {
	fs := pflag.NewFlagSet("logs", pflag.ExitOnError)

	follow := fs.BoolP("follow", "f", true, "Follow log output")
	noFollow := fs.Bool("no-follow", false, "Disable follow mode")
	lines := fs.IntP("lines", "n", 100, "Number of lines to show")

	fs.Usage = func() {
		fmt.Printf(`View logs for the %s service

Usage:
  ht %s logs [options]

Options:
`, svcType, svcType)
		fs.PrintDefaults()
		fmt.Println(`
Note: Follow mode is enabled by default. Use --no-follow to disable.
Press Ctrl+C to stop following logs.`)
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	// Handle -n with string argument
	for i, arg := range args {
		if arg == "-n" && i+1 < len(args) {
			if n, err := strconv.Atoi(args[i+1]); err == nil {
				*lines = n
			}
		}
	}

	// no-follow overrides follow
	if *noFollow {
		*follow = false
	}

	if err := service.Logs(svcType, *follow, *lines); err != nil {
		// Don't treat signal interrupt as error
		if err.Error() != "signal: interrupt" {
			fmt.Fprintf(os.Stderr, "❌ Failed to get logs: %v\n", err)
			os.Exit(1)
		}
	}
}
