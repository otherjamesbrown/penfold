# Task 10: penf cert init

**Status**: pending | **Phase**: 4 - CLI Commands

## Objective

Add `penf cert init` command to set up client certificates.

## Output

`cmd/penf/commands/cert_init.go`

## Command Usage

```bash
# Interactive: prompts for CA location
penf cert init

# Non-interactive: specify paths
penf cert init --ca-dir ~/secrets/penfold-ca --name dev-macbook

# Just copy existing certs
penf cert init --from /path/to/certs
```

## Implementation

```go
package commands

import (
    "github.com/spf13/cobra"
)

var certInitCmd = &cobra.Command{
    Use:   "init",
    Short: "Initialize client certificates for TLS authentication",
    Long: `Sets up client certificates for mTLS authentication with the gateway.

This command can:
1. Generate a new client certificate signed by the penfold CA
2. Copy existing certificates to the config directory

Certificates are stored in ~/.config/penf/certs/`,
    RunE: runCertInit,
}

func runCertInit(cmd *cobra.Command, args []string) error {
    // 1. Check if certs already exist
    certDir := getCertDir()
    if certsExist(certDir) && !forceFlag {
        return fmt.Errorf("certificates already exist in %s (use --force to overwrite)", certDir)
    }

    // 2. Option A: Copy from existing location
    if fromDir != "" {
        return copyCertsFrom(fromDir, certDir)
    }

    // 3. Option B: Generate new cert
    if caDir == "" {
        // Interactive prompt
        caDir = promptForCADir()
    }

    // Generate using the create-client-cert.sh script or Go implementation
    return generateClientCert(caDir, certDir, clientName)
}

func init() {
    certCmd.AddCommand(certInitCmd)
    certInitCmd.Flags().StringVar(&caDir, "ca-dir", "", "Directory containing CA cert and key")
    certInitCmd.Flags().StringVar(&fromDir, "from", "", "Copy existing certs from directory")
    certInitCmd.Flags().StringVar(&clientName, "name", hostname(), "Client name for certificate CN")
    certInitCmd.Flags().BoolVar(&forceFlag, "force", false, "Overwrite existing certificates")
}
```

## Acceptance Criteria

- [ ] Creates ~/.config/penf/certs/ directory
- [ ] --from copies existing certs
- [ ] --ca-dir generates new cert
- [ ] Interactive mode if no flags
- [ ] --force to overwrite
- [ ] Updates ~/.penf/config.yaml with TLS settings

## Notes

- Could call the shell script or implement in Go
- Go implementation more portable
- Should validate certs after copying/generating
