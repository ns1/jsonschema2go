package jsonschema2go

import (
	"bufio"
	"context"
	"github.com/stretchr/testify/require"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode"
)

func Test_mapPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		prefixes [][2]string
		want     string
	}{
		{"empty", "blah", nil, "blah"},
		{"one", "github.com/jsonschema2go/foo/bar", [][2]string{{"github.com/jsonschema2go", "code"}}, "code/foo/bar"},
		{
			"greater",
			"github.com/jsonschema2go/foo/bar",
			[][2]string{{"github.com/jsonschema2go", "code"}, {"github.com/otherpath", "blob"}},
			"code/foo/bar",
		},
		{
			"less",
			"github.com/jsonschema2go/foo/bar",
			[][2]string{{"github.com/a", "other"}, {"github.com/jsonschema2go", "code"}},
			"code/foo/bar",
		},
		{
			"takes longest",
			"github.com/jsonschema2go/foo/bar",
			[][2]string{{"github.com/", "other"}, {"github.com/jsonschema2go", "code"}},
			"code/foo/bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pathMapper(tt.prefixes)(tt.path); got != tt.want {
				t.Errorf("mapPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRender(t *testing.T) {
	testDataDir, err := filepath.Abs("internal/testdata")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("internal/testdata/render")
	if err != nil {
		t.Fatal(err)
	}
	ents, err := ioutil.ReadDir(root)
	if err != nil {
		t.Fatalf("unable to list dir: %v", err)
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}

		t.Run(path.Base(e.Name()), func(t *testing.T) {
			r := require.New(t)

			testDir := path.Join(root, e.Name())

			args, err := readLines(path.Join(testDir, "args.txt"))
			r.NoError(err)

			var golden bool
			paths := make([]string, 0, len(args))
			for _, a := range args {
				if a == "GOLDEN" {
					golden = true
					continue
				}
				paths = append(paths, "file:"+path.Join(testDir, a))
			}


			dirName, err := ioutil.TempDir("", e.Name())
			r.NoError(err)
			defer os.RemoveAll(dirName)

			r.NoError(Generate(
				context.Background(),
				paths,
				PrefixMap("github.com/jwilner/jsonschema2go/internal/testdata", dirName),
				TypeFromID("https://example.com/testdata", "github.com/jwilner/jsonschema2go/internal/testdata"),
			))
			results, err := listAllFiles(dirName, ".gen.go")
			r.NoError(err)
			if golden {
				for _, p := range results {
					newPath := path.Join(testDataDir, p[len(dirName):])
					r.NoError(os.MkdirAll(filepath.Dir(newPath), 0755))
					func() {
						f, err := os.Open(p)
						r.NoError(err)
						defer f.Close()

						f2, err := os.Create(newPath)
						r.NoError(err)
						defer f2.Close()

						_, err = io.Copy(f2, f)
						r.NoError(err)
					}()
				}
			}

			resultsByName := keyedBySuffix(dirName, results)
			wanted, err := listAllFiles(testDir, ".gen.go")
			if err != nil {
				r.NoError(err)
			}
			wantedByName := keyedBySuffix(testDataDir, wanted)

			r.Equal(sortedKeys(wantedByName), sortedKeys(resultsByName))

			for _, k := range sortedKeys(wantedByName) {
				wanted, err := readString(wantedByName[k])
				r.NoError(err)

				result, err := readString(resultsByName[k])
				r.NoError(err)

				r.Equal(wanted, result, k)
			}
		})
	}
}

func listAllFiles(dir, ext string) (fns []string, err error) {
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ext) {
			fns = append(fns, path)
		}
		return nil
	})
	return
}

func keyedBySuffix(prefix string, paths []string) map[string]string {
	mapped := make(map[string]string)
	for _, p := range paths {
		if strings.HasPrefix(p, prefix) {
			mapped[p[len(prefix):]] = p
		}
	}
	return mapped
}

func sortedKeys(m map[string]string) (keys []string) {
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return
}

func readString(fname string) (string, error) {
	byts, err := ioutil.ReadFile(fname)
	if err != nil {
		return "", err
	}
	return string(byts), nil
}

func readLines(fname string) ([]string, error) {
	f, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	var lines []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		if l := strings.TrimFunc(s.Text(), unicode.IsSpace); l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

func Test_typeFromID(t *testing.T) {
	for _, tt := range []struct {
		name                   string
		pairs                  [][2]string
		id, wantPath, wantName string
	}{
		{
			name:     "maps",
			pairs:    [][2]string{{"https://example.com/v1/", "github.com/example/"}},
			id:       "https://example.com/v1/blah/bar.json",
			wantPath: "github.com/example/blah",
			wantName: "bar",
		},
		{
			name:     "maps no extension",
			pairs:    [][2]string{{"https://example.com/v1/", "github.com/example/"}},
			id:       "https://example.com/v1/blah/bar",
			wantPath: "github.com/example/blah",
			wantName: "bar",
		},
		{
			name:     "maps no pairs",
			pairs:    [][2]string{},
			id:       "https://example.com/v1/blah/bar",
			wantPath: "example.com/v1/blah",
			wantName: "bar",
		},
		{
			name:     "maps no scheme",
			pairs:    [][2]string{},
			id:       "example.com/v1/blah/bar",
			wantPath: "example.com/v1/blah",
			wantName: "bar",
		},
		{
			name:     "maps empty fragment",
			pairs:    [][2]string{{"https://example.com/v1/", "github.com/example/"}},
			id:       "https://example.com/v1/blah/bar.json#",
			wantPath: "github.com/example/blah",
			wantName: "bar",
		},
		{
			name:     "maps properties fragment",
			pairs:    [][2]string{{"https://example.com/v1/", "github.com/example/"}},
			id:       "https://example.com/v1/blah/bar.json#/properties/baz",
			wantPath: "github.com/example/blah",
			wantName: "barBaz",
		},
		{
			name:     "maps extended fragment",
			pairs:    [][2]string{{"https://example.com/v1/", "github.com/example/"}},
			id:       "https://example.com/v1/blah/bar.json#/properties/baz/items/2/properties/hello",
			wantPath: "github.com/example/blah",
			wantName: "barBazItems2Hello",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if path, name := typeFromID(tt.pairs)(tt.id); tt.wantName != name || tt.wantPath != path {
				t.Errorf("wanted (%q, %q) got (%q, %q)", tt.wantPath, tt.wantName, path, name)
			}
		})
	}
}
