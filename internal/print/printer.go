package print

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ns1/jsonschema2go/pkg/gen"
	"go/format"
	"io"
	"text/template"
)

var baseImports = []string{"fmt"} // used for error messaging

type Printer interface {
	Print(ctx context.Context, w io.Writer, goPath string, plans []gen.Plan) error
}

//go:generate go run ../cmd/embedtmpl/embedtmpl.go print values.tmpl tmpl.gen.go
func New(t *template.Template) Printer {
	if t == nil {
		t = tmpl
	}
	return &printer{t}
}

type printer struct {
	tmpl *template.Template
}

func (p *printer) Print(ctx context.Context, w io.Writer, goPath string, plans []gen.Plan) error {
	depPaths := make([]string, len(baseImports))
	copy(depPaths, baseImports)
	for _, pl := range plans {
		for _, d := range pl.Deps() {
			depPaths = append(depPaths, d.GoPath)
		}
	}
	imps := gen.NewImports(goPath, depPaths)

	var buf bytes.Buffer
	if err := p.tmpl.Execute(&buf, &Plans{imps, defaultSort(plans)}); err != nil {
		return fmt.Errorf("unable to execute tmpl: %w", err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		if gen.IsDebug(ctx) {
			_, _ = w.Write(buf.Bytes()) // write unformatted for debugging
		}
		return fmt.Errorf("unable to format: %w", err)
	}

	_, err = w.Write(formatted)
	return err
}

type Plans struct {
	Imports *gen.Imports
	Plans   []gen.Plan
}
