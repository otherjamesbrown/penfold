// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"bufio"
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

//go:embed templates/preferences.md
var preferencesTemplate string

//go:embed templates/processes.md
var processesTemplate string

//go:embed templates/acronym-review.md
var acronymReviewTemplate string

//go:embed templates/docs
var docsFS embed.FS

//go:embed templates/shared
var sharedFS embed.FS

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
2. Create ~/.penf/config.yaml (global config)
3. Test the connection to the gateway
4. Create CLAUDE.md in current directory (for Claude Code)
5. Create preferences.md in current directory (user settings - never overwritten)
6. Install process definitions in current directory
7. Install documentation hierarchy for Claude agents

Run this from your project directory. Global config goes to ~/.penf/,
but context files (CLAUDE.md, preferences.md, processes/, docs/) are created
in the current directory so Claude Code can find them.

After init, run 'penf init entities' to seed known people, products, and glossary.

Documentation (installed to docs/):
  docs/assistant-rules.md   How Penfold (the AI) should operate
  docs/index.md             System overview and navigation
  docs/shared/vision.md     What Penfold is and why
  docs/shared/entities.md   Data model and relationships`,
		RunE: runInit,
	}

	initCmd.Flags().StringVar(&initServerAddr, "server", "", "Gateway server address (host:port)")
	initCmd.Flags().BoolVar(&initNonInteractive, "non-interactive", false, "Skip prompts, use defaults or flags")

	// Add subcommands
	initCmd.AddCommand(NewInitEntitiesCommand())

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
	cwd, _ := os.Getwd()
	if err := downloadAssistantClaudeMd(cfg); err != nil {
		fmt.Printf("  \033[33mWarning:\033[0m Could not download assistant CLAUDE.md: %v\n", err)
		fmt.Println("  You can manually create this file later or run 'penf update' to retry.")
	} else {
		fmt.Printf("  \033[32m✓\033[0m Assistant CLAUDE.md saved to %s\n", filepath.Join(cwd, "CLAUDE.md"))
	}
	fmt.Println()

	// Step 6: Create user preferences file (only if it doesn't exist).
	fmt.Println("Setting up user preferences...")
	if err := initUserPreferences(); err != nil {
		fmt.Printf("  \033[33mWarning:\033[0m Could not create preferences: %v\n", err)
	}
	fmt.Println()

	// Step 7: Create/update process definitions.
	fmt.Println("Installing process definitions...")
	if err := initProcessDefinitions(); err != nil {
		fmt.Printf("  \033[33mWarning:\033[0m Could not create process files: %v\n", err)
	}
	fmt.Println()

	// Step 8: Install documentation for Claude agents.
	fmt.Println("Installing documentation...")
	if err := initDocs(); err != nil {
		fmt.Printf("  \033[33mWarning:\033[0m Could not install docs: %v\n", err)
	}
	fmt.Println()

	// Summary.
	fmt.Println("Initialization complete!")
	fmt.Println()
	fmt.Println("Configuration summary:")
	fmt.Printf("  Server address:  %s\n", cfg.ServerAddress)
	fmt.Printf("  Config file:     %s\n", configPath)
	fmt.Printf("  CLAUDE.md:       %s\n", filepath.Join(cwd, "CLAUDE.md"))
	fmt.Printf("  Preferences:     %s\n", filepath.Join(cwd, "preferences.md"))
	fmt.Printf("  Processes:       %s\n", filepath.Join(cwd, "processes/"))
	fmt.Printf("  Documentation:   %s\n", filepath.Join(cwd, "docs/"))
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  • Edit preferences.md to customize your settings")
	fmt.Println("  • Run 'penf init entities' to seed known people, products, glossary")
	fmt.Println("  • Run 'penf status' to verify the connection")
	fmt.Println("  • Run 'penf health' to check system health")
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

// downloadAssistantClaudeMd creates the assistant CLAUDE.md in current directory.
func downloadAssistantClaudeMd(cfg *config.CLIConfig) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	claudeMdPath := filepath.Join(cwd, "CLAUDE.md")

	// For now, create a default assistant CLAUDE.md.
	// In the future, this could fetch from the gateway or a central repository.
	content := generateAssistantClaudeMd(cfg)

	if err := os.WriteFile(claudeMdPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing CLAUDE.md: %w", err)
	}

	return nil
}

// initUserPreferences creates the preferences.md file if it doesn't exist.
// This file is NEVER overwritten - it belongs to the user.
func initUserPreferences() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	prefsPath := filepath.Join(cwd, "preferences.md")

	// Check if preferences already exist - never overwrite
	if _, err := os.Stat(prefsPath); err == nil {
		fmt.Printf("  \033[32m✓\033[0m preferences.md already exists (not modified)\n")
		return nil
	}

	// Create new preferences file
	if err := os.WriteFile(prefsPath, []byte(preferencesTemplate), 0644); err != nil {
		return fmt.Errorf("writing preferences.md: %w", err)
	}

	fmt.Printf("  \033[32m✓\033[0m Created preferences.md\n")
	fmt.Println("    Edit preferences.md to customize your settings")
	return nil
}

// initProcessDefinitions creates/updates process definition files in the current directory.
// These CAN be updated by penf init or penf update.
func initProcessDefinitions() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	processDir := filepath.Join(cwd, "processes")

	// Create processes directory
	if err := os.MkdirAll(processDir, 0755); err != nil {
		return fmt.Errorf("creating processes directory: %w", err)
	}

	// Write/update processes index
	indexPath := filepath.Join(cwd, "processes.md")
	if err := os.WriteFile(indexPath, []byte(processesTemplate), 0644); err != nil {
		return fmt.Errorf("writing processes.md: %w", err)
	}
	fmt.Printf("  \033[32m✓\033[0m Updated processes.md index\n")

	// Write/update acronym-review process
	acronymPath := filepath.Join(processDir, "acronym-review.md")
	if err := os.WriteFile(acronymPath, []byte(acronymReviewTemplate), 0644); err != nil {
		return fmt.Errorf("writing acronym-review.md: %w", err)
	}
	fmt.Printf("  \033[32m✓\033[0m Updated processes/acronym-review.md\n")

	return nil
}

// initDocs installs the documentation hierarchy for Claude agents.
// These CAN be updated by penf init or penf update.
// Structure:
//   docs/           - Client docs (concepts, workflows)
//   docs/shared/    - Shared docs (vision, entities, use-cases)
func initDocs() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	docsDir := filepath.Join(cwd, "docs")

	// Walk the embedded docs filesystem and copy all files
	err = fs.WalkDir(docsFS, "templates/docs", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Calculate the relative path (strip "templates/docs" prefix)
		relPath, err := filepath.Rel("templates/docs", path)
		if err != nil {
			return err
		}

		// Skip the root
		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(docsDir, relPath)

		if d.IsDir() {
			// Create directory
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("creating directory %s: %w", destPath, err)
			}
			return nil
		}

		// Read and write file
		content, err := docsFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", destPath, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("installing docs: %w", err)
	}

	// Also install shared docs (vision, entities, use-cases, interaction-model)
	sharedDir := filepath.Join(docsDir, "shared")
	if err := os.MkdirAll(sharedDir, 0755); err != nil {
		return fmt.Errorf("creating shared directory: %w", err)
	}

	err = fs.WalkDir(sharedFS, "templates/shared", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Calculate the relative path (strip "templates/shared" prefix)
		relPath, err := filepath.Rel("templates/shared", path)
		if err != nil {
			return err
		}

		// Skip the root
		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(sharedDir, relPath)

		if d.IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("creating directory %s: %w", destPath, err)
			}
			return nil
		}

		// Read and write file
		content, err := sharedFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", destPath, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("installing shared docs: %w", err)
	}

	fmt.Printf("  \033[32m✓\033[0m Installed docs/ hierarchy (concepts, workflows, shared)\n")
	fmt.Println("    Claude reads docs/index.md first, then follows links for details")
	fmt.Println("    Shared docs (vision, entities, use-cases) are in docs/shared/")

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
