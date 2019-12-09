package testharness

import (
	"bufio"
	"context"
	"github.com/jwilner/jsonschema2go"
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

// RunGenerateTests runs tests which verify the appearance of generated code.
func RunGenerateTests(t *testing.T, testDataDir, root, goPath string) {
	var err error
	if testDataDir, err = filepath.Abs(testDataDir); err != nil {
		t.Fatal(err)
	}
	if root, err = filepath.Abs(root); err != nil {
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

			r.NoError(jsonschema2go.Generate(
				context.Background(),
				paths,
				jsonschema2go.PrefixMap(goPath, dirName),
				jsonschema2go.TypeFromID(
					"https://example.com/testdata",
					goPath,
				),
				jsonschema2go.Debug(true),
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
