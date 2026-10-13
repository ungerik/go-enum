package enums

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"text/template"
)

// generatedMethods are the templates of the generated methods
// in the order they are written. A method has a template per
// underlying type it is generated for.
var generatedMethods = []struct {
	name     string
	tmpl     *template.Template
	generate func(*Enum) bool
}{
	{"Valid", validTemplate, always},
	{"Validate", validateTemplate, always},
	{"Enums", enumsTemplate, always},
	{"EnumStrings", enumStringsTemplate, always},
	{"String", stringMethodsTemplate, (*Enum).IsStringType},
	{"IsNull", isNullTemplate, (*Enum).IsNullable},
	{"IsNotNull", isNotNullTemplate, (*Enum).IsNullable},
	{"SetNull", setNullTemplate, (*Enum).IsNullable},
	{"MarshalJSON", nullableMarshalJSONTemplate, (*Enum).IsNullable},
	{"UnmarshalJSON", nullableUnmarshalJSONTemplate, (*Enum).IsNullable},
	{"Scan", nullableStringScanTemplate, isNullableString},
	{"Scan", nullableIntScanTemplate, isNullableInt},
	{"Value", nullableStringValueTemplate, isNullableString},
	{"Value", nullableIntValueTemplate, isNullableInt},
	{"JSONSchema", jsonSchemaMethodTemplate, func(e *Enum) bool { return e.JSONSchema }},
}

func always(*Enum) bool { return true }

func isNullableString(e *Enum) bool { return e.IsNullable() && e.IsStringType() }

func isNullableInt(e *Enum) bool { return e.IsNullable() && e.IsIntType() }

// methodPattern recognizes a method generated from a template
// with any identifiers in place of the template actions.
type methodPattern struct {
	// doc matches the doc comment lines
	doc []*regexp.Regexp
	// docHasActions is false if the doc comment is the same for all enums
	docHasActions bool
	// body matches the formatted method without doc comment,
	// nil for templates with conditions or loops
	body *regexp.Regexp
}

var (
	actionRegexp = regexp.MustCompile(`\{\{[^}]*\}\}`)
	identPattern = `[\p{L}_][\p{L}\p{N}_]*`
	// typePattern matches the underlying type, like string, j[int] or (string)
	typePattern = `(?:` + identPattern + `(?:\[[^\]\n]*\])?|\([^\n]*?\))`
)

// generatedPatterns are the patterns of the generated methods by method name,
// used to recognize methods that were generated before
var generatedPatterns = func() map[string][]methodPattern {
	const ident = "goenumident"
	patterns := make(map[string][]methodPattern)
	for _, m := range generatedMethods {
		lines := strings.Split(strings.TrimLeft(m.tmpl.Root.String(), "\n"), "\n")
		var p methodPattern
		for len(lines) > 0 && strings.HasPrefix(lines[0], "//") {
			p.docHasActions = p.docHasActions || actionRegexp.MatchString(lines[0])
			pattern := regexp.QuoteMeta(actionRegexp.ReplaceAllString(strings.TrimSuffix(lines[0], "."), ident))
			// With optional period at the end, like added by the godot linter
			p.doc = append(p.doc, regexp.MustCompile("^"+strings.ReplaceAll(pattern, ident, identPattern)+`\.?$`))
			lines = lines[1:]
		}
		// The underlying type is a type expression, not only an identifier
		const typeIdent = "goenumtype"
		body := strings.ReplaceAll(strings.Join(lines, "\n"), "{{.Underlying}}", typeIdent)
		if !strings.Contains(body, "{{range") && !strings.Contains(body, "{{if") {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", "package p\n"+actionRegexp.ReplaceAllString(body, ident), parser.SkipObjectResolution)
			if err != nil {
				panic(err)
			}
			pattern := regexp.QuoteMeta(formatWithoutDoc(fset, file.Decls[0].(*ast.FuncDecl)))
			pattern = strings.ReplaceAll(pattern, typeIdent, typePattern)
			p.body = regexp.MustCompile("^" + strings.ReplaceAll(pattern, ident, identPattern) + "$")
		}
		patterns[m.name] = append(patterns[m.name], p)
	}
	return patterns
}()

