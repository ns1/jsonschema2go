package gen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

// Loader is the contract required to be able to resolve a schema.
type Loader interface {
	io.Closer
	// Read returns a schema for a URL
	Load(ctx context.Context, u *url.URL) (*Schema, error)
}

// NewLoader returns a basic loader which can handle either file system or HTTP(s) requests
func NewLoader() Loader {
	return &baseLoader{client: http.DefaultClient}
}

type baseLoader struct {
	client *http.Client
}

// Load loads the requested resource from the provided URL (must be file, http, or https), times out, or errors
func (b *baseLoader) Load(ctx context.Context, src *url.URL) (*Schema, error) {
	// open IO
	var r io.ReadCloser
	switch src.Scheme {
	case "file":
		var err error
		if r, err = os.Open(src.Path); err != nil {
			return nil, fmt.Errorf("unable to open %q from %q: %w", src.Path, src, err)
		}
	case "http", "https":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("unable to create request for %q: %w", src, err)
		}
		resp, err := b.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed requesting %q: %w", src, err)
		}
		r = resp.Body
	default:
		return nil, fmt.Errorf("unsupported scheme: %v", src.Scheme)
	}
	defer func() {
		_ = r.Close()
	}()

	// read and init schema
	var s Schema
	if err := json.NewDecoder(r).Decode(&s); err != nil {
		return nil, fmt.Errorf("decoding %q failed: %w", src, err)
	}
	if s.ID == nil {
		return nil, fmt.Errorf("no ID set on %q", src)
	}
	s.calculateID()
	s.setSrc(src)

	return &s, nil
}

// Close closes any associated resources
func (b *baseLoader) Close() error {
	return nil
}
