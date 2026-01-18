package registry

import "errors"

// Registry errors.
var (
	// ErrModelNotFound is returned when a requested model is not in the registry.
	ErrModelNotFound = errors.New("model not found")

	// ErrModelAlreadyExists is returned when trying to register a duplicate model.
	ErrModelAlreadyExists = errors.New("model already exists")

	// ErrInvalidModelID is returned when a model ID is empty or invalid.
	ErrInvalidModelID = errors.New("invalid model ID")

	// ErrInvalidModelName is returned when a model name is empty or invalid.
	ErrInvalidModelName = errors.New("invalid model name")

	// ErrInvalidProvider is returned when a provider is empty or unknown.
	ErrInvalidProvider = errors.New("invalid provider")

	// ErrNoCapabilities is returned when a model has no declared capabilities.
	ErrNoCapabilities = errors.New("model must have at least one capability")

	// ErrRegistryClosed is returned when operations are attempted on a closed registry.
	ErrRegistryClosed = errors.New("registry is closed")

	// ErrDiscoveryFailed is returned when model discovery from a provider fails.
	ErrDiscoveryFailed = errors.New("model discovery failed")

	// ErrHealthCheckFailed is returned when a health check fails.
	ErrHealthCheckFailed = errors.New("health check failed")
)
