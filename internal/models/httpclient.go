// internal/models/httpclient.go
package models

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Common HTTP errors that should trigger retry
var (
	ErrRateLimit  = errors.New("rate limit exceeded (429)")
	ErrServerBusy = errors.New("server busy (503)")
	ErrBadGateway = errors.New("bad gateway (502)")
	ErrGatewayTimeout = errors.New("gateway timeout (504)")
)

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// DefaultRetryConfig returns sensible defaults
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   time.Second,
		MaxDelay:    10 * time.Second,
	}
}

// RetryableClient wraps http.Client with retry logic
type RetryableClient struct {
	client *http.Client
	config RetryConfig
}

// NewRetryableClient creates a client with retry support
func NewRetryableClient(config RetryConfig) *RetryableClient {
	return &RetryableClient{
		client: &http.Client{
			Timeout: 120 * time.Second, // Long timeout for streaming
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				IdleConnTimeout:       90 * time.Second,
				MaxIdleConns:          10,
				MaxIdleConnsPerHost:   5,
			},
		},
		config: config,
	}
}

// DoWithRetry executes a request with retry logic for transient errors
func (c *RetryableClient) DoWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var lastErr error
	delay := c.config.BaseDelay

	for attempt := 0; attempt < c.config.MaxAttempts; attempt++ {
		// Clone request for retry (body already read on first attempt)
		reqClone := req.Clone(ctx)

		resp, err := c.client.Do(reqClone)
		if err != nil {
			// Check if it's a connection error worth retrying
			if isRetryableError(err) {
				lastErr = err
				if attempt < c.config.MaxAttempts-1 {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(delay):
						delay = min(delay*2, c.config.MaxDelay)
						continue
					}
				}
				continue
			}
			return nil, err
		}

		// Check for retryable status codes
		if shouldRetryStatus(resp.StatusCode) {
			resp.Body.Close()
			lastErr = statusError(resp.StatusCode)
			if attempt < c.config.MaxAttempts-1 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
					delay = min(delay*2, c.config.MaxDelay)
					continue
				}
			}
			continue
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("after %d attempts: %w", c.config.MaxAttempts, lastErr)
	}
	return nil, fmt.Errorf("request failed after %d attempts", c.config.MaxAttempts)
}

// isRetryableError checks if a network error is worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Context errors - don't retry
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Network errors - retry
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Temporary() || netErr.Timeout()
	}

	// DNS errors - retry
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.Temporary()
	}

	return false
}

// shouldRetryStatus checks if an HTTP status code warrants a retry
func shouldRetryStatus(code int) bool {
	switch code {
	case 429, 502, 503, 504:
		return true
	default:
		return false
	}
}

// statusError returns a descriptive error for HTTP status
func statusError(code int) error {
	switch code {
	case 429:
		return ErrRateLimit
	case 502:
		return ErrBadGateway
	case 503:
		return ErrServerBusy
	case 504:
		return ErrGatewayTimeout
	default:
		return fmt.Errorf("HTTP %d", code)
	}
}

// NewRequestWithBody creates a new HTTP request with the given body bytes
// The body is stored so it can be re-read on retry
func NewRequestWithBody(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	// Store body for potential retries using GetBody
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.Body, _ = req.GetBody()
	req.ContentLength = int64(len(body))
	return req, nil
}
