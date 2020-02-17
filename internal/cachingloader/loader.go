package cachingloader

import (
	"context"
	"github.com/jwilner/jsonschema2go/pkg/gen"
	"log"
	"net/url"
	"sync"
)

// New returns a new thread safe loader which caches requests and can handle either file system or http URIs. If the
// debug bool flag is set true, messages will be logged concerning every served request.
func New(debug bool) gen.Loader {
	c := &cachingLoader{
		requests: make(chan schemaRequest),
		closeC:   make(chan chan<- error),
		delegate: gen.NewLoader(),
	}
	go c.run(debug)
	return c
}

type cachingLoader struct {
	requests chan schemaRequest
	closeC   chan chan<- error
	delegate gen.Loader
}

// Read returns a schema for the provided URL, either filesystem or HTTP
func (c *cachingLoader) Load(ctx context.Context, u *url.URL) (*gen.Schema, error) {
	req := make(chan uriSchemaResult)
	select {
	case c.requests <- schemaRequest{u, req}:
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
	err := <-errC
	if dErr := c.delegate.Close(); dErr != nil && err == nil {
		err = dErr
	}
	return err
}

func (c *cachingLoader) run(debug bool) {
	ctx, cncl := context.WithCancel(context.Background())

	respond := func(wg *sync.WaitGroup, res uriSchemaResult, c chan<- uriSchemaResult) {
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

	fetch := func(wg *sync.WaitGroup, result chan<- uriSchemaResult, u *url.URL) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := c.fetch(ctx, u)
			select {
			case result <- uriSchemaResult{s, err, u}:
			case <-ctx.Done():
			}
		}()
	}

	var (
		childRoutines = new(sync.WaitGroup)
		activeReqs    = make(map[string][]chan<- uriSchemaResult)
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
			key := req.url.String()
			if s, ok := cache[key]; ok {
				if debug {
					log.Printf("loader: cache hit for %v", req.url)
				}
				respond(childRoutines, uriSchemaResult{s, nil, req.url}, req.c)
				continue
			}

			activeReqs[key] = append(activeReqs[key], req.c)

			if len(activeReqs[key]) == 1 { // this is the first req, so start a fetch
				if debug {
					log.Printf("loader: initiating fetch for %v", req.url)
				}
				fetch(childRoutines, fetches, req.url)
				continue
			}

		case fet := <-fetches:
			key := fet.url.String()

			reqs := activeReqs[key]
			delete(activeReqs, key)

			if debug {
				log.Printf("loader: serving %v for %d requests", fet.url, len(reqs))
			}

			for _, r := range reqs {
				respond(childRoutines, fet, r)
			}

			// we won't cache errors
			if fet.error == nil && fet.schema != nil {
				cache[fet.url.String()] = fet.schema
			}
		}
	}
}

func (c *cachingLoader) fetch(ctx context.Context, u *url.URL) (*gen.Schema, error) {
	return c.delegate.Load(ctx, u)
}

type uriSchemaResult struct {
	schema *gen.Schema
	error  error
	url    *url.URL
}

type schemaRequest struct {
	url *url.URL
	c   chan<- uriSchemaResult
}
