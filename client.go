package ocache

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/hirundohq/logging"
	"github.com/olric-data/olric"
	"github.com/olric-data/olric/config"
	"github.com/olric-data/olric/stats"
)

type Option func(*Client)

type Client struct {
	c    *olric.ClusterClient
	addr string
	log  logging.Logger
}

func New(address string, opts ...Option) (*Client, error) {
	client := &Client{
		addr: address,
		log:  logging.New(io.Discard, 0),
	}

	for _, opt := range opts {
		opt(client)
	}

	if client.c == nil {
		c, err := newClient(address)
		if err != nil {
			return nil, fmt.Errorf("new returned an error: %w", err)
		}
		client.c = c
	}

	return client, nil
}

func newClient(address string) (*olric.ClusterClient, error) {
	cfg := config.NewClient()

	if err := cfg.Sanitize(); err != nil {
		return nil, fmt.Errorf("sanitize returned an error: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate returned an error: %w", err)
	}

	cfg.DialTimeout = 1 * time.Second

	c, err := olric.NewClusterClient([]string{address}, olric.WithConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("olric.NewClusterClient returned an error: %w", err)
	}

	return c, nil
}

// WithLogger sets the logger for the client.
func WithLogger(log logging.Logger) Option {
	return func(c *Client) {
		c.log = log.Register("olric")
	}
}

// WithClusterClient sets the cluster client for the client.
func WithClusterClient(clusterClient *olric.ClusterClient) Option {
	return func(c *Client) {
		c.c = clusterClient
	}
}

// Stats returns the statistics of the node the client was given: per-partition
// DMap sizes, eviction and hit counters, runtime figures. Olric has no exporter
// of its own, so this is the only way to watch a DMap against its maxInuse.
// DMap figures are per partition; sum them for the DMap total.
func (c *Client) Stats(ctx context.Context) (stats.Stats, error) {
	st, err := c.c.Stats(ctx, c.addr)
	if err != nil {
		return stats.Stats{}, fmt.Errorf("olric.Stats: %w", err)
	}
	return st, nil
}
