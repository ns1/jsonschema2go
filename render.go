package jsonschema2go

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

func NewRenderer() *Renderer {
	return &Renderer{
		crawler: newCrawler(),
		printer: newPrinter(),
	}
}

type Renderer struct {
	crawler *Crawler
	printer *Printer
}

func (r *Renderer) Render(ctx context.Context, fileNames []string, prefixes [][2]string) error {
	var childRoutines sync.WaitGroup
	defer childRoutines.Wait()

	ctx, cncl := context.WithCancel(ctx)
	defer cncl()

	loader := newLoader()
	childRoutines.Add(1)
	go func() {
		defer childRoutines.Done()
		_ = loader.Run(ctx) // run the background cache processes
	}()

	errs := make(chan error, 1)
	sendErr := func(err error) {
		// don't block if one is already sent on buffered chan
		select {
		case errs <- err:
		default:
		}
	}

	// load all initial schemas concurrently
	loaded := make(chan *Schema)
	{
		sent := int64(0) // int64 used to track completion of tasks
		for _, fileName := range fileNames {
			childRoutines.Add(1)
			go func(fileName string) {
				defer childRoutines.Done()

				u, err := url.Parse(fileName)
				if err != nil {
					sendErr(err)
					return
				}

				schema, err := loader.Load(ctx, u)
				if err != nil {
					sendErr(fmt.Errorf("unable to resolve schema from %q: %w", fileName, err))
					return
				}
				select {
				case <-ctx.Done():
				case loaded <- schema:
					if atomic.AddInt64(&sent, 1) == int64(len(fileNames)) {
						close(loaded)
					}
				}
			}(fileName)
		}
	}

	results := r.crawler.Plan(ctx, loader, loaded)

	// group together results
	grouped := make(map[string][]Plan)
outer:
	for {
		select {
		case err := <-errs:
			return err
		case result, ok := <-results:
			if !ok {
				break outer
			}
			if result.Err != nil {
				return result.Err
			}
			plan := result.Plan
			goPath := plan.Type().GoPath
			grouped[goPath] = append(grouped[goPath], plan)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	mapper := pathMapper(prefixes)
	done := make(chan struct{})
	for k, group := range grouped {
		childRoutines.Add(1)
		go func(k string, group []Plan) {
			defer childRoutines.Done()

			path := mapper(k)
			if path == "" {
				sendErr(fmt.Errorf("unable to map go path: %q", k[0]))
			}
			if err := os.MkdirAll(path, 0755); err != nil {
				sendErr(fmt.Errorf("unable to create dir %q: %w", path, err))
			}
			if err := func() error {
				f, err := os.Create(filepath.Join(path, "values.gen.go"))
				if err != nil {
					return fmt.Errorf("unable to open: %w", err)
				}
				defer f.Close()

				if err := r.printer.Print(ctx, f, k, group); err != nil {
					return fmt.Errorf("unable to print: %w", err)
				}
				return nil
			}(); err != nil {
				sendErr(err)
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
