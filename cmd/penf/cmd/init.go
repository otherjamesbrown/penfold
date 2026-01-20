// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

var (
	initServerAddr string
	initNonInteractive bool
)

// NewInitCommand creates the init command.
func NewInitCommand() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize penf configuration",
		Long: `Initialize penf for first-time use.

This command will:
1. Prompt for the gateway server address
2. Create ~/.penf/config.yaml
3. Test the connection to the gateway
4. Download/update the assistant CLAUDE.md

Run this command on a new machine or to reconfigure an existing setup.`,
		RunE: runInit,
	}

	initCmd.Flags().StringVar(&initServerAddr, "server", "", "Gateway server address (host:port)")
	initCmd.Flags().BoolVar(&initNonInteractive, "non-interactive", false, "Skip prompts, use defaults or flags")

	return initCmd
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Println("Penfold CLI Initialization")
	fmt.Println("==========================")
	fmt.Println()

	// Load existing config if present.
	existingCfg, _ := config.LoadConfig()
	cfg := config.DefaultConfig()

	// Step 1: Get server address.
	serverAddr := initServerAddr
	if serverAddr == "" && !initNonInteractive {
		defaultAddr := config.DefaultServerAddress
		if existingCfg != nil && existingCfg.ServerAddress != "" {
			defaultAddr = existingCfg.ServerAddress
		}

		serverAddr = promptWithDefault("Gateway server address", defaultAddr)
	} else if serverAddr == "" {
		serverAddr = config.DefaultServerAddress
		if existingCfg != nil && existingCfg.ServerAddress != "" {
			serverAddr = existingCfg.ServerAddress
		}
	}
	cfg.ServerAddress = serverAddr

	// Step 2: Preserve other settings from existing config.
	if existingCfg != nil {
		if existingCfg.TenantID != "" {
			cfg.TenantID = existingCfg.TenantID
		}
		if existingCfg.TenantAliases != nil {
			cfg.TenantAliases = existingCfg.TenantAliases
		}
	}

	// Step 3: Test connection.
	fmt.Println()
	fmt.Printf("Testing connection to %s...\n", serverAddr)

	if err := testGatewayConnection(serverAddr); err != nil {
		fmt.Printf("  \033[33mWarning:\033[0m Could not connect to gateway: %v\n", err)
		fmt.Println("  Configuration will be saved, but you may need to check your server address.")
		fmt.Println()
	} else {
		fmt.Printf("  \033[32m✓\033[0m Successfully connected to gateway\n")
		fmt.Println()
	}

	// Step 4: Save configuration.
	configPath, _ := config.ConfigPath()
	fmt.Printf("Saving configuration to %s...\n", configPath)

	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("saving configuration: %w", err)
	}
	fmt.Printf("  \033[32m✓\033[0m Configuration saved\n")
	fmt.Println()

	// Step 5: Download/update assistant CLAUDE.md.
	fmt.Println("Updating assistant configuration...")
	if err := downloadAssistantClaudeMd(cfg); err != nil {
		fmt.Printf("  \033[33mWarning:\033[0m Could not download assistant CLAUDE.md: %v\n", err)
		fmt.Println("  You can manually create this file later or run 'penf update' to retry.")
	} else {
		configDir, _ := config.ConfigDir()
		fmt.Printf("  \033[32m✓\033[0m Assistant CLAUDE.md saved to %s\n", filepath.Join(configDir, "CLAUDE.md"))
	}
	fmt.Println()

	// Summary.
	fmt.Println("Initialization complete!")
	fmt.Println()
	fmt.Println("Configuration summary:")
	fmt.Printf("  Server address: %s\n", cfg.ServerAddress)
	fmt.Printf("  Config file:    %s\n", configPath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  • Run 'penf status' to verify the connection")
	fmt.Println("  • Run 'penf health' to check system health")
	fmt.Println("  • Run 'penf search <query>' to search your content")
	fmt.Println()

	return nil
}

// promptWithDefault prompts the user for input with a default value.
func promptWithDefault(prompt, defaultValue string) string {
	reader := bufio.NewReader(os.Stdin)

	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Printf("%s: ", prompt)
	}

	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultValue
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}

	return input
}

// testGatewayConnection tests the connection to the gateway.
func testGatewayConnection(serverAddr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	conn, err := grpc.DialContext(ctx, serverAddr, opts...)
	if err != nil {
		return fmt.Errorf("connecting to gateway: %w", err)
	}
	defer conn.Close()

	return nil
}

// downloadAssistantClaudeMd downloads or creates the assistant CLAUDE.md.
func downloadAssistantClaudeMd(cfg *config.CLIConfig) error {
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("getting config directory: %w", err)
	}

	claudeMdPath := filepath.Join(configDir, "CLAUDE.md")

	// For now, create a default assistant CLAUDE.md.
	// In the future, this could fetch from the gateway or a central repository.
	content := generateAssistantClaudeMd(cfg)

	if err := os.WriteFile(claudeMdPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing CLAUDE.md: %w", err)
	}

	return nil
}

// generateAssistantClaudeMd generates the assistant CLAUDE.md content.
func generateAssistantClaudeMd(cfg *config.CLIConfig) string {
	return fmt.Sprintf(`# Penfold Assistant Configuration

This file provides context to Claude Code when working with Penfold CLI.

## Configuration

- **Server Address:** %s
- **Config Directory:** ~/.penf/

## Quick Reference

### Common Commands
`+"```"+`bash
penf status              # Check connection status
penf health              # View system health
penf search <query>      # Search content

# Glossary management
penf glossary list       # List all terms
penf glossary add <term> # Add a new term
penf glossary lookup <text>  # Look up terms in text

# Review questions
penf review questions list    # List pending questions
penf review questions next    # Get next question
penf review questions resolve <id> <answer>  # Answer a question

# Ingestion
penf ingest meeting <file>    # Ingest meeting transcript
penf ingest email             # Start email sync
`+"```"+`

### Environment Variables
- ` + "`PENF_SERVER_ADDRESS`" + ` - Override server address
- ` + "`PENF_TIMEOUT`" + ` - Request timeout (default: 30s)
- ` + "`PENF_OUTPUT_FORMAT`" + ` - Output format (text, json, yaml)
- ` + "`PENF_TENANT_ID`" + ` - Tenant identifier

## Architecture

Penfold CLI communicates with the gateway service via gRPC. The gateway handles:
- Authentication and authorization
- Request routing to backend services
- Rate limiting and connection management

The CLI does not have direct database access. All operations go through the gateway API.

## Troubleshooting

If you encounter connection issues:
1. Verify the server address: ` + "`penf status`" + `
2. Check server health: ` + "`penf health`" + `
3. Ensure the gateway is running on the configured address
4. Check firewall rules allow gRPC traffic (port 50051)

## More Information

Run ` + "`penf help`" + ` for full command documentation.
Run ` + "`penf <command> --help`" + ` for command-specific help.
`, cfg.ServerAddress)
}
