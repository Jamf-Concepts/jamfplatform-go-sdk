// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strconv"
	"strings"
	"text/template"
)

// ---------------------------------------------------------------------------
// Templates — parsed once at init
// ---------------------------------------------------------------------------

var funcMap = template.FuncMap{
	"httpConst": func(method string) string {
		m := map[string]string{
			"GET": "http.MethodGet", "POST": "http.MethodPost",
			"PATCH": "http.MethodPatch", "PUT": "http.MethodPut",
			"DELETE": "http.MethodDelete",
		}
		if v, ok := m[method]; ok {
			return v
		}
		return fmt.Sprintf("%q", method)
	},
	"statusConst": func(code int) string {
		m := map[int]string{200: "http.StatusOK", 201: "http.StatusCreated", 202: "http.StatusAccepted", 204: "http.StatusNoContent"}
		if v, ok := m[code]; ok {
			return v
		}
		return strconv.Itoa(code)
	},
	"fmtPath": func(m GoMethod) string {
		if len(m.PathParams) == 0 {
			return fmt.Sprintf(`prefix + "%s"`, m.ResourcePath)
		}
		fmtStr := pathParamRe.ReplaceAllString(m.ResourcePath, "%s")
		args := []string{"prefix"}
		for _, p := range m.PathParams {
			args = append(args, "url.PathEscape("+p.GoName+")")
		}
		return fmt.Sprintf(`fmt.Sprintf("%%s%s", %s)`, fmtStr, strings.Join(args, ", "))
	},
	"errWrap": func(m GoMethod) string {
		if len(m.PathParams) == 0 {
			return fmt.Sprintf(`"%s: %%w", err`, m.Name)
		}
		return fmt.Sprintf(`"%s(%%s): %%w", %s, err`, m.Name, m.PathParams[0].GoName)
	},
	"testPath": func(m GoMethod) string {
		var base string
		if m.Version == "" {
			base = fmt.Sprintf("/api/%s/tenant/t-test", m.Namespace)
		} else {
			base = fmt.Sprintf("/api/%s/%s/tenant/t-test", m.Namespace, m.Version)
		}
		path := pathParamRe.ReplaceAllString(stripVersionPrefix(m.SpecPath), "test-id")
		return base + path
	},
	"testCallArgs": func(m GoMethod) string {
		if len(m.PathParams) == 0 {
			return ""
		}
		args := make([]string, len(m.PathParams))
		for i := range m.PathParams {
			args[i] = `"test-id"`
		}
		return ", " + strings.Join(args, ", ")
	},
	"testExtraArgs": func(m GoMethod) string {
		if len(m.QueryParams) == 0 {
			return ""
		}
		args := make([]string, len(m.QueryParams))
		for i, qp := range m.QueryParams {
			switch qp.Type {
			case "[]string":
				args[i] = "nil"
			case "bool":
				args[i] = "false"
			case "int", "int64":
				args[i] = "0"
			default:
				args[i] = `""`
			}
		}
		return ", " + strings.Join(args, ", ")
	},
	"isStringSlice": func(s string) bool { return s == "[]string" },
	"requestArg": func(t string) string {
		// Test stub's zero-value literal for the request parameter.
		// Primitives can't be composite-literal'd (e.g. `&string{}`
		// is invalid), so use new(T) which yields *T zero-pointer.
		switch t {
		case "string", "bool", "int", "int32", "int64", "float32", "float64":
			return "new(" + t + ")"
		}
		return "&" + t + "{}"
	},
	// resolverNameStub converts a dot-path nameField to a Go map literal entry
	// for use in test stubs. "name" → `"name": "target"`;
	// "general.name" → `"general": map[string]any{"name": "target"}`.
	"resolverNameStub": func(nameField string) string {
		parts := strings.Split(nameField, ".")
		if len(parts) == 1 {
			return fmt.Sprintf(`%q: "target"`, parts[0])
		}
		inner := fmt.Sprintf(`%q: "target"`, parts[len(parts)-1])
		for i := len(parts) - 2; i >= 0; i-- {
			inner = fmt.Sprintf(`%q: map[string]any{%s}`, parts[i], inner)
		}
		return inner
	},
	// resolverIDStub returns the test stub value for the ID field.
	// Numeric IDs emit 42 (unquoted); string IDs emit "resolved-id".
	"resolverIDStub": func(r *GoResolver) string {
		if r.IDNumeric {
			return fmt.Sprintf(`%q: 42`, r.IDField)
		}
		return fmt.Sprintf(`%q: "resolved-id"`, r.IDField)
	},
	// resolverExpectedID returns the expected ID string for test assertions.
	"resolverExpectedID": func(r *GoResolver) string {
		if r.IDNumeric {
			return "42"
		}
		return "resolved-id"
	},
	"testMultipartArgs": func(m GoMethod) string {
		if len(m.MultipartFields) == 0 {
			return ""
		}
		args := make([]string, 0, len(m.MultipartFields)*2)
		for _, f := range m.MultipartFields {
			if f.IsFile {
				args = append(args, `"test.bin"`, `bytes.NewBufferString("stub")`)
			} else {
				args = append(args, `""`)
			}
		}
		return ", " + strings.Join(args, ", ")
	},
	// applyListPath builds the test-server handler path for the list endpoint
	// used by the apply method's resolver call.
	"applyListPath": func(a *GoApply) string {
		if a.ListVersion == "" {
			return fmt.Sprintf("/api/%s/tenant/t-test%s", a.ListNamespace, a.ListPath)
		}
		return fmt.Sprintf("/api/%s/%s/tenant/t-test%s", a.ListNamespace, a.ListVersion, a.ListPath)
	},
	// applyCreatePath builds the test-server handler path for the create endpoint.
	"applyCreatePath": func(a *GoApply) string {
		path := pathParamRe.ReplaceAllString(a.CreatePath, "test-id")
		if a.CreateVer == "" {
			return fmt.Sprintf("/api/%s/tenant/t-test%s", a.CreateNS, path)
		}
		return fmt.Sprintf("/api/%s/%s/tenant/t-test%s", a.CreateNS, a.CreateVer, path)
	},
	// applyUpdatePath builds the test-server handler path for the update endpoint.
	"applyUpdatePath": func(a *GoApply) string {
		path := pathParamRe.ReplaceAllString(a.UpdatePath, "existing-id")
		if a.UpdateVer == "" {
			return fmt.Sprintf("/api/%s/tenant/t-test%s", a.UpdateNS, path)
		}
		return fmt.Sprintf("/api/%s/%s/tenant/t-test%s", a.UpdateNS, a.UpdateVer, path)
	},
	// applyCreateStatus returns the HTTP status code for test create responses.
	"applyCreateStatus": func(a *GoApply) int {
		if a.CreateStatus != 0 {
			return a.CreateStatus
		}
		return 201
	},
	// applyUpdateStatus returns the HTTP status code for test update responses.
	"applyUpdateStatus": func(a *GoApply) int {
		if a.UpdateStatus != 0 {
			return a.UpdateStatus
		}
		return 200
	},
	// applyRequestExpr returns a Go expression for the request struct literal
	// with the name field populated to "target". Handles both pointer and
	// non-pointer name fields.
	"applyRequestExpr": func(a *GoApply) string {
		if a.NameIsPointer {
			return fmt.Sprintf("&%s{%s: ptrStr(\"target\")}", a.RequestType, a.NameGoField)
		}
		return fmt.Sprintf("&%s{%s: \"target\"}", a.RequestType, a.NameGoField)
	},
	// applyTestCreateIDJSON returns the JSON value for the mock create response ID.
	"applyTestCreateIDJSON": func(a *GoApply) string {
		if a.IDNumeric {
			return "42"
		}
		return `"new-id"`
	},
	// applyTestCreateIDExpected returns the expected string ID after create.
	"applyTestCreateIDExpected": func(a *GoApply) string {
		if a.IDNumeric {
			return "42"
		}
		return "new-id"
	},
}

