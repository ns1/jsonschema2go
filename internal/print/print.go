package print

import (
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/pkg/gen"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
)

func TypeFromId(pairs [][2]string) func(string) (string, string) {
	mapper := PathMapper(pairs)
	return func(s string) (string, string) {
		s = mapper(s)
		u, err := url.Parse(s)
		if err != nil {
			return "", ""
		}
		pathParts := strings.Split(u.Host+u.Path, "/")
		if len(pathParts) < 2 {
			return "", ""
		}
		// drop the extension
		nameParts := strings.SplitN(pathParts[len(pathParts)-1], ".", 2)
		if len(nameParts) == 0 {
			return "", ""
		}
		path, name := strings.Join(pathParts[:len(pathParts)-1], "/"), nameParts[0]
		// add any fragment info
		if u.Fragment != "" {
			frags := strings.Split(u.Fragment, "/")
			for _, frag := range frags {
				if frag == "" || frag == "properties" {
					continue
				}
				runes := []rune(frag)
				runes[0] = unicode.ToUpper(runes[0])
				name += string(runes)
			}
		}
		return path, name
	}
}

func Print(
	ctx context.Context,
	printer Printer,
	grouped map[string][]gen.Plan,
	prefixes [][2]string,
) error {
	var childRoutines sync.WaitGroup
	defer childRoutines.Wait()

	mapper := PathMapper(prefixes)
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

func PathMapper(prefixes [][2]string) func(string) string {
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
