package jsonschema2go

import (
	"context"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
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
	root, err := filepath.Abs("testdata/render")
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
		testDir := path.Join(root, e.Name())

		schemas, err := listAllFiles(testDir, ".json")
		if err != nil {
			t.Fatal(err)
		}
		for i := range schemas {
			schemas[i] = "file:" + schemas[i]
		}

		wantDir := path.Join(root, e.Name())
		wanted, err := listAllFiles(testDir, ".gen.go")
		if err != nil {
			t.Fatal(err)
		}

		t.Run(path.Base(e.Name()), func(t *testing.T) {
			r := require.New(t)

			dirName, err := ioutil.TempDir("", e.Name())
			r.NoError(err)
			defer os.RemoveAll(dirName)

			renderer := NewRenderer()
			r.NoError(renderer.Render(context.Background(), schemas, [][2]string{{"example.com/v1", dirName}}))
			results, err := listAllFiles(dirName, ".gen.go")
			r.NoError(err)
			wantedByName := keyedBySuffix(wantDir, wanted)
			resultsByName := keyedBySuffix(dirName, results)

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