var sourceTmpl = template.Must(template.New("source").Funcs(funcMap).Parse(sourceTemplate))
var testTmpl = template.Must(template.New("tests").Delims("<%", "%>").Funcs(funcMap).Parse(testTemplate))

// ---------------------------------------------------------------------------
// Source template
// ---------------------------------------------------------------------------

var sourceTemplate = `// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package {{ .Package }}

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"{{ .Module }}/internal/client"
)

{{- if .Types }}
{{ range .Types }}
{{- if .AliasTarget }}
// {{ .Comment }}
type {{ .Name }} = {{ .AliasTarget }}
{{- else if .IsRawJSON }}
// {{ .Comment }}
type {{ .Name }} = json.RawMessage
{{- else if .Discriminator }}
// {{ .Comment }}
type {{ .Name }} struct {
	{{ .Discriminator.GoFieldName }} string ` + "`" + `json:"{{ .Discriminator.PropertyName }}"` + "`" + `
{{- range .Discriminator.Variants }}
	{{ .FieldName }} *{{ .TypeName }} ` + "`" + `json:"-"` + "`" + `
{{- end }}
}

// UnmarshalJSON dispatches the payload to the variant matching the
// {{ .Discriminator.PropertyName }} discriminator. Unknown values leave the variant
// pointers nil but preserve the discriminator string.
func (m *{{ .Name }}) UnmarshalJSON(data []byte) error {
	var d struct {
		{{ .Discriminator.GoFieldName }} string ` + "`" + `json:"{{ .Discriminator.PropertyName }}"` + "`" + `
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}
	m.{{ .Discriminator.GoFieldName }} = d.{{ .Discriminator.GoFieldName }}
	switch d.{{ .Discriminator.GoFieldName }} {
{{- range .Discriminator.Variants }}
	case "{{ .Value }}":
		m.{{ .FieldName }} = new({{ .TypeName }})
		return json.Unmarshal(data, m.{{ .FieldName }})
{{- end }}
	}
	return nil
}

// MarshalJSON emits the active variant's JSON. If the matching variant
// pointer is nil, emits a minimal object carrying only the discriminator.
func (m {{ .Name }}) MarshalJSON() ([]byte, error) {
	switch m.{{ .Discriminator.GoFieldName }} {
{{- range .Discriminator.Variants }}
	case "{{ .Value }}":
		return json.Marshal(m.{{ .FieldName }})
{{- end }}
	}
	return json.Marshal(map[string]string{"{{ .Discriminator.PropertyName }}": m.{{ .Discriminator.GoFieldName }}})
}
{{- else if .Fields }}
// {{ .Comment }}
type {{ .Name }} struct {
{{- if and (eq $.Format "xml") .XMLName }}
	XMLName xml.Name
{{- end }}
{{- range .Fields }}
{{- if .Comment }}
	// {{ .Comment }}
{{- end }}
{{- if eq $.Format "xml" }}
	{{ .Name }} {{ .Type }} ` + "`" + `xml:"{{ .JSONTag }}"` + "`" + `
{{- else }}
	{{ .Name }} {{ .Type }} ` + "`" + `json:"{{ .JSONTag }}"` + "`" + `
{{- end }}
{{- end }}
}

{{- if and (eq $.Format "xml") .XMLName }}

// MarshalXML forces the {{ .Name }} root element name to the wire value
// declared by the spec (<{{ .XMLName }}>) regardless of what XMLName.Local
// holds. Classic resources are frequently decoded from polymorphic wire
// roots (<static_user_group>, <smart_user_group>, <user_group>, etc.) —
// stashing the incoming root name in XMLName is useful context but must
// not leak back into writes. The shadow type suppresses re-entry into
// this method during encoding.
func (t {{ .Name }}) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: "{{ .XMLName }}"}
	type shadow {{ .Name }}
	return e.EncodeElement(shadow(t), start)
}
{{- end }}
{{- else }}
// {{ .Comment }}
type {{ .Name }} = string
{{- end }}
{{ end }}
{{- end }}
{{ range .Methods }}
{{- if eq .Category "paginated" }}
{{ template "paginated" . }}
{{- else if eq .Category "unwrap" }}
{{ template "unwrap" . }}
{{- else if eq .Category "get" }}
{{ template "get" . }}
{{- else if eq .Category "create" }}
{{ template "create" . }}
{{- else if eq .Category "actionWithResponse" }}
{{ template "actionWithResponse" . }}
{{- else if eq .Category "update" }}
{{ template "update" . }}
{{- else if eq .Category "multipart" }}
{{ template "multipart" . }}
{{- else if eq .Category "raw" }}
{{ template "raw" . }}
{{- else if eq .Category "resolverID" }}
{{ template "resolverID" . }}
{{- else if eq .Category "resolverTyped" }}
{{ template "resolverTyped" . }}
{{- else if eq .Category "resolverIDDirect" }}
{{ template "resolverIDDirect" . }}
{{- else if eq .Category "resolverTypedDirect" }}
{{ template "resolverTypedDirect" . }}
{{- else if eq .Category "apply" }}
{{ template "apply" . }}
{{- else }}
{{ template "action" . }}
{{- end }}
{{- end }}

{{/* ---- Shared sub-templates ---- */}}

{{- define "buildQueryParams" -}}
{{- if .QueryParams }}
	params := url.Values{}
{{- range .QueryParams }}
{{- if eq .Type "[]string" }}
	if len({{ .Go }}) > 0 {
		params.Set("{{ .Spec }}", strings.Join({{ .Go }}, ","))
	}
{{- else if eq .Type "bool" }}
	if {{ .Go }} {
		params.Set("{{ .Spec }}", "true")
	}
{{- else if eq .Type "int" }}
	if {{ .Go }} != 0 {
		params.Set("{{ .Spec }}", strconv.Itoa({{ .Go }}))
	}
{{- else if eq .Type "int64" }}
	if {{ .Go }} != 0 {
		params.Set("{{ .Spec }}", strconv.FormatInt({{ .Go }}, 10))
	}
{{- else }}
	if {{ .Go }} != "" {
		params.Set("{{ .Spec }}", {{ .Go }})
	}
{{- end }}
{{- end }}
	if encoded := params.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
{{- end }}
{{- end }}

{{- define "get" }}
// {{ .Comment }}
{{- if .ReturnsSlice }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) ({{ .ResponseType }}, error) {
{{- else }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) (*{{ .ResponseType }}, error) {
{{- end }}
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result {{ .ResponseType }}
	endpoint := {{ fmtPath . }}
{{- template "buildQueryParams" . }}
	if err := c.transport.Do(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, &result); err != nil {
		return nil, fmt.Errorf({{ errWrap . }})
	}
{{- if .ReturnsSlice }}
	return result, nil
{{- else }}
	return &result, nil
{{- end }}
}
{{ end }}

{{- define "create" }}
// {{ .Comment }}
{{- if .ReturnsSlice }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}, request *{{ .RequestType }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) ({{ .ResponseType }}, error) {
{{- else }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}, request *{{ .RequestType }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) (*{{ .ResponseType }}, error) {
{{- end }}
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result {{ .ResponseType }}
	endpoint := {{ fmtPath . }}
{{- template "buildQueryParams" . }}
{{- if .ContentType }}
	if err := c.transport.DoWithContentType(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, "{{ .ContentType }}", {{ statusConst .ExpectedStatus }}, &result); err != nil {
{{- else }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, {{ statusConst .ExpectedStatus }}, &result); err != nil {
{{- end }}
		return nil, fmt.Errorf({{ errWrap . }})
	}
{{- if .ReturnsSlice }}
	return result, nil
{{- else }}
	return &result, nil
{{- end }}
}
{{ end }}

{{- define "actionWithResponse" }}
// {{ .Comment }}
{{- if .ReturnsSlice }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) ({{ .ResponseType }}, error) {
{{- else }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) (*{{ .ResponseType }}, error) {
{{- end }}
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result {{ .ResponseType }}
	endpoint := {{ fmtPath . }}
{{- template "buildQueryParams" . }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, {{ statusConst .ExpectedStatus }}, &result); err != nil {
		return nil, fmt.Errorf({{ errWrap . }})
	}
{{- if .ReturnsSlice }}
	return result, nil
{{- else }}
	return &result, nil
{{- end }}
}
{{ end }}

{{- define "update" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}, request *{{ .RequestType }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) error {
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	endpoint := {{ fmtPath . }}
{{- template "buildQueryParams" . }}
{{- if .ContentType }}
	if err := c.transport.DoWithContentType(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, "{{ .ContentType }}", {{ statusConst .ExpectedStatus }}, nil); err != nil {
{{- else }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, request, {{ statusConst .ExpectedStatus }}, nil); err != nil {
{{- end }}
		return fmt.Errorf({{ errWrap . }})
	}
	return nil
}
{{ end }}

{{- define "action" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) error {
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	endpoint := {{ fmtPath . }}
{{- template "buildQueryParams" . }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, {{ statusConst .ExpectedStatus }}, nil); err != nil {
		return fmt.Errorf({{ errWrap . }})
	}
	return nil
}
{{ end }}

{{- define "multipart" }}
// {{ .Comment }}
//
// For file parts, pass an *os.File or *bytes.Reader (anything that
// implements io.Seeker) so the SDK can precompute an exact
// Content-Length and retry once on a 429/Retry-After. A plain
// io.Reader is accepted too but the upload falls back to chunked
// transfer encoding and is not retried on 429.
{{- if .ResponseType }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .MultipartFields }}{{ if .IsFile }}, {{ .GoName }}Filename string, {{ .GoName }} io.Reader{{ else }}, {{ .GoName }} {{ .Type }}{{ end }}{{ end }}) (*{{ .ResponseType }}, error) {
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result {{ .ResponseType }}
	endpoint := {{ fmtPath . }}
	parts := []client.MultipartField{
{{- range .MultipartFields }}
{{- if .IsFile }}
		{Name: "{{ .Name }}", Filename: {{ .GoName }}Filename, Content: {{ .GoName }}},
{{- else }}
		{Name: "{{ .Name }}", Value: {{ .GoName }}},
{{- end }}
{{- end }}
	}
	if err := c.transport.DoMultipart(ctx, {{ httpConst .HTTPMethod }}, endpoint, parts, {{ statusConst .ExpectedStatus }}, &result); err != nil {
		return nil, fmt.Errorf({{ errWrap . }})
	}
	return &result, nil
}
{{- else }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .MultipartFields }}{{ if .IsFile }}, {{ .GoName }}Filename string, {{ .GoName }} io.Reader{{ else }}, {{ .GoName }} {{ .Type }}{{ end }}{{ end }}) error {
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	endpoint := {{ fmtPath . }}
	parts := []client.MultipartField{
{{- range .MultipartFields }}
{{- if .IsFile }}
		{Name: "{{ .Name }}", Filename: {{ .GoName }}Filename, Content: {{ .GoName }}},
{{- else }}
		{Name: "{{ .Name }}", Value: {{ .GoName }}},
{{- end }}
{{- end }}
	}
	if err := c.transport.DoMultipart(ctx, {{ httpConst .HTTPMethod }}, endpoint, parts, {{ statusConst .ExpectedStatus }}, nil); err != nil {
		return fmt.Errorf({{ errWrap . }})
	}
	return nil
}
{{- end }}
{{ end }}

{{- define "raw" }}
// {{ .Comment }}
{{- if and .RequestType .ResponseType }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}, body []byte) ([]byte, error) {
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result []byte
	endpoint := {{ fmtPath . }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, body, {{ statusConst .ExpectedStatus }}, &result); err != nil {
		return nil, fmt.Errorf({{ errWrap . }})
	}
	return result, nil
}
{{- else if .RequestType }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}, body []byte) error {
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	endpoint := {{ fmtPath . }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, body, {{ statusConst .ExpectedStatus }}, nil); err != nil {
		return fmt.Errorf({{ errWrap . }})
	}
	return nil
}
{{- else if .ResponseType }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}) ([]byte, error) {
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	var result []byte
	endpoint := {{ fmtPath . }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, {{ statusConst .ExpectedStatus }}, &result); err != nil {
		return nil, fmt.Errorf({{ errWrap . }})
	}
	return result, nil
}
{{- else }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}) error {
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	endpoint := {{ fmtPath . }}
	if err := c.transport.DoExpect(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, {{ statusConst .ExpectedStatus }}, nil); err != nil {
		return fmt.Errorf({{ errWrap . }})
	}
	return nil
}
{{- end }}
{{ end }}

{{- define "unwrap" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) ({{ .UnwrapResults }}, error) {
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	endpoint := {{ fmtPath . }}
{{- template "buildQueryParams" . }}

	var result struct {
		TotalCount int              ` + "`" + `json:"totalCount"` + "`" + `
		Results    {{ .UnwrapResults }} ` + "`" + `json:"results"` + "`" + `
	}
	if err := c.transport.Do(ctx, {{ httpConst .HTTPMethod }}, endpoint, nil, &result); err != nil {
		return nil, fmt.Errorf({{ errWrap . }})
	}
	return result.Results, nil
}
{{ end }}

{{- define "paginated" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context{{ range .PathParams }}, {{ .GoName }} string{{ end }}{{ range .QueryParams }}, {{ .Go }} {{ .Type }}{{ end }}) ([]{{ .ItemType }}, error) {
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	return client.ListAllPages(ctx, func(ctx context.Context, page, pageSize int) ([]{{ .ItemType }}, bool, error) {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("{{ .PageSizeParam }}", strconv.Itoa(pageSize))
{{- range .QueryParams }}
{{- if eq .Type "[]string" }}
		if len({{ .Go }}) > 0 {
			params.Set("{{ .Spec }}", strings.Join({{ .Go }}, ","))
		}
{{- else if eq .Type "bool" }}
		if {{ .Go }} {
			params.Set("{{ .Spec }}", "true")
		}
{{- else if eq .Type "int" }}
		if {{ .Go }} != 0 {
			params.Set("{{ .Spec }}", strconv.Itoa({{ .Go }}))
		}
{{- else if eq .Type "int64" }}
		if {{ .Go }} != 0 {
			params.Set("{{ .Spec }}", strconv.FormatInt({{ .Go }}, 10))
		}
{{- else }}
		if {{ .Go }} != "" {
			params.Set("{{ .Spec }}", {{ .Go }})
		}
{{- end }}
{{- end }}

		endpoint := {{ fmtPath . }}
		if encoded := params.Encode(); encoded != "" {
			endpoint += "?" + encoded
		}

{{- if eq .PaginationStyle "hasNext" }}
		var result struct {
			client.PaginatedResponseRepresentation
			Results []{{ .ItemType }} ` + "`" + `json:"{{ .ResultsField }}"` + "`" + `
		}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		return result.Results, result.HasNext, nil
{{- else if eq .PaginationStyle "sizeCheck" }}
		var result struct {
			Results    []{{ .ItemType }} ` + "`" + `json:"{{ .ResultsField }}"` + "`" + `
			TotalCount int64 ` + "`" + `json:"totalCount"` + "`" + `
		}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		return result.Results, len(result.Results) >= pageSize && len(result.Results) > 0, nil
{{- else if eq .PaginationStyle "totalCount" }}
		var result struct {
			TotalCount int ` + "`" + `json:"totalCount"` + "`" + `
			Results    []{{ .ItemType }} ` + "`" + `json:"{{ .ResultsField }}"` + "`" + `
		}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		hasNext := (page+1)*pageSize < result.TotalCount
		return result.Results, hasNext, nil
{{- else if eq .PaginationStyle "rawArray" }}
		var result []{{ .ItemType }}
		if err := c.transport.Do(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, false, err
		}
		return result, len(result) >= pageSize && len(result) > 0, nil
{{- end }}
	})
}
{{ end }}

{{- define "resolverID" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context, name string) (string, error) {
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	listPath := prefix + "{{ .ResourcePath }}{{ if .Resolver.ExtraParams }}?{{ .Resolver.ExtraParams }}{{ end }}"
{{- if eq .Resolver.Mode "filtered" }}
	id, _, err := c.transport.ResolveByNameFiltered(ctx, listPath, "{{ .Resolver.ResultsField }}", "{{ .Resolver.NameField }}", "{{ .Resolver.MatchField }}", "{{ .Resolver.IDField }}", name)
{{- else if .Resolver.Paginated }}
	id, _, err := c.transport.ResolveByNameClientPaged(ctx, listPath, "{{ .Resolver.SearchParam }}", "{{ .Resolver.ResultsField }}", "{{ .Resolver.NameField }}", "{{ .Resolver.IDField }}", name)
{{- else }}
	id, _, err := c.transport.ResolveByNameClient(ctx, listPath, "{{ .Resolver.SearchParam }}", "{{ .Resolver.ResultsField }}", "{{ .Resolver.NameField }}", "{{ .Resolver.IDField }}", name)
{{- end }}
	if err != nil {
		return "", fmt.Errorf("{{ .Name }}(%s): %w", name, err)
	}
	return id, nil
}
{{ end }}

{{- define "resolverTyped" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context, name string) (*{{ .Resolver.TypedReturn }}, error) {
	prefix := c.transport.TenantPrefix("{{ .Namespace }}", "{{ .Version }}")
	listPath := prefix + "{{ .ResourcePath }}{{ if .Resolver.ExtraParams }}?{{ .Resolver.ExtraParams }}{{ end }}"
{{- if eq .Resolver.Mode "filtered" }}
	_, raw, err := c.transport.ResolveByNameFiltered(ctx, listPath, "{{ .Resolver.ResultsField }}", "{{ .Resolver.NameField }}", "{{ .Resolver.MatchField }}", "{{ .Resolver.IDField }}", name)
{{- else if .Resolver.Paginated }}
	_, raw, err := c.transport.ResolveByNameClientPaged(ctx, listPath, "{{ .Resolver.SearchParam }}", "{{ .Resolver.ResultsField }}", "{{ .Resolver.NameField }}", "{{ .Resolver.IDField }}", name)
{{- else }}
	_, raw, err := c.transport.ResolveByNameClient(ctx, listPath, "{{ .Resolver.SearchParam }}", "{{ .Resolver.ResultsField }}", "{{ .Resolver.NameField }}", "{{ .Resolver.IDField }}", name)
{{- end }}
	if err != nil {
		return nil, fmt.Errorf("{{ .Name }}(%s): %w", name, err)
	}
	var out {{ .Resolver.TypedReturn }}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("{{ .Name }}(%s): decoding matched element: %w", name, err)
	}
	return &out, nil
}
{{ end }}

{{- define "resolverIDDirect" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context, name string) (string, error) {
	r, err := c.{{ .Resolver.SourceMethod }}(ctx, name)
	if err != nil {
		return "", fmt.Errorf("{{ .Name }}(%s): %w", name, err)
	}
	if {{ .Resolver.IDNilCheck }} {
		return "", fmt.Errorf("{{ .Name }}(%s): response missing id", name)
	}
	return strconv.Itoa({{ .Resolver.IDDeref }}), nil
}
{{ end }}

{{- define "resolverTypedDirect" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context, name string) (*{{ .Resolver.TypedReturn }}, error) {
	return c.{{ .Resolver.SourceMethod }}(ctx, name)
}
{{ end }}

{{- define "apply" }}
// {{ .Comment }}
func (c *Client) {{ .Name }}(ctx context.Context, request *{{ .Apply.RequestType }}{{ .Apply.ExtraArgs }}) (string, bool, error) {
{{- if .Apply.NameIsPointer }}
	var name string
	if request.{{ .Apply.NameGoField }} != nil {
		name = *request.{{ .Apply.NameGoField }}
	}
{{- else }}
	name := request.{{ .Apply.NameGoField }}
{{- end }}
	if name == "" {
		return "", false, fmt.Errorf("{{ .Name }}: {{ .Apply.NameGoField }} must not be empty")
	}
	id, err := c.{{ .Apply.ResolverMethod }}(ctx, name)
	if err != nil {
		if apiErr := client.AsAPIError(err); apiErr != nil && apiErr.HasStatus(404) {
{{- if .Apply.ClassicCreate }}
			resp, createErr := c.{{ .Apply.CreateMethod }}(ctx, "0", request)
{{- else }}
			resp, createErr := c.{{ .Apply.CreateMethod }}(ctx, request{{ .Apply.ExtraCallArgs }})
{{- end }}
			if createErr != nil {
				return "", false, fmt.Errorf("{{ .Name }}: create: %w", createErr)
			}
			return {{ .Apply.CreateReturnID }}, true, nil
		}
		return "", false, fmt.Errorf("{{ .Name }}: resolve: %w", err)
	}
{{- if .Apply.UpdateReturnsVal }}
	_, err = c.{{ .Apply.UpdateMethod }}(ctx, id, request)
{{- else }}
	err = c.{{ .Apply.UpdateMethod }}(ctx, id, request)
{{- end }}
	if err != nil {
		return "", false, fmt.Errorf("{{ .Name }}: update(%s): %w", id, err)
	}
	return id, false, nil
}
{{ end }}
`

