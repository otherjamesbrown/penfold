// Package backend provides backend connectors for AI services.
package backend

import "errors"

// Backend errors.
var (
	// ErrEmptyText indicates an empty string was provided for embedding.
	ErrEmptyText = errors.New("backend: text cannot be empty")

	// ErrEmptyMessages indicates no messages were provided for completion.
	ErrEmptyMessages = errors.New("backend: messages cannot be empty")

	// ErrServiceUnavailable indicates the service cannot be reached.
	ErrServiceUnavailable = errors.New("backend: service unavailable")

	// ErrRequestFailed indicates the request failed.
	ErrRequestFailed = errors.New("backend: request failed")

	// ErrInvalidResponse indicates the server returned an invalid response.
	ErrInvalidResponse = errors.New("backend: invalid server response")

	// ErrContextCanceled indicates the context was canceled during the request.
	ErrContextCanceled = errors.New("backend: context canceled")

	// ErrRequestTimeout indicates the request timed out.
	ErrRequestTimeout = errors.New("backend: request timeout")

	// ErrNoChoices indicates the LLM returned no completion choices.
	ErrNoChoices = errors.New("backend: no completion choices returned")

	// ErrJSONParseFailed indicates JSON parsing failed.
	ErrJSONParseFailed = errors.New("backend: JSON parse failed")
)
