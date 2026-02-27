package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/dogbertdev/plexcli/internal/auth"
	"github.com/dogbertdev/plexcli/internal/cache"
	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/plexclient"
)

// ClientContext holds the authenticated client and context for command execution.
type ClientContext struct {
	Client  *plexclient.Client
	Ctx     context.Context
	Cancel  context.CancelFunc
	Timeout time.Duration
}

// NewClientContext validates config, authenticates, and creates a Plex client.
// Caller must call ctx.Cancel() when done.
func NewClientContext(cfg *config.Config) (*ClientContext, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}

	authCtx, authCancel := context.WithTimeout(context.Background(), auth.DefaultTimeout)
	defer authCancel()

	token, err := auth.GetToken(authCtx, *cfg)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	timeout := time.Duration(cfg.Timeout) * time.Second
	if cfg.Timeout == 0 {
		timeout = plexclient.DefaultTimeout
	}

	opts := []plexclient.ClientOption{
		plexclient.WithTimeout(timeout),
	}

	if !cfg.CacheDisabled {
		cacheStore, cacheErr := cache.NewDefaultLibraryPayloadCache()
		if cacheErr == nil {
			ttl := time.Duration(cfg.CacheTTL) * time.Second
			if ttl <= 0 {
				ttl = plexclient.DefaultLibraryCacheTTL
			}
			opts = append(opts,
				plexclient.WithLibraryCache(cacheStore),
				plexclient.WithLibraryCacheTTL(ttl),
				plexclient.WithLibraryCacheRefresh(cfg.CacheRefresh),
			)
		}
	}

	client, err := plexclient.NewClient(cfg.ServerURL, token, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create plex client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	return &ClientContext{
		Client:  client,
		Ctx:     ctx,
		Cancel:  cancel,
		Timeout: timeout,
	}, nil
}
