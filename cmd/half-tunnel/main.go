// Package main provides the entry point for the Half-Tunnel CLI.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/sahmadiut/half-tunnel/internal/config"
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
		fmt.Printf("half-tunnel %s (commit: %s, built: %s)\n", version, commit, buildDate)
		os.Exit(0)
	}
	
	// Handle help for no arguments
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}
	
	// Route to subcommand
	switch os.Args[1] {
	case "config":
		runConfigCommand(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Half-Tunnel - Split-path VPN System

Usage:
  half-tunnel <command> [options]

Commands:
  config    Manage configuration files (generate, validate, sample)
  help      Show this help message

Flags:
  -v, --version    Show version information
  -h, --help       Show this help message

Use "half-tunnel <command> --help" for more information about a command.`)
}

func runConfigCommand(args []string) {
	if len(args) == 0 {
		printConfigUsage()
		os.Exit(0)
	}
	
	switch args[0] {
	case "generate":
		runConfigGenerate(args[1:])
	case "validate":
		runConfigValidate(args[1:])
	case "sample":
		runConfigSample(args[1:])
	case "help", "--help", "-h":
		printConfigUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown config subcommand: %s\n", args[0])
		printConfigUsage()
		os.Exit(1)
	}
}

func printConfigUsage() {
	fmt.Println(`Manage Half-Tunnel configuration files

Usage:
  half-tunnel config <subcommand> [options]

Subcommands:
  generate    Generate a new configuration file
  validate    Validate an existing configuration file
  sample      Print a sample configuration

Use "half-tunnel config <subcommand> --help" for more information.`)
}

func runConfigGenerate(args []string) {
	fs := pflag.NewFlagSet("generate", pflag.ExitOnError)
	
	configType := fs.String("type", "", "Configuration type: 'client' or 'server' (required)")
	output := fs.String("output", "", "Output file path")
	
	// Server flags
	upstreamPort := fs.Int("upstream-port", 0, "Upstream listener port (server)")
	downstreamPort := fs.Int("downstream-port", 0, "Downstream listener port (server)")
	tlsCert := fs.String("tls-cert", "", "TLS certificate path")
	tlsKey := fs.String("tls-key", "", "TLS key path")
	
	// Client flags
	upstreamURL := fs.String("upstream-url", "", "Upstream server URL (client)")
	downstreamURL := fs.String("downstream-url", "", "Downstream server URL (client)")
	portForwards := fs.StringArray("port-forward", nil, "Port forward specification (can be specified multiple times)")
	socks5Port := fs.Int("socks5-port", 0, "SOCKS5 proxy port (client)")
	
	fs.Usage = func() {
		fmt.Println(`Generate a new configuration file

Usage:
  half-tunnel config generate --type <client|server> [options]

Options:`)
		fs.PrintDefaults()
		fmt.Println(`
Examples:
  # Generate server config interactively
  half-tunnel config generate --type server --output server.yml

  # Generate client config interactively
  half-tunnel config generate --type client --output client.yml

  # Generate client config with flags (non-interactive)
  half-tunnel config generate --type client \
    --upstream-url "wss://domain-a.example.com:8443/ws/upstream" \
    --downstream-url "wss://domain-b.example.com:8444/ws/downstream" \
    --port-forward "2083" \
    --port-forward "8080:example.com:80" \
    --socks5-port 1080 \
    --output client.yml

  # Generate server config with flags
  half-tunnel config generate --type server \
    --upstream-port 8443 \
    --downstream-port 8444 \
    --tls-cert /path/to/cert.pem \
    --tls-key /path/to/key.pem \
    --output server.yml

Port forward formats:
  - "2083"                    Listen on 2083, forward to remote:2083
  - "8080:80"                 Listen on 8080, forward to remote:80
  - "8080:example.com:80"     Listen on 8080, forward to example.com:80`)
	}
	
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	
	if *configType == "" {
		fmt.Fprintln(os.Stderr, "Error: --type is required")
		fs.Usage()
		os.Exit(1)
	}
	
	// Determine if we should run interactively (when no CLI options are provided)
	hasNonInteractiveOptions := *upstreamPort > 0 ||
		*downstreamPort > 0 ||
		*upstreamURL != "" ||
		*downstreamURL != "" ||
		len(*portForwards) > 0 ||
		*socks5Port > 0
	
	opts := config.GenerateOptions{
		OutputPath:     *output,
		UpstreamPort:   *upstreamPort,
		DownstreamPort: *downstreamPort,
		TLSCert:        *tlsCert,
		TLSKey:         *tlsKey,
		UpstreamURL:    *upstreamURL,
		DownstreamURL:  *downstreamURL,
		PortForwards:   *portForwards,
		SOCKS5Port:     *socks5Port,
		EnableSOCKS5:   *socks5Port > 0,
	}
	
	var generator *config.ConfigGenerator
	if hasNonInteractiveOptions {
		generator = config.NewNonInteractiveGenerator()
	} else {
		generator = config.NewInteractiveGenerator()
	}
	
	switch *configType {
	case "client":
		clientCfg, err := generator.GenerateClientConfig(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating client config: %v\n", err)
			os.Exit(1)
		}
		
		if *output != "" {
			if err := config.WriteClientConfigToFile(clientCfg, *output); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✅ Configuration saved to: %s\n", *output)
		} else {
			content, _ := config.RenderClientConfigYAML(clientCfg)
			fmt.Println(content)
		}
		
	case "server":
		serverCfg, err := generator.GenerateServerConfig(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating server config: %v\n", err)
			os.Exit(1)
		}
		
		if *output != "" {
			if err := config.WriteServerConfigToFile(serverCfg, *output); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✅ Configuration saved to: %s\n", *output)
		} else {
			content, _ := config.RenderServerConfigYAML(serverCfg)
			fmt.Println(content)
		}
		
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown config type: %s (use 'client' or 'server')\n", *configType)
		os.Exit(1)
	}
}

func runConfigValidate(args []string) {
	fs := pflag.NewFlagSet("validate", pflag.ExitOnError)
	
	configPath := fs.String("config", "", "Path to configuration file (required)")
	configType := fs.String("type", "", "Configuration type: 'client' or 'server' (optional, auto-detected if not specified)")
	
	fs.Usage = func() {
		fmt.Println(`Validate an existing configuration file

Usage:
  half-tunnel config validate --config <path> [--type <client|server>]

Options:`)
		fs.PrintDefaults()
	}
	
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	
	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --config is required")
		fs.Usage()
		os.Exit(1)
	}
	
	// Auto-detect type if not specified
	if *configType == "" {
		// Try to detect from filename
		if strings.Contains(*configPath, "client") {
			*configType = "client"
		} else if strings.Contains(*configPath, "server") {
			*configType = "server"
		} else {
			fmt.Fprintln(os.Stderr, "Error: could not auto-detect config type, please specify --type")
			os.Exit(1)
		}
	}
	
	if err := config.ValidateConfigFile(*configPath, *configType); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Validation failed: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("✅ Configuration is valid: %s\n", *configPath)
}

func runConfigSample(args []string) {
	fs := pflag.NewFlagSet("sample", pflag.ExitOnError)
	
	configType := fs.String("type", "", "Configuration type: 'client' or 'server' (required)")
	
	fs.Usage = func() {
		fmt.Println(`Print a sample configuration

Usage:
  half-tunnel config sample --type <client|server>

Options:`)
		fs.PrintDefaults()
	}
	
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	
	if *configType == "" {
		fmt.Fprintln(os.Stderr, "Error: --type is required")
		fs.Usage()
		os.Exit(1)
	}
	
	switch *configType {
	case "client":
		fmt.Println(config.GetSampleClientConfig())
	case "server":
		fmt.Println(config.GetSampleServerConfig())
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown config type: %s (use 'client' or 'server')\n", *configType)
		os.Exit(1)
	}
}
