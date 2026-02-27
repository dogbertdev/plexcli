package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/LukeHagar/plexgo/models/components"

	"github.com/dogbertdev/plexcli/internal/auth"
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

	client, err := plexclient.NewClient(cfg.ServerURL, token, plexclient.WithTimeout(timeout))
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

func fetchLibraryItems(ctx context.Context, client *plexclient.Client, sectionID string) ([]*components.Metadata, error) {
	if sectionID != "" {
		return client.GetAllLibraryItems(ctx, sectionID)
	}

	sections, err := client.GetSections(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sections: %w", err)
	}

	var allItems []*components.Metadata
	for _, section := range sections {
		items, err := client.GetAllLibraryItems(ctx, section.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get items from section %s: %w", section.ID, err)
		}
		allItems = append(allItems, items...)
	}

	return allItems, nil
}

func forEachStream(item *components.Metadata, visitor func(media *components.Media, stream *components.Stream)) {
	for mediaIndex := range item.Media {
		media := &item.Media[mediaIndex]
		for partIndex := range media.Part {
			part := &media.Part[partIndex]
			for streamIndex := range part.Stream {
				stream := &part.Stream[streamIndex]
				visitor(media, stream)
			}
		}
	}
}