// ---------------------------------------------------------------------------
// Test template (uses <% %> delimiters to avoid {{ }} conflicts with Go maps)
// ---------------------------------------------------------------------------

var testTemplate = `// Code generated by tools/generate; DO NOT EDIT.

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package <% .Package %>

import (
	"context"
	"net/http"
	"testing"
)

<% range .Methods -%>
<%- if eq .Category "paginated" %>
<% template "testPaginated" . %>
<%- else if eq .Category "unwrap" %>
<% template "testUnwrap" . %>
<%- else if eq .Category "get" %>
<% template "testGet" . %>
<%- else if eq .Category "create" %>
<% template "testCreate" . %>
<%- else if eq .Category "actionWithResponse" %>
<% template "testActionWithResponse" . %>
<%- else if eq .Category "update" %>
<% template "testUpdate" . %>
<%- else if eq .Category "multipart" %>
<% template "testMultipart" . %>
<%- else if eq .Category "raw" %>
<% template "testRaw" . %>
<%- else if eq .Category "resolverID" %>
<% template "testResolverID" . %>
<%- else if eq .Category "resolverTyped" %>
<% template "testResolverTyped" . %>
<%- else if eq .Category "resolverIDDirect" %>
<% template "testResolverIDDirect" . %>
<%- else if eq .Category "resolverTypedDirect" %>
<% template "testResolverTypedDirect" . %>
<%- else if eq .Category "apply" %>
<% template "testApply" . %>
<%- else %>
<% template "testAction" . %>
<%- end %>
<% end %>

<%- define "testGet" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
		<%- if eq .Format "xml" %>
		writeXML(t, w, http.StatusOK, "<<% .ResponseWireName %>></<% .ResponseWireName %>>")
		<%- else if .ReturnsSlice %>
		writeJSON(t, w, http.StatusOK, []map[string]any{{}})
		<%- else %>
		writeJSON(t, w, http.StatusOK, map[string]any{})
		<%- end %>
	})

	result, err := c.<% .Name %>(context.Background()<% testCallArgs . %><% testExtraArgs . %>)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func Test<% .Name %>_NotFound(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, _ *http.Request) {
		<%- if eq .Format "xml" %>
		writeXML(t, w, http.StatusNotFound, "<error>not found</error>")
		<%- else %>
		writeJSON(t, w, http.StatusNotFound, map[string]any{
			"httpStatus": 404,
			"traceId":    "trace-nf",
			"errors":     []map[string]string{{"code": "NOT_FOUND", "field": "id", "description": "not found"}},
		})
		<%- end %>
	})

	_, err := c.<% .Name %>(context.Background()<% testCallArgs . %><% testExtraArgs . %>)
	if err == nil {
		t.Fatal("expected error")
	}
}
<% end %>

<%- define "testCreate" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
<%- if eq .Format "xml" %>
		writeXML(t, w, <% statusConst .ExpectedStatus %>, "<<% .ResponseWireName %>></<% .ResponseWireName %>>")
<%- else if .ReturnsSlice %>
		writeJSON(t, w, <% statusConst .ExpectedStatus %>, []map[string]any{{}})
<%- else %>
		writeJSON(t, w, <% statusConst .ExpectedStatus %>, map[string]any{})
<%- end %>
	})

	result, err := c.<% .Name %>(context.Background()<% testCallArgs . %>, <% requestArg .RequestType %><% testExtraArgs . %>)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
<% end %>

<%- define "testUpdate" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
		w.WriteHeader(<% statusConst .ExpectedStatus %>)
	})

	err := c.<% .Name %>(context.Background()<% testCallArgs . %>, <% requestArg .RequestType %><% testExtraArgs . %>)
	if err != nil {
		t.Fatal(err)
	}
}
<% end %>

<%- define "testAction" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
		w.WriteHeader(<% statusConst .ExpectedStatus %>)
	})

	err := c.<% .Name %>(context.Background()<% testCallArgs . %><% testExtraArgs . %>)
	if err != nil {
		t.Fatal(err)
	}
}
<% end %>

<%- define "testMultipart" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("Content-Type = %q, want multipart/form-data", ct)
		}
		<%- if and .ResponseType .ReturnsSlice %>
		writeJSON(t, w, <% statusConst .ExpectedStatus %>, []map[string]any{{}})
		<%- else if .ResponseType %>
		writeJSON(t, w, <% statusConst .ExpectedStatus %>, map[string]any{})
		<%- else %>
		w.WriteHeader(<% statusConst .ExpectedStatus %>)
		<%- end %>
	})

	<%- if .ResponseType %>
	result, err := c.<% .Name %>(context.Background()<% testCallArgs . %><% testMultipartArgs . %>)
	<%- else %>
	err := c.<% .Name %>(context.Background()<% testCallArgs . %><% testMultipartArgs . %>)
	<%- end %>
	if err != nil {
		t.Fatal(err)
	}
	<%- if .ResponseType %>
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	<%- end %>
}
<% end %>

<%- define "testRaw" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
		<%- if .ResponseType %>
		w.WriteHeader(<% statusConst .ExpectedStatus %>)
		_, _ = w.Write([]byte("<ok/>"))
		<%- else %>
		w.WriteHeader(<% statusConst .ExpectedStatus %>)
		<%- end %>
	})

	<%- if and .RequestType .ResponseType %>
	result, err := c.<% .Name %>(context.Background()<% testCallArgs . %>, []byte("<in/>"))
	<%- else if .RequestType %>
	err := c.<% .Name %>(context.Background()<% testCallArgs . %>, []byte("<in/>"))
	<%- else if .ResponseType %>
	result, err := c.<% .Name %>(context.Background()<% testCallArgs . %>)
	<%- else %>
	err := c.<% .Name %>(context.Background()<% testCallArgs . %>)
	<%- end %>
	if err != nil {
		t.Fatal(err)
	}
	<%- if .ResponseType %>
	if len(result) == 0 {
		t.Fatal("expected non-empty result body")
	}
	<%- end %>
}
<% end %>

<%- define "testUnwrap" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
<%- if isStringSlice .UnwrapResults %>
		writeJSON(t, w, http.StatusOK, map[string]any{"totalCount": 1, "results": []string{"item-1"}})
<%- else %>
		writeJSON(t, w, http.StatusOK, map[string]any{"totalCount": 1, "results": []map[string]any{{"id": "item-1"}}})
<%- end %>
	})

	results, err := c.<% .Name %>(context.Background()<% testCallArgs . %><% testExtraArgs . %>)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
}
<% end %>

<%- define "testActionWithResponse" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
<%- if .ReturnsSlice %>
		writeJSON(t, w, <% statusConst .ExpectedStatus %>, []map[string]any{{}})
<%- else %>
		writeJSON(t, w, <% statusConst .ExpectedStatus %>, map[string]any{})
<%- end %>
	})

	result, err := c.<% .Name %>(context.Background()<% testCallArgs . %><% testExtraArgs . %>)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
<% end %>

<%- define "testPaginated" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != <% httpConst .HTTPMethod %> {
			t.Errorf("method = %s, want <% .HTTPMethod %>", r.Method)
		}
<%- if eq .PaginationStyle "rawArray" %>
		writeJSON(t, w, http.StatusOK, []map[string]any{{}})
<%- else %>
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results":    []map[string]any{{}},
			"totalCount": 1,
			"hasNext":    false,
		})
<%- end %>
	})

	results, err := c.<% .Name %>(context.Background()<% testCallArgs . %><% testExtraArgs . %>)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
}
<% end %>

<%- define "testResolverID" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
<%- if .Resolver.ResultsField %>
			"<% .Resolver.ResultsField %>": []map[string]any{
				{<% resolverIDStub .Resolver %>, <% resolverNameStub .Resolver.MatchField %>},
			},
<%- else %>
			"results": []map[string]any{
				{<% resolverIDStub .Resolver %>, <% resolverNameStub .Resolver.MatchField %>},
			},
			"totalCount": 1,
<%- end %>
		})
	})

	id, err := c.<% .Name %>(context.Background(), "target")
	if err != nil {
		t.Fatal(err)
	}
	if id != "<% resolverExpectedID .Resolver %>" {
		t.Errorf("id = %q, want <% resolverExpectedID .Resolver %>", id)
	}
}
<% end %>

<%- define "testResolverTyped" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
<%- if .Resolver.ResultsField %>
			"<% .Resolver.ResultsField %>": []map[string]any{
				{<% resolverIDStub .Resolver %>, <% resolverNameStub .Resolver.MatchField %>},
			},
<%- else %>
			"results": []map[string]any{
				{<% resolverIDStub .Resolver %>, <% resolverNameStub .Resolver.MatchField %>},
			},
			"totalCount": 1,
<%- end %>
		})
	})

	result, err := c.<% .Name %>(context.Background(), "target")
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
<% end %>

<%- define "testResolverIDDirect" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		writeXML(t, w, http.StatusOK, "<<% .ResponseWireName %>><% .Resolver.IDTestInnerXML %></<% .ResponseWireName %>>")
	})

	id, err := c.<% .Name %>(context.Background(), "test-id")
	if err != nil {
		t.Fatal(err)
	}
	if id != "42" {
		t.Errorf("id = %q, want 42", id)
	}
}
<% end %>

<%- define "testResolverTypedDirect" %>
func Test<% .Name %>(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	mux.HandleFunc("<% testPath . %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		writeXML(t, w, http.StatusOK, "<<% .ResponseWireName %>><% .Resolver.IDTestInnerXML %></<% .ResponseWireName %>>")
	})

	result, err := c.<% .Name %>(context.Background(), "test-id")
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
<% end %>

<%- define "testApply" %>
func Test<% .Name %>_Create(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
<%- if .Apply.SameListCreatePath %>
	// List and create share the same path — single handler dispatches on method.
	mux.HandleFunc("<% applyListPath .Apply %>", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(t, w, http.StatusOK, map[string]any{
				"results":    []any{},
				"totalCount": 0,
			})
		case http.MethodPost:
<%- if .Apply.ClassicCreate %>
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(<% applyCreateStatus .Apply %>)
			_, _ = fmt.Fprint(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?><id>42</id>")
<%- else %>
			writeJSON(t, w, <% applyCreateStatus .Apply %>, map[string]any{
				"id":   <% applyTestCreateIDJSON .Apply %>,
				"href": "/new-id",
			})
<%- end %>
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})
<%- else %>
	// List returns no matches → resolver returns 404 → apply creates.
	mux.HandleFunc("<% applyListPath .Apply %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results":    []any{},
			"totalCount": 0,
		})
	})
<%- if .Apply.ClassicCreate %>
	mux.HandleFunc("<% applyCreatePath .Apply %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(<% applyCreateStatus .Apply %>)
		_, _ = fmt.Fprint(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?><id>42</id>")
	})
<%- else %>
	mux.HandleFunc("<% applyCreatePath .Apply %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		writeJSON(t, w, <% applyCreateStatus .Apply %>, map[string]any{
			"id":   <% applyTestCreateIDJSON .Apply %>,
			"href": "/new-id",
		})
	})
<%- end %>
<%- end %>

	id, created, err := c.<% .Name %>(context.Background(), <% applyRequestExpr .Apply %><% .Apply.ExtraTestCallArgs %>)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Error("expected created = true")
	}
<%- if .Apply.ClassicCreate %>
	if id != "42" {
		t.Errorf("id = %q, want 42", id)
	}
<%- else %>
	if id != "<% applyTestCreateIDExpected .Apply %>" {
		t.Errorf("id = %q, want <% applyTestCreateIDExpected .Apply %>", id)
	}
<%- end %>
}

func Test<% .Name %>_Update(t *testing.T) {
	c, mux := testServerWithOpts(t, WithTenantID("t-test"))
	// List returns a match → resolver succeeds → apply updates.
	mux.HandleFunc("<% applyListPath .Apply %>", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"results": []map[string]any{
				{"<% .Apply.ListIDField %>": "existing-id", "<% .Apply.ListNameField %>": "target"},
			},
			"totalCount": 1,
		})
	})
	mux.HandleFunc("<% applyUpdatePath .Apply %>", func(w http.ResponseWriter, r *http.Request) {
<%- if .Apply.UpdateReturnsVal %>
<%- if .Apply.IDNumeric %>
		writeJSON(t, w, <% applyUpdateStatus .Apply %>, map[string]any{"id": 99})
<%- else %>
		writeJSON(t, w, <% applyUpdateStatus .Apply %>, map[string]any{"id": "existing-id"})
<%- end %>
<%- else %>
		w.WriteHeader(<% applyUpdateStatus .Apply %>)
<%- end %>
	})

	id, created, err := c.<% .Name %>(context.Background(), <% applyRequestExpr .Apply %><% .Apply.ExtraTestCallArgs %>)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("expected created = false")
	}
	if id != "existing-id" {
		t.Errorf("id = %q, want existing-id", id)
	}
}
<% end %>
`
