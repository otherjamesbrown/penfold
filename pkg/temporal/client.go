// Package temporal provides Temporal client factory and configuration for Penfold services.
package temporal

import (
	"fmt"

	"go.temporal.io/sdk/client"
)

// Config holds Temporal connection settings.
type Config struct {
	HostPort  string
	Namespace string
	TaskQueue string
}

// NewClient creates a new Temporal client with the given configuration.
func NewClient(cfg *Config) (client.Client, error) {
	c, err := client.Dial(client.Options{
		HostPort:  cfg.HostPort,
		Namespace: cfg.Namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create temporal client: %w", err)
	}
	return c, nil
}
