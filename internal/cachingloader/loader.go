package cachingloader

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jwilner/jsonschema2go/pkg/gen"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
)

// New returns a new thread safe loader which caches requests and can handle either file system or http URIs. If the
// debug bool flag is set true, messages will be logged concerning every served request.
func New(debug bool) gen.Loader {
	c := &cachingLoader{
		requests: make(chan schemaRequest),
		client:   http.DefaultClient,
		closeC:   make(chan chan<- error),
	}
	go c.run(debug)
	return c
}

type cachingLoader struct {
	requests chan schemaRequest
	client   httpDoer
	closeC   chan chan<- error
}

// Load returns a schema for the provided URL, either filesystem or HTTP
func (c *cachingLoader) Load(ctx context.Context, u *url.URL) (*gen.Schema, error) {
	req := make(chan schemaResult)
	select {
	case c.requests <- schemaRequest{u.String(), req}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case res := <-req:
		return res.schema, res.error
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close closes the loader and waits for it to stop.
func (c *cachingLoader) Close() error {
	errC := make(chan error)
	c.closeC <- errC
	return <-errC
}

func (c *cachingLoader) run(debug bool) {
	type uriSchemaResult struct {
		schemaResult
		url string
	}

	ctx, cncl := context.WithCancel(context.Background())

	respond := func(wg *sync.WaitGroup, res schemaResult, c chan<- schemaResult) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case c <- res:
			case <-ctx.Done():
				close(c) // signal we're outta here to downstream
				return
			}
		}()
	}

	fetch := func(wg *sync.WaitGroup, result chan<- uriSchemaResult, u string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := c.fetch(ctx, u)
			select {
			case result <- uriSchemaResult{res, u}:
			case <-ctx.Done():
			}
		}()
	}

	var (
		childRoutines = new(sync.WaitGroup)
		activeReqs    = make(map[string][]chan<- schemaResult)
		fetches       = make(chan uriSchemaResult)
		cache         = make(map[string]*gen.Schema)
	)

	for {
		select {
		case errC := <-c.closeC:
			cncl()
			childRoutines.Wait()
			errC <- nil
			return

		case req := <-c.requests:
			if s, ok := cache[req.url]; ok {
				if debug {
					log.Printf("loader: cache hit for %v", req.url)
				}
				respond(childRoutines, schemaResult{s, nil}, req.c)
				continue
			}

			activeReqs[req.url] = append(activeReqs[req.url], req.c)

			if len(activeReqs[req.url]) == 1 { // this is the first req, so start a fetch
				if debug {
					log.Printf("loader: initiating fetch for %v", req.url)
				}
				fetch(childRoutines, fetches, req.url)
				continue
			}

		case fet := <-fetches:
			reqs := activeReqs[fet.url]
			delete(activeReqs, fet.url)

			if debug {
				log.Printf("loader: serving %v for %d requests", fet.url, len(reqs))
			}

			for _, r := range reqs {
				respond(childRoutines, fet.schemaResult, r)
			}

			// we won't cache errors
			if fet.error == nil && fet.schema != nil {
				cache[fet.url] = fet.schema
			}
		}
	}
}

func (c *cachingLoader) fetch(ctx context.Context, rawURL string) schemaResult {
	u, err := url.Parse(rawURL)
	if err != nil {
		return schemaResult{nil, err}
	}

	// open IO
	var r io.ReadCloser
	switch u.Scheme {
	case "file":
		var err error
		if r, err = os.Open(u.Path); err != nil {
			return schemaResult{nil, fmt.Errorf("unable to open %q from %q: %w", u.Path, rawURL, err)}
		}
	case "http", "https":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return schemaResult{nil, fmt.Errorf("unable to create request for %q: %w", u, err)}
		}
		resp, err := c.client.Do(req)
		if err != nil {
			return schemaResult{nil, fmt.Errorf("failed requesting %q: %w", u, err)}
		}
		r = resp.Body
	default:
		return schemaResult{nil, fmt.Errorf("unsupported scheme: %v", u.Scheme)}
	}
	defer func() {
		_ = r.Close()
	}()

	var s gen.Schema
	if err := json.NewDecoder(r).Decode(&s); err != nil {
		return schemaResult{nil, fmt.Errorf("unable to decode %q: %w", u.Path, err)}
	}
	s.SetLoc(u)
	return schemaResult{&s, nil}
}

type httpDoer interface {
	Do(r *http.Request) (*http.Response, error)
}

var _ httpDoer = http.DefaultClient

type schemaResult struct {
	schema *gen.Schema
	error  error
}

type schemaRequest struct {
	url string
	c   chan<- schemaResult
}
