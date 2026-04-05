// Package errors defines structured error types for the llm package.
//
// All error types are compatible with [errors.Is] and [errors.As].
// Use [IsFallbackable] to determine whether an error should trigger
// fallback to the next provider.
package errors

import (
	"errors"
	"fmt"
	"net"
	"time"
)

// ErrProviderNotFound indicates the URI scheme is not registered.
type ErrProviderNotFound struct {
	Scheme string
}

func (e *ErrProviderNotFound) Error() string {
	return fmt.Sprintf("provider not found: scheme %q", e.Scheme)
}

func (e *ErrProviderNotFound) Unwrap() error { return nil }

// NewProviderNotFound creates an [ErrProviderNotFound].
func NewProviderNotFound(scheme string) error {
	return &ErrProviderNotFound{Scheme: scheme}
}

// ErrCapabilityNotSupported indicates the adapter does not implement
// the requested interface.
type ErrCapabilityNotSupported struct {
	Capability string
	Provider   string
}

func (e *ErrCapabilityNotSupported) Error() string {
	return fmt.Sprintf("capability %q not supported by provider %q",
		e.Capability, e.Provider)
}

func (e *ErrCapabilityNotSupported) Unwrap() error { return nil }

// NewCapabilityNotSupported creates an [ErrCapabilityNotSupported].
func NewCapabilityNotSupported(capability, provider string) error {
	return &ErrCapabilityNotSupported{
		Capability: capability,
		Provider:   provider,
	}
}

// ErrAuth indicates an authentication/authorization failure (401/403).
type ErrAuth struct {
	Provider string
	Err      error
}

func (e *ErrAuth) Error() string {
	return fmt.Sprintf("auth error (provider %q): %v", e.Provider, e.Err)
}

func (e *ErrAuth) Unwrap() error { return e.Err }

// NewAuth creates an [ErrAuth] wrapping the underlying cause.
func NewAuth(provider string, err error) error {
	return &ErrAuth{Provider: provider, Err: err}
}

// ErrRateLimit indicates a 429 response.
type ErrRateLimit struct {
	Provider   string
	RetryAfter time.Duration
}

func (e *ErrRateLimit) Error() string {
	if e.RetryAfter == 0 {
		return fmt.Sprintf("rate limited (provider %q)", e.Provider)
	}
	return fmt.Sprintf("rate limited (provider %q, retry after %s)",
		e.Provider, e.RetryAfter)
}

func (e *ErrRateLimit) Unwrap() error { return nil }

// NewRateLimit creates an [ErrRateLimit].
func NewRateLimit(provider string, retryAfter time.Duration) error {
	return &ErrRateLimit{Provider: provider, RetryAfter: retryAfter}
}

// ErrContext indicates a context cancellation or deadline exceeded.
type ErrContext struct {
	Err error
}

func (e *ErrContext) Error() string {
	return fmt.Sprintf("context error: %v", e.Err)
}

func (e *ErrContext) Unwrap() error { return e.Err }

// NewContext creates an [ErrContext] wrapping the underlying cause.
func NewContext(err error) error {
	return &ErrContext{Err: err}
}

// ErrModel indicates the model is not found or not available.
type ErrModel struct {
	Model    string
	Provider string
}

func (e *ErrModel) Error() string {
	return fmt.Sprintf("model %q not available (provider %q)",
		e.Model, e.Provider)
}

func (e *ErrModel) Unwrap() error { return nil }

// NewModel creates an [ErrModel].
func NewModel(model, provider string) error {
	return &ErrModel{Model: model, Provider: provider}
}

// ErrFallbackExhausted indicates all providers in the fallback chain
// have failed. The Errors field holds each provider's error.
type ErrFallbackExhausted struct {
	Errors []error
}

func (e *ErrFallbackExhausted) Error() string {
	return fmt.Sprintf("all providers failed (%d errors)", len(e.Errors))
}

// Unwrap returns nil; individual errors are accessible via Errors
// field through [errors.As].
func (e *ErrFallbackExhausted) Unwrap() error { return nil }

// NewFallbackExhausted creates an [ErrFallbackExhausted].
func NewFallbackExhausted(errs []error) error {
	return &ErrFallbackExhausted{Errors: errs}
}

// HTTPStatusError represents an HTTP error response. Used to classify
// 5xx errors as fallbackable.
type HTTPStatusError struct {
	StatusCode int
	Status     string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("http %d: %s", e.StatusCode, e.Status)
}

// NewHTTPStatusError creates an [HTTPStatusError].
func NewHTTPStatusError(code int, status string) error {
	return &HTTPStatusError{StatusCode: code, Status: status}
}

// IsFallbackable returns true when the error should trigger fallback
// to the next provider. Network errors, rate limits, and 5xx responses
// are fallbackable. Auth errors, model errors, capability errors, and
// context errors are not.
func IsFallbackable(err error) bool {
	if err == nil {
		return false
	}

	// Rate limit — fallbackable.
	var rl *ErrRateLimit
	if errors.As(err, &rl) {
		return true
	}

	// Network error — fallbackable.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	// 5xx HTTP status — fallbackable.
	var httpErr *HTTPStatusError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500
	}

	// Non-fallbackable error types.
	var authErr *ErrAuth
	if errors.As(err, &authErr) {
		return false
	}
	var modelErr *ErrModel
	if errors.As(err, &modelErr) {
		return false
	}
	var capErr *ErrCapabilityNotSupported
	if errors.As(err, &capErr) {
		return false
	}
	var ctxErr *ErrContext
	if errors.As(err, &ctxErr) {
		return false
	}

	return false
}
