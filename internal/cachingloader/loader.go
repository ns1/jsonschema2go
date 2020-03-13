package cachingloader

import (
	"context"
	"github.com/ns1/jsonschema2go/pkg/gen"
	"log"
	"net/url"
	"sync"
)

// New returns a new thread safe loader which caches requests and can handle either file system or http URIs. If the
// debug bool flag is set true, messages will be logged concerning every served request.
func NewSimple() gen.Loader {
	return &loader{
		cache:  make(map[string]*gen.Schema),
		loader: gen.NewLoader(),
	}
}

type loader struct {
	cache map[string]*gen.Schema
	mu    sync.RWMutex

	loader gen.Loader
}

func (l *loader) Close() error {
	return nil
}

// Read returns a schema for the provided URL, either filesystem or HTTP
func (l *loader) Load(ctx context.Context, u *url.URL) (*gen.Schema, error) {
	k := u.String()

	l.mu.RLock()
	v := l.cache[k]
	l.mu.RUnlock()

	if v != nil {
		return v, nil
	}

	if gen.IsDebug(ctx) {
		log.Printf("cache miss -- requesting %v", u)
	}
	schema, err := l.loader.Load(ctx, u)
	if err != nil {
		return nil, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.cache[k]; !ok {
		l.cache[k] = schema
	}
	return l.cache[k], nil
}
