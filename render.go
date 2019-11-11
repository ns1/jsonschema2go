package jsonschema2go

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Option func(s *settings)

type settings struct {
	prefixes [][2]string
	typer    Typer
	planner  Planner
	printer  *Printer
	loader   Loader
}

func PrefixMap(pairs ...string) Option {
	if len(pairs)%2 != 0 {
		panic("must be even list of prefixes")
	}
	var prefixes [][2]string
	for i := 0; i < len(pairs); i += 2 {
		prefixes = append(prefixes, [2]string{pairs[i], pairs[i+1]})
	}
	return func(s *settings) {
		s.prefixes = prefixes
	}
}

func CustomTyper(typer Typer) Option {
	return func(s *settings) {
		s.typer = typer
	}
}

func CustomPlanners(planners ...Planner) Option {
	return func(s *settings) {
		s.planner = CompositePlanner(planners)
	}
}

func Render(ctx context.Context, fileNames []string, options ...Option) error {

	s := &settings{
		typer:   defaultTypeInfoer{},
		planner: Composite,
		printer: new(Printer),
	}
	for _, o := range options {
		o(s)
	}

	if s.loader == nil {
		c := newCachingLoader()
		defer func() {
			_ = c.Close()
		}()
		s.loader = c
	}
	ctx, cncl := context.WithCancel(ctx)
	defer cncl()

	grouped, err := doCrawl(ctx, s.planner, s.loader, s.typer, fileNames)
	if err != nil {
		return err
	}

	return print(ctx, s.printer, grouped, s.prefixes)
}

func print(
	ctx context.Context,
	printer *Printer,
	grouped map[string][]Plan,
	prefixes [][2]string,
) error {
	var childRoutines sync.WaitGroup
	defer childRoutines.Wait()

	mapper := pathMapper(prefixes)
	done := make(chan struct{})
	errs := make(chan error, 1)
	for k, group := range grouped {
		childRoutines.Add(1)
		go func(k string, group []Plan) {
			defer childRoutines.Done()
			if err := func() error {
				path := mapper(k)
				if path == "" {
					return fmt.Errorf("unable to map go path: %q", k[0])
				}
				if err := os.MkdirAll(path, 0755); err != nil {
					return fmt.Errorf("unable to create dir %q: %w", path, err)
				}

				f, err := os.Create(filepath.Join(path, "values.gen.go"))
				if err != nil {
					return fmt.Errorf("unable to open: %w", err)
				}
				defer f.Close()

				if err := printer.Print(ctx, f, k, group); err != nil {
					return fmt.Errorf("unable to print: %w", err)
				}
				return nil
			}(); err != nil {
				select {
				case errs <- err:
				default:
				}
				return
			}

			select {
			case <-ctx.Done():
			case done <- struct{}{}:
			}
		}(k, group)
	}

	for range grouped { // we know there are len(grouped) routines to wait for
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errs:
			return err
		case <-done:
		}
	}
	return nil
}

func group(ctx context.Context, results <-chan CrawlResult, errC <-chan error) (map[string][]Plan, error) {
	// group together results
	grouped := make(map[string][]Plan)
	for {
		select {
		case err := <-errC:
			return nil, err
		case result, ok := <-results:
			if !ok {
				return grouped, nil
			}
			if result.Err != nil {
				return nil, result.Err
			}
			plan := result.Plan
			goPath := plan.Type().GoPath
			grouped[goPath] = append(grouped[goPath], plan)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func pathMapper(prefixes [][2]string) func(string) string {
	sort.Slice(prefixes, func(i, j int) bool {
		return prefixes[i][0] < prefixes[j][0]
	})
	return func(path string) string {
		i := sort.Search(len(prefixes), func(i int) bool {
			return prefixes[i][0] > path
		})
		for i = i - 1; i >= 0; i-- {
			if strings.HasPrefix(path, prefixes[i][0]) {
				return prefixes[i][1] + path[len(prefixes[i][0]):]
			}
		}
		return path
	}
}
