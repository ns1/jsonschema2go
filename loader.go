package jsonschema2go

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

type HTTPDoer interface {
	Do(r *http.Request) (*http.Response, error)
}

var _ HTTPDoer = http.DefaultClient

func newCachingLoader() *CachingLoader {
	return &CachingLoader{make(map[string]*Schema), http.DefaultClient}
}

type CachingLoader struct {
	cache  map[string]*Schema
	client HTTPDoer
}

func (c *CachingLoader) Load(ctx context.Context, u *url.URL) (*Schema, error) {
	// cache check
	schema, ok := c.cache[u.String()]
	if ok {
		return schema, nil
	}
	defer func() {
		if schema != nil {
			c.cache[u.String()] = schema
		}
	}()

	// open IO
	var r io.ReadCloser
	switch u.Scheme {
	case "file":
		var err error
		if r, err = os.Open(u.Path); err != nil {
			return nil, fmt.Errorf("unable to open %q: %w", u.Path, err)
		}
	case "http", "https":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("unable to create request for %q: %w", u, err)
		}
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed requesting %q: %w", u, err)
		}
		r = resp.Body
	default:
		return nil, fmt.Errorf("unsupported scheme: %v", u.Scheme)
	}
	defer func() {
		_ = r.Close()
	}()

	var sch Schema
	if err := json.NewDecoder(r).Decode(&sch); err != nil {
		return nil, fmt.Errorf("unable to decode %q: %w", u.Path, err)
	}
	schema = &sch
	schema.curLoc = u

	return schema, nil
}
