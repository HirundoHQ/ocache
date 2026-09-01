package ocache

import (
	"fmt"
	"io"
	"time"

	"github.com/hirundohq/logging"
	"github.com/olric-data/olric"
	"github.com/olric-data/olric/config"
)

type Option func(*Client)

type Client struct {
	c   *olric.ClusterClient
	log logging.Logger
}

func New(address string, opts ...Option) (*Client, error) {
	client := &Client{
		log: logging.New(io.Discard, 0),
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
