package templates

import "text/template"

// ValidateMethods provides the methods: Valid, Validate
var ValidateMethods = template.Must(template.New("").Parse(`
// Valid indicates if {{.Recv}} is any of the valid values for {{.Type}}
func ({{.Recv}} {{.Type}}) Valid() bool {
	switch {{.Recv}} {
	case
		{{$lastIndex := .LastIndex}}{{range $index, $element := .Enums}}{{$element}}{{if lt $index $lastIndex}},
		{{else}}:{{end}}{{end}}
		return true
	}
	return false
}

// Validate returns an error if {{.Recv}} is none of the valid values for {{.Type}}
func ({{.Recv}} {{.Type}}) Validate() error {
	if !{{.Recv}}.Valid() {
		return fmt.Errorf("invalid value %#v for type {{.Package}}.{{.Type}}", {{.Recv}})
	}
	return nil
}
`))

// NullableMethods provides the methods: IsNull, IsNotNull, SetNull, MarshalJSON, UnmarshalJSON
var NullableMethods = template.Must(template.New("").Parse(`
// IsNull returns true if {{.Recv}} is the null value {{.Null}}
func ({{.Recv}} {{.Type}}) IsNull() bool {
	return {{.Recv}} == {{.Null}}
}

// IsNotNull returns true if {{.Recv}} is not the null value {{.Null}}
func ({{.Recv}} {{.Type}}) IsNotNull() bool {
	return {{.Recv}} != {{.Null}}
}

// SetNull sets the null value {{.Null}} at {{.Recv}}
func ({{.Recv}} *{{.Type}}) SetNull() {
	*{{.Recv}} = {{.Null}}
}

// MarshalJSON implements encoding/json.Marshaler for {{.Type}}
// by returning the JSON null value for {{.Null}}.
func ({{.Recv}} {{.Type}}) MarshalJSON() ([]byte, error) {
	if {{.Recv}} == {{.Null}} {
		return []byte("null"), nil
	}
	return json.Marshal({{.Underlying}}({{.Recv}}))
}

// UnmarshalJSON implements encoding/json.Unmarshaler
func ({{.Recv}} *{{.Type}}) UnmarshalJSON(j []byte) error {
	if bytes.Equal(j, []byte("null")) {
		*{{.Recv}} = {{.Null}}
		return nil
	}
	return json.Unmarshal(j, (*{{.Underlying}})({{.Recv}}))
}
`))

// StringMethods provides the methods: String
var StringMethods = template.Must(template.New("").Parse(`
// String implements the fmt.Stringer interface for {{.Type}}
func ({{.Recv}} {{.Type}}) String() string {
	return string({{.Recv}})
}
`))

var EnumsMethods = template.Must(template.New("").Parse(`
// Enums returns all valid values for {{.Type}}
func ({{.Type}}) Enums() []{{.Type}} {
	return []{{.Type}}{
		{{range .Enums}}{{.}},
{{end}}
	}
}

// EnumStrings returns all valid values for {{.Type}} as strings
func ({{.Type}}) EnumStrings() []string {
	return []string{
		{{if .IsStringType}}{{range .Literals}}{{.}},
{{end}}{{else}}{{range .Literals}}"{{.}}",
{{end}}{{end}}
	}
}
`))

// NullableStringMethods provides the methods: Scan, Value
var NullableStringMethods = template.Must(template.New("").Parse(`
// Scan implements the database/sql.Scanner interface for {{.Type}}
func ({{.Recv}} *{{.Type}}) Scan(value any) error {
	switch value := value.(type) {
	case string:
		*{{.Recv}} = {{.Type}}(value)
	case []byte:
		*{{.Recv}} = {{.Type}}(value)
	case nil:
		*{{.Recv}} = {{.Null}}
	default:
		return fmt.Errorf("can't scan SQL value of type %T as {{.Package}}.{{.Type}}", value)
	}
	return nil
}

// Value implements the driver database/sql/driver.Valuer interface for {{.Type}}
func ({{.Recv}} {{.Type}}) Value() (driver.Value, error) {
	if {{.Recv}} == {{.Null}} {
		return nil, nil
	}
	return {{.Underlying}}({{.Recv}}), nil
}
`))

// NullableIntMethods provides the methods: Scan, Value
var NullableIntMethods = template.Must(template.New("").Parse(`
// Scan implements the database/sql.Scanner interface for {{.Type}}
func ({{.Recv}} *{{.Type}}) Scan(value any) error {
	switch value := value.(type) {
	case int64:
		*{{.Recv}} = {{.Type}}(value)
	case float64:
		*{{.Recv}} = {{.Type}}(value)
	case nil:
		*{{.Recv}} = {{.Null}}
	default:
		return fmt.Errorf("can't scan SQL value of type %T as {{.Package}}.{{.Type}}", value)
	}
	return nil
}

// Value implements the driver database/sql/driver.Valuer interface for {{.Type}}
func ({{.Recv}} {{.Type}}) Value() (driver.Value, error) {
	if {{.Recv}} == {{.Null}} {
		return nil, nil
	}
	return int64({{.Recv}}), nil
}
`))

var JSONSchemaMethod = template.Must(template.New("").Parse(`
// JSONSchema implements the jsonschema.Schema interface for {{.Type}}
func ({{.Type}}) JSONSchema() jsonschema.Schema {
	return jsonschema.Schema{
		{{if .IsNullable}}OneOf: []*jsonschema.Schema{
			{
				Type: "{{.JSONType}}",
				Enum: []any{
					{{range .JSONSchemaEnum}}{{.}},
{{end}}},
			},
			{Type: "null"},
		},
		Default: {{.Null}},{{else}}Type: "{{.JSONType}}",
		Enum: []any{
			{{range .JSONSchemaEnum}}{{.}},
{{end}}},{{end}}
	}
}
`))
