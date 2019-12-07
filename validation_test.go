package jsonschema2go_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jwilner/jsonschema2go"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type SchemaCase struct {
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
	Tests       []TestCase      `json:"tests"`
	Skip        string          `json:"skip"`
}

type TestCase struct {
	Description string          `json:"description"`
	Data        json.RawMessage `json:"data"`
	Output      json.RawMessage `json:"output"`
	Valid       bool            `json:"valid"`
	Skip        string          `json:"skip"`
}

func TestValidation(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()

	const root = "test/testdata/validation/"

	ents, err := ioutil.ReadDir(root)
	r.NoError(err)

	for _, e := range ents {
		t.Run(path.Base(e.Name()), func(t *testing.T) {
			r := require.New(t)

			var cases []SchemaCase
			r.NoError(func() error {
				f, err := os.Open(path.Join(root, e.Name()))
				if err != nil {
					return err
				}
				defer f.Close()

				return json.NewDecoder(f).Decode(&cases)
			}())
			for _, sc := range cases {
				t.Run(sc.Description, func(t *testing.T) {
					if sc.Skip != "" {
						t.Skip(sc.Skip)
					}
					v := compileValidator(ctx, require.New(t), sc.Schema)
					defer v.Close()

					for _, tc := range sc.Tests {
						t.Run(tc.Description, func(t *testing.T) {
							if tc.Skip != "" {
								t.Skip(tc.Skip)
							}
							r := require.New(t)
							res, errS, err := v.Validate(ctx, tc.Data)
							r.NoError(err)

							f, _ := ioutil.ReadFile(v.valuesPath)

							if tc.Valid {
								r.Equal("valid", res, "got this: "+errS+string(f))
								return
							}

							r.Contains([]string{"err_unmarshal", "err_validate"}, res, string(f))
						})
					}
				})
			}
		})
	}
}

type validator struct {
	workDir, harnessPath, valuesPath string
}

func (v *validator) Validate(ctx context.Context, msg json.RawMessage) (string, string, error) {
	cmd := exec.CommandContext(ctx, v.harnessPath)
	cmd.Stdin = bytes.NewReader(msg)
	var out, errB bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errB
	err := cmd.Run()
	return string(out.Bytes()), string(errB.Bytes()), err
}

func (v *validator) Close() error {
	return os.RemoveAll(v.workDir)
}

func compileValidator(ctx context.Context, r *require.Assertions, schema json.RawMessage) *validator {
	dirName, err := ioutil.TempDir("", "")
	r.NoError(err)

	src := path.Join(dirName, "schema.json")

	r.NoError(ioutil.WriteFile(src, schema, 0644))

	src, err = filepath.Abs(src)
	r.NoError(err)

	var (
		names = make(map[*jsonschema2go.Schema]string)
		mux   sync.Mutex
	)

	r.NoError(jsonschema2go.Generate(
		ctx,
		[]string{"file:" + src},
		jsonschema2go.CustomTypeFunc(func(schema *jsonschema2go.Schema) jsonschema2go.TypeInfo {
			if schema.Config.GoPath != "" {
				parts := strings.SplitN(schema.Config.GoPath, "#", 2)
				return jsonschema2go.TypeInfo{GoPath: parts[0], Name: parts[1]}
			}
			mux.Lock()
			defer mux.Unlock()

			if _, ok := names[schema]; !ok {
				names[schema] = string('a' + len(names))
			}

			return jsonschema2go.TypeInfo{GoPath: "main", Name: names[schema]}
		}),
		jsonschema2go.PrefixMap("main", dirName),
		jsonschema2go.Debug(true),
	))

	_, err = os.Stat(path.Join(dirName, "values.gen.go"))
	r.NoError(err)

	main, err := os.Create(path.Join(dirName, "main.go"))
	r.NoError(err)

	_, _ = main.WriteString(`
package main

import (
	"os"
	"fmt"
	"encoding/json"
)

func main() {
	var val A
	if err := json.NewDecoder(os.Stdin).Decode(&val); err != nil {
		fmt.Fprint(os.Stdout, "err_unmarshal")
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := val.Validate(); err != nil {
		fmt.Fprint(os.Stdout, "err_validate")
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Fprint(os.Stdout, "valid")
}
`)
	harnessPath := path.Join(dirName, "harness")
	mainPath := path.Join(dirName, "main.go")
	valuesPath := path.Join(dirName, "values.gen.go")
	cmd := exec.CommandContext(
		ctx,
		"go",
		"build",
		"-o",
		harnessPath,
		mainPath,
		valuesPath,
	)
	cmd.Stderr = os.Stderr
	f, _ := ioutil.ReadFile(valuesPath)
	if err := cmd.Run(); err != nil {
		r.NoError(err, f)
	}
	return &validator{workDir: dirName, harnessPath: harnessPath, valuesPath: valuesPath}
}
