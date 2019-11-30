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
	"text/template"
	"unicode"
)

type Option func(s *settings)

type settings struct {
	prefixes [][2]string
	typer    typer
	planner  Planner
	printer  *Printer
	loader   Loader
}

func PrefixMap(pairs ...string) Option {
	prefixes := prefixPairs(pairs)
	return func(s *settings) {
		s.prefixes = prefixes
	}
}

func CustomTypeFunc(typeFunc func(schema *Schema) TypeInfo) Option {
	return func(s *settings) {
		s.typer.typeFunc = typeFunc
	}
}

func CustomPrimitivesMap(primitivesMap map[SimpleType]string) Option {
	return func(s *settings) {
		s.typer.primitives = primitivesMap
	}
}

func CustomPlanners(planners ...Planner) Option {
	return func(s *settings) {
		s.planner = CompositePlanner(planners)
	}
}

func TypeFromID(pairs ...string) Option {
	mapper := typeFromID(prefixPairs(pairs))
	return func(s *settings) {
		s.typer.typeFunc = func(schema *Schema) TypeInfo {
			if t := defaultTypeFunc(schema); !t.Unknown() {
				return t
			}
			if schema.CalcID != nil {
				if path, name := mapper(schema.CalcID.String()); name != "" {
					return TypeInfo{GoPath: path, Name: name}
				}
			}
			return TypeInfo{}
		}
	}
}

func CustomTemplate(tmpl *template.Template) Option {
	return func(s *settings) {
		s.printer = newPrinter(tmpl)
	}
}

func CustomInitialisms(names ...string) Option {
	return func(s *settings) {
		s.typer.namer = newNamer(append(names, "id", "http"))
	}
}

func typeFromID(pairs [][2]string) func(string) (string, string) {
	mapper := pathMapper(pairs)
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

func Generate(ctx context.Context, fileNames []string, options ...Option) error {
	s := &settings{
		planner: Composite,
		printer: newPrinter(nil),
		typer:   defaultTyper,
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

func prefixPairs(pairs []string) [][2]string {
	if len(pairs)%2 != 0 {
		panic("must be even list of prefixes")
	}
	var prefixes [][2]string
	for i := 0; i < len(pairs); i += 2 {
		prefixes = append(prefixes, [2]string{pairs[i], pairs[i+1]})
	}
	return prefixes
}