// matchesDoc reports whether doc has the text lines of the generated doc
// comment. Empty lines, comments without space after the slashes like
// //#custom or //nolint:gosec, and lines starting with # like a //#nosec G115
// that gofmt rewrites to // #nosec G115, are not compared.
func (p *methodPattern) matchesDoc(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	var lines []string
	for _, c := range doc.List {
		if strings.HasPrefix(c.Text, "// ") && !strings.HasPrefix(strings.TrimSpace(c.Text[2:]), "#") {
			lines = append(lines, c.Text)
		}
	}
	if len(lines) != len(p.doc) {
		return false
	}
	for i, line := range lines {
		if !p.doc[i].MatchString(line) {
			return false
		}
	}
	return true
}

// generates reports whether a method with the name is generated for the enum
// by its underlying type, null value and flags, regardless of //#custom.
func (e *Enum) generates(name string) bool {
	for _, m := range generatedMethods {
		if m.name == name && m.generate(e) {
			return true
		}
	}
	return false
}

// canGenerate reports whether a method with the name is generated for the
// enum with any null value and flags, so that a method that is never
// generated for its underlying type, like String of an integer enum,
// is hand-written also with the doc comment of a generated method.
func (e *Enum) canGenerate(name string) bool {
	variant := *e
	variant.Null = "null"
	variant.JSONSchema = true
	return variant.generates(name)
}

// hasGeneratedDoc reports whether a method has the whole doc comment
// of a generated method with the same name, and also its body if that
// doc comment is the same for all enums, like the common phrase
// "UnmarshalJSON implements encoding/json.Unmarshaler", so that
// a hand-written method with a similar doc comment is not replaced.
func hasGeneratedDoc(fset *token.FileSet, fn *ast.FuncDecl) bool {
	for _, p := range generatedPatterns[fn.Name.Name] {
		if p.matchesDoc(fn.Doc) && (p.docHasActions || p.body != nil && p.body.MatchString(formatWithoutDoc(fset, fn))) {
			return true
		}
	}
	return false
}

// hasGeneratedBody reports whether a method with the doc comment of a
// generated method also has its body with any identifiers, like a method
// generated with a null value that was renamed or deleted since.
func hasGeneratedBody(fset *token.FileSet, fn *ast.FuncDecl) bool {
	for _, p := range generatedPatterns[fn.Name.Name] {
		if p.matchesDoc(fn.Doc) && p.body != nil && p.body.MatchString(formatWithoutDoc(fset, fn)) {
			return true
		}
	}
	return false
}

// wasGenerated reports whether a method that is no longer generated
// equals the method generated for the enum with one of its values as
// null value or with the jsonschema flag, so that methods generated
// before //#null or ,jsonschema was removed can be removed without
// removing a hand-written method. Doc comments of generated methods
// like "UnmarshalJSON implements encoding/json.Unmarshaler" are common
// phrases, so a hand-written method can have the same doc comment.
func (e *Enum) wasGenerated(fset *token.FileSet, fn *ast.FuncDecl) bool {
	existing := formatWithoutDoc(fset, fn)
	if existing == "" {
		return false
	}
	// Generated without or with a value as null value, which the code of
	// every method generated for a null value contains
	nulls := []string{""}
	for _, v := range e.Values {
		if strings.Contains(existing, v.Name) {
			nulls = append(nulls, v.Name)
		}
	}
	for _, null := range nulls {
		variant := *e
		variant.JSONSchema = true
		variant.Null = null
		variant.Values = make([]*Value, len(e.Values))
		for i, v := range e.Values {
			value := *v
			value.Null = v.Name == null
			variant.Values[i] = &value
		}
		if names := fn.Recv.List[0].Names; len(names) > 0 {
			variant.Recv = names[0].Name
		}
		for _, m := range generatedMethods {
			if m.name != fn.Name.Name {
				continue
			}
			var source bytes.Buffer
			source.WriteString("package p\n")
			if m.tmpl.Execute(&source, &variant) != nil {
				continue
			}
			genFset := token.NewFileSet()
			file, err := parser.ParseFile(genFset, "", source.Bytes(), parser.SkipObjectResolution)
			if err == nil && len(file.Decls) == 1 && formatWithoutDoc(genFset, file.Decls[0].(*ast.FuncDecl)) == existing {
				return true
			}
		}
	}
	return false
}

