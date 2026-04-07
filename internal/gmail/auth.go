package gmail

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/axqd/mbox-reporter/internal/cache"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// NewService creates an authenticated Gmail API service.
// The token is read from and saved to the provided cache.
func NewService(ctx context.Context, clientSecretPath string, c *cache.Cache) (*gmail.Service, error) {
	b, err := os.ReadFile(clientSecretPath)
	if err != nil {
		return nil, fmt.Errorf("read client secret file: %w", err)
	}

	config, err := google.ConfigFromJSON(b, gmail.GmailModifyScope)
	if err != nil {
		return nil, fmt.Errorf("parse client secret: %w", err)
	}

	tok, err := loadOrObtainToken(ctx, config, c)
	if err != nil {
		return nil, err
	}

	return gmail.NewService(ctx, option.WithTokenSource(config.TokenSource(ctx, tok)))
}

func loadOrObtainToken(ctx context.Context, config *oauth2.Config, c *cache.Cache) (*oauth2.Token, error) {
	if c.OAuthToken != nil {
		return c.OAuthToken, nil
	}

	tok, err := obtainToken(ctx, config)
	if err != nil {
		return nil, err
	}

	c.OAuthToken = tok
	if err := c.Save(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: could not cache token: %v\n", err)
	}

	return tok, nil
}

func obtainToken(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	// Start a temporary local server on a random port to receive the OAuth callback.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for OAuth callback: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	config.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code parameter", http.StatusBadRequest)
			return
		}
		_, _ = fmt.Fprintln(w, "Authorization received. You can close this tab.")
		codeCh <- code
	})
	srv := &http.Server{Handler: mux}

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Open this URL in your browser and authorize the application:\n\n%s\n\nWaiting for authorization...\n", authURL)

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, fmt.Errorf("OAuth callback server: %w", err)
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	_ = srv.Shutdown(ctx)

	tok, err := config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange authorization code: %w", err)
	}

	return tok, nil
}

