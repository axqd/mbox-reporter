package gmail

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
)

const maxRetries = 5

// ErrNotFound indicates the requested thread was not found (already deleted).
var ErrNotFound = errors.New("thread not found")

// ErrInvalidGrant indicates the OAuth token has been expired or revoked.
var ErrInvalidGrant = errors.New("token expired or revoked")

// Client wraps a Gmail service with rate limiting and retry logic.
type Client struct {
	svc          *gmail.Service
	limiter      *rate.Limiter
	backoffCount atomic.Int64
}

// NewClient creates a rate-limited Gmail client.
// ratePerSec is the max API calls per second (e.g., 25 for threads.trash at 10 units each).
func NewClient(svc *gmail.Service, ratePerSec int) *Client {
	return &Client{
		svc:     svc,
		limiter: rate.NewLimiter(rate.Limit(ratePerSec), ratePerSec),
	}
}

// TrashThread moves a single thread to trash with rate limiting and retry.
func (c *Client) TrashThread(ctx context.Context, threadID string) error {
	return c.withRetry(ctx, func() error {
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}
		_, err := c.svc.Users.Threads.Trash("me", threadID).Context(ctx).Do()
		if isNotFound(err) {
			return ErrNotFound
		}
		return err
	})
}

// BackoffCount returns the number of times exponential backoff was triggered.
func (c *Client) BackoffCount() int64 {
	return c.backoffCount.Load()
}

func (c *Client) withRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := range maxRetries {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if isInvalidGrantError(lastErr) {
			return ErrInvalidGrant
		}
		if errors.Is(lastErr, ErrNotFound) || !isRateLimitError(lastErr) {
			return lastErr
		}

		c.backoffCount.Add(1)
		backoff := backoffDuration(attempt)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

func isRateLimitError(err error) bool {
	if apiErr, ok := errors.AsType[*googleapi.Error](err); ok {
		return apiErr.Code == 429 || apiErr.Code == 403
	}
	return false
}

func isInvalidGrantError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid_grant")
}

func isNotFound(err error) bool {
	if apiErr, ok := errors.AsType[*googleapi.Error](err); ok {
		return apiErr.Code == 404
	}
	return false
}

func backoffDuration(attempt int) time.Duration {
	base := time.Duration(1<<uint(attempt)) * time.Second // 1s, 2s, 4s, 8s, 16s
	jitter := time.Duration(rand.Int64N(int64(base) / 2)) // 0-50% jitter
	return base + jitter
}