// formatWithoutDoc returns the formatted function declaration without its doc comment.
func formatWithoutDoc(fset *token.FileSet, fn *ast.FuncDecl) string {
	withoutDoc := *fn
	withoutDoc.Doc = nil
	var formatted strings.Builder
	if format.Node(&formatted, fset, &withoutDoc) != nil {
		return ""
	}
	return formatted.String()
}

// Valid + Validate templates, split per method so `//#custom` can target
// each individually. Generated for all enum types.

var validTemplate = template.Must(template.New("").Parse(`
// Valid indicates if {{.Recv}} is any of the valid values for {{.Type}}
func ({{.Recv}} {{.Type}}) Valid() bool {
	switch {{.Recv}} {
	case
		{{$lastIndex := .LastIndex}}{{range $index, $element := .Values}}{{$element.Name}}{{if lt $index $lastIndex}},
		{{else}}:{{end}}{{end}}
		return true
	}
	return false
}
`))

var validateTemplate = template.Must(template.New("").Parse(`
// Validate returns an error if {{.Recv}} is none of the valid values for {{.Type}}
func ({{.Recv}} {{.Type}}) Validate() error {
	if !{{.Recv}}.Valid() {
		return fmt.Errorf("invalid value %#v for type {{.Package}}.{{.Type}}", {{.Recv}})
	}
	return nil
}
`))

// Nullable methods — one template per method so callers can skip
// individual methods that are marked `//#custom` on the enum.
// Generated for enum types with a null value (marked with //#null).

var isNullTemplate = template.Must(template.New("").Parse(`
// IsNull returns true if {{.Recv}} is the null value {{.Null}}
func ({{.Recv}} {{.Type}}) IsNull() bool {
	return {{.Recv}} == {{.Null}}
}
`))

var isNotNullTemplate = template.Must(template.New("").Parse(`
// IsNotNull returns true if {{.Recv}} is not the null value {{.Null}}
func ({{.Recv}} {{.Type}}) IsNotNull() bool {
	return {{.Recv}} != {{.Null}}
}
`))

var setNullTemplate = template.Must(template.New("").Parse(`
// SetNull sets the null value {{.Null}} at {{.Recv}}
func ({{.Recv}} *{{.Type}}) SetNull() {
	*{{.Recv}} = {{.Null}}
}
`))

var nullableMarshalJSONTemplate = template.Must(template.New("").Parse(`
// MarshalJSON implements encoding/json.Marshaler for {{.Type}}
// by returning the JSON null value for {{.Null}}.
func ({{.Recv}} {{.Type}}) MarshalJSON() ([]byte, error) {
	if {{.Recv}} == {{.Null}} {
		return []byte("null"), nil
	}
	return json.Marshal({{.Underlying}}({{.Recv}}))
}
`))

var nullableUnmarshalJSONTemplate = template.Must(template.New("").Parse(`
// UnmarshalJSON implements encoding/json.Unmarshaler
func ({{.Recv}} *{{.Type}}) UnmarshalJSON({{.Param "j" "data"}} []byte) error {
	if bytes.Equal({{.Param "j" "data"}}, []byte("null")) {
		*{{.Recv}} = {{.Null}}
		return nil
	}
	return json.Unmarshal({{.Param "j" "data"}}, (*{{.Underlying}})({{.Recv}}))
}
`))

// stringMethodsTemplate provides the String method that implements fmt.Stringer.
// Generated only for string-based enum types.
var stringMethodsTemplate = template.Must(template.New("").Parse(`
// String implements the fmt.Stringer interface for {{.Type}}
func ({{.Recv}} {{.Type}}) String() string {
	return string({{.Recv}})
}
`))

