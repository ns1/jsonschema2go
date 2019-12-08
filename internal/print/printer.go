package print

import (
	"bytes"
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/templates"
	"github.com/jwilner/jsonschema2go/pkg/ctxflags"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	"go/format"
	"io"
	"text/template"
)

type Printer interface {
	Print(ctx context.Context, w io.Writer, goPath string, plans []generate.Plan) error
}

func New(tmpl *template.Template) Printer {
	if tmpl == nil {
		tmpl = templates.Template
	}
	return &printer{tmpl}
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
	if err := templates.Template.Execute(&buf, &Plans{imps, defaultSort(plans)}); err != nil {
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
