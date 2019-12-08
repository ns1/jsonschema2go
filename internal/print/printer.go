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

func New(tmpl *template.Template) Printer {
	if tmpl == nil {
		tmpl = valueTmpl
	}
	return &printer{valueTmpl}
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

	sorted := make([]generate.PrintablePlan, 0, len(plans))
	for _, p := range defaultSort(plans) {
		sorted = append(sorted, p.Printable(imps))
	}

	var buf bytes.Buffer
	if err := valueTmpl.Execute(&buf, &Plans{imps, sorted}); err != nil {
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
	Plans   []generate.PrintablePlan
}

func (ps *Plans) Structs() (structs []generate.PrintablePlan) {
	for _, p := range ps.Plans {
		if p.Template() == "struct.tmpl" {
			structs = append(structs, p)
		}
	}
	return
}

func (ps *Plans) Slices() (slices []generate.PrintablePlan) {
	for _, p := range ps.Plans {
		if p.Template() == "slice.tmpl" {
			slices = append(slices, p)
		}
	}
	return
}

func (ps *Plans) Tuples() (tuples []generate.PrintablePlan) {
	for _, p := range ps.Plans {
		if p.Template() == "tuple.tmpl" {
			tuples = append(tuples, p)
		}
	}
	return
}

func (ps *Plans) Enums() (enums []generate.PrintablePlan) {
	for _, p := range ps.Plans {
		if p.Template() == "enum.tmpl" {
			enums = append(enums, p)
		}
	}
	return
}