// Enums + EnumStrings templates, split per method so `//#custom` can
// target each individually. These methods return all valid enum values
// as a slice.

var enumsTemplate = template.Must(template.New("").Parse(`
// Enums returns all valid values for {{.Type}}
func ({{.Type}}) Enums() []{{.Type}} {
	return []{{.Type}}{
		{{range .Values}}{{.Name}},
{{end}}
	}
}
`))

var enumStringsTemplate = template.Must(template.New("").Parse(`
// EnumStrings returns all valid values for {{.Type}} as strings
func ({{.Type}}) EnumStrings() []string {
	return []string{
		{{range .Values}}{{.StringExpr}},
{{end}}
	}
}
`))

// Scan + Value templates per underlying type, split per method so
// `//#custom` can target Scan or Value individually.

var nullableStringScanTemplate = template.Must(template.New("").Parse(`
// Scan implements the database/sql.Scanner interface for {{.Type}}
func ({{.Recv}} *{{.Type}}) Scan({{.Param "value" "v"}} any) error {
	switch {{.Param "value" "v"}} := {{.Param "value" "v"}}.(type) {
	case string:
		*{{.Recv}} = {{.Type}}({{.Param "value" "v"}})
	case []byte:
		*{{.Recv}} = {{.Type}}({{.Param "value" "v"}})
	case nil:
		*{{.Recv}} = {{.Null}}
	default:
		return fmt.Errorf("can't scan SQL value of type %T as {{.Package}}.{{.Type}}", {{.Param "value" "v"}})
	}
	return nil
}
`))

var nullableStringValueTemplate = template.Must(template.New("").Parse(`
// Value implements the driver database/sql/driver.Valuer interface for {{.Type}}
func ({{.Recv}} {{.Type}}) Value() (driver.Value, error) {
	if {{.Recv}} == {{.Null}} {
		return nil, nil
	}
	return string({{.Recv}}), nil
}
`))

var nullableIntScanTemplate = template.Must(template.New("").Parse(`
// Scan implements the database/sql.Scanner interface for {{.Type}}
func ({{.Recv}} *{{.Type}}) Scan({{.Param "value" "v"}} any) error {
	switch {{.Param "value" "v"}} := {{.Param "value" "v"}}.(type) {
	case int64:
		*{{.Recv}} = {{.Type}}({{.Param "value" "v"}})
	case float64:
		*{{.Recv}} = {{.Type}}({{.Param "value" "v"}})
	case nil:
		*{{.Recv}} = {{.Null}}
	default:
		return fmt.Errorf("can't scan SQL value of type %T as {{.Package}}.{{.Type}}", {{.Param "value" "v"}})
	}
	return nil
}
`))

var nullableIntValueTemplate = template.Must(template.New("").Parse(`
// Value implements the driver database/sql/driver.Valuer interface for {{.Type}}
func ({{.Recv}} {{.Type}}) Value() (driver.Value, error) {
	if {{.Recv}} == {{.Null}} {
		return nil, nil
	}
	return int64({{.Recv}}), nil
}
`))

// jsonSchemaMethodTemplate provides the JSONSchema method for generating JSON Schema definitions.
// Generated for enum types with the ,jsonschema flag.
// Supports both nullable and non-nullable enums.
var jsonSchemaMethodTemplate = template.Must(template.New("").Parse(`
// JSONSchema returns a github.com/invopop/jsonschema.Schema for {{.Type}}
func ({{.Type}}) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		{{if .IsNullable}}OneOf: []*jsonschema.Schema{
			{
				Type: "{{.JSONType}}",
				Enum: []any{
					{{range .Values}}{{if not .Null}}{{.JSONExpr}},
{{end}}{{end}}},
			},
			{Type: "null"},
		},
		Default: {{.Null}},{{else}}Type: "{{.JSONType}}",
		Enum: []any{
			{{range .Values}}{{if not .Null}}{{.JSONExpr}},
{{end}}{{end}}},{{end}}
	}
}
`))
