package jsonschema2go

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Render(ctx context.Context, loader Loader, fileNames []string, prefixes [][2]string) error {
	seen := make(map[TypeInfo]bool)
	grouped := make(map[[2]string][]Plan)
	for _, fileName := range fileNames {
		schema, err := Resolve(ctx, loader, fileName)
		if err != nil {
			return fmt.Errorf("unable to resolve schema from %q: %w", fileName, err)
		}
		newPlans, err := SchemaToPlan(schema)
		if err != nil {
			return fmt.Errorf("unable to create plans from schema %q: %w ", fileName, err)
		}

		for _, plan := range newPlans {
			if typ := plan.Type(); !seen[typ] {
				seen[typ] = true

				key := [...]string{typ.GoPath, typ.FileName}
				grouped[key] = append(grouped[key], plan)
			}
		}
	}

	sort.Slice(prefixes, func(i, j int) bool {
		return prefixes[i][0] < prefixes[j][0]
	})

	for k, group := range grouped {
		path := mapPath(k[0], prefixes)
		if path == "" {
			return fmt.Errorf("unable to map go path: %q", k[0])
		}
		if err := func() error {
			if err := os.MkdirAll(path, 0755); err != nil {
				return fmt.Errorf("unable to dir %q: %w", path, err)
			}
			f, err := os.Create(filepath.Join(path, k[1]))
			if err != nil {
				return fmt.Errorf("unable to open: %w", err)
			}
			defer f.Close()

			if err := PrintFile(ctx, f, k[0], group); err != nil {
				return fmt.Errorf("unable to print: %w", err)
			}
			return nil
		}(); err != nil {
			return err
		}
	}
	return nil
}

func mapPath(path string, sortedPrefixes [][2]string) string {
	i := sort.Search(len(sortedPrefixes), func(i int) bool {
		return sortedPrefixes[i][0] > path
	})
	for i = i - 1; i >= 0; i-- {
		if strings.HasPrefix(path, sortedPrefixes[i][0]) {
			return sortedPrefixes[i][1] + path[len(sortedPrefixes[i][0]):]
		}
	}
	return ""
}
