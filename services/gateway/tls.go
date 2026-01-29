// Package main provides TLS configuration helpers for the API Gateway service.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/otherjamesbrown/penfold/services/gateway/config"
)

// LoadTLSConfig creates a tls.Config from the gateway TLS configuration.
// Returns nil if TLS is not enabled.
func LoadTLSConfig(cfg *config.TLSConfig) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	// Load server certificate and key pair.
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server cert: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		ClientAuth:   cfg.ClientAuthType(),
	}

	// Load CA certificate for client verification if client auth is enabled.
	if cfg.ClientAuth != "none" && cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA cert: %w", err)
		}

		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("parse CA cert: invalid PEM")
		}

		tlsConfig.ClientCAs = caPool
	}

	return tlsConfig, nil
}
