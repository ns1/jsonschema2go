// Code generated by internal/cmd/embedtmpl/embedtmpl.go DO NOT EDIT.
package mapobj

import (
	"text/template"
)

var tmpl = template.Must(template.New("").Parse(`{{/* gotype: github.com/ns1/jsonschema2go/internal/mapobj.mapPlanContext */}}
// {{ .Type.Name }} is generated from {{ .ID }}
{{ if .Comment -}}
// {{ .Comment }}
{{ end -}}
type {{ .Type.Name }} map[string]{{ .QualName .ValTypeInfo }}


{{ if .ValidateInitialize }}
var (
{{ range .Validators -}}
{{ .Var $.NameSpace }}
{{ end -}}
)
{{ end -}}

// Validate returns an error if this value is invalid according to rules defined in {{ .ID }}
func (m {{ .Type.Name }}) Validate() error {
{{ if gt .MinProperties 0 -}}
    if len(m) < {{ .MinProperties }} {
        return &validationError{
            errType: "min_properties",
            message: "minimum of {{ .MinProperties }} properties",
        }
    }
{{ end -}}
{{ if .HasMaxProperties -}}
    if len(m) > {{ .MaxProperties }} {
        return &validationError{
            errType: "max_properties",
            message: "maximum of {{ .MaxProperties }} properties",
        }
    }
{{ end -}}
{{ if .MapPlan.Validators -}}
    keys := make([]string, 0, len(m))
    for k := range m {
    	keys = append(keys, k)
    }
    strings.Sort(keys)
    for k := range keys {
    	v := m[k]
    	{{ range .MapPlan.Validators }}
        {{ if eq .Name "subschema" -}}
        if err := v.Validate(); err != nil {
            return err
        }
        {{ else -}}
        if {{ .Test $.NameSpace "v" }} {
            return &validationError{
                path: []interface{}{k},
                jsonPath: []interface{}{k},
                errType: "{{ .Name }}",
                message: fmt.Sprintf({{ .Sprintf $.NameSpace "v" }}),
            }
        }
        {{ end -}}
        {{ end -}}
    }
{{ end -}}
    return nil
}
`))
