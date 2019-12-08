package print

import (
	"bytes"
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/pkg/ctxflags"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	"go/format"
	"io"
	"text/template"
)

type Printer interface {
	Print(ctx context.Context, w io.Writer, goPath string, plans []generate.Plan) error
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

func (p *printer) Print(ctx context.Context, w io.Writer, goPath string, plans []generate.Plan) error {
	var depPaths []string
	for _, pl := range plans {
		for _, d := range pl.Deps() {
			depPaths = append(depPaths, d.GoPath)
		}
	}
	imps := generate.NewImports(goPath, depPaths)

	var buf bytes.Buffer
	if err := p.tmpl.Execute(&buf, &Plans{imps, defaultSort(plans)}); err != nil {
		return fmt.Errorf("unable to execute tmpl: %w", err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		if ctxflags.IsDebug(ctx) {
			_, _ = w.Write(buf.Bytes()) // write unformatted for debugging
		}
		return fmt.Errorf("unable to format: %w", err)
	}

	_, err = w.Write(formatted)
	return err
}

type Plans struct {
	Imports *generate.Imports
	Plans   []generate.Plan
}
