package jsonschema2go

import (
	"context"
	"github.com/jwilner/jsonschema2go/internal/cachingloader"
	"github.com/jwilner/jsonschema2go/internal/crawl"
	"github.com/jwilner/jsonschema2go/internal/planning"
	"github.com/jwilner/jsonschema2go/internal/print"
	"github.com/jwilner/jsonschema2go/pkg/ctxflags"
)

//go:generate go run internal/cmd/gentmpl/gentmpl.go

func Generate(ctx context.Context, fileNames []string, options ...Option) error {
	s := &settings{
		planner: planning.Composite,
		printer: print.New(nil),
		typer:   planning.DefaultTyper,
	}
	for _, o := range options {
		o(s)
	}

	if s.loader == nil {
		c := cachingloader.New(s.debug)
		defer func() {
			_ = c.Close()
		}()
		s.loader = c
	}
	ctx, cncl := context.WithCancel(ctx)
	defer cncl()

	if s.debug {
		ctx = ctxflags.SetDebug(ctx)
	}

	grouped, err := crawl.Crawl(ctx, s.planner, s.loader, s.typer, fileNames)
	if err != nil {
		return err
	}

	return print.Print(ctx, s.printer, grouped, s.prefixes)
}