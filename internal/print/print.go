package print

import (
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/planning"
	"github.com/jwilner/jsonschema2go/pkg/gen"
	"log"
	"os"
	"path/filepath"
	"sync"
)

func Print(
	ctx context.Context,
	printer Printer,
	grouped map[string][]gen.Plan,
	prefixes [][2]string,
) error {
	var childRoutines sync.WaitGroup
	defer childRoutines.Wait()

	mapper := planning.PrefixMapper(prefixes)
	done := make(chan struct{})
	errs := make(chan error, 1)
	for k, group := range grouped {
		childRoutines.Add(1)
		go func(k string, group []gen.Plan) {
			defer childRoutines.Done()
			if err := func() error {
				path := mapper(k)
				if path == "" {
					return fmt.Errorf("unable to map go path: %q", k)
				}
				if err := os.MkdirAll(path, 0755); err != nil {
					return fmt.Errorf("unable to create dir %q: %w", path, err)
				}

				p := filepath.Join(path, "values.gen.go")

				f, err := os.Create(p)
				if err != nil {
					return fmt.Errorf("unable to open: %w", err)
				}
				defer f.Close()

				if err := printer.Print(ctx, f, k, group); err != nil {
					return fmt.Errorf("unable to print: %w", err)
				}
				if gen.IsDebug(ctx) {
					log.Printf("printer: successfully printed %d plans to %v", len(group), p)
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
