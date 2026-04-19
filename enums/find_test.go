package enums

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseSource(t *testing.T, source string) (*token.FileSet, *ast.Package, *ast.File) {
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "test.go", source, parser.ParseComments)
	require.NoError(t, err)

	pkg := &ast.Package{
		Name:  astFile.Name.Name,
		Files: map[string]*ast.File{"test.go": astFile},
	}

	return fset, pkg, astFile
}

func TestFind_BasicStringEnum(t *testing.T) {
	source := `package example

// Status represents order status
type Status string //#enum

const (
	StatusPending   Status = "pending"
	StatusConfirmed Status = "confirmed"
	StatusShipped   Status = "shipped"
	StatusDelivered Status = "delivered"
	StatusCancelled Status = "cancelled"
)`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)

	require.NoError(t, err)
	require.Len(t, enums, 1)
	require.Contains(t, enums, "Status")

	e := enums["Status"]
	assert.Equal(t, "example", e.Package)
	assert.Equal(t, "Status", e.Type)
	assert.Equal(t, "string", e.Underlying)
	assert.Equal(t, "s", e.Recv)
	assert.False(t, e.JSONSchema)
	assert.Empty(t, e.Null)

	assert.Equal(t, []string{"StatusPending", "StatusConfirmed", "StatusShipped", "StatusDelivered", "StatusCancelled"}, e.Enums)
	assert.Equal(t, []string{`"pending"`, `"confirmed"`, `"shipped"`, `"delivered"`, `"cancelled"`}, e.Literals)
	assert.Equal(t, []string{`"pending"`, `"confirmed"`, `"shipped"`, `"delivered"`, `"cancelled"`}, e.JSONSchemaEnum)

	assert.True(t, e.IsStringType())
	assert.False(t, e.IsIntType())
	assert.False(t, e.IsNullable())
}

func TestFind_BasicIntEnum(t *testing.T) {
	source := `package example

type Priority int //#enum

const (
	PriorityLow  Priority = 1
	PriorityMid  Priority = 2
	PriorityHigh Priority = 3
)`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)

	require.NoError(t, err)
	require.Len(t, enums, 1)
	require.Contains(t, enums, "Priority")

	e := enums["Priority"]
	assert.Equal(t, "example", e.Package)
	assert.Equal(t, "Priority", e.Type)
	assert.Equal(t, "int", e.Underlying)
	assert.Equal(t, "p", e.Recv)
	assert.False(t, e.JSONSchema)
	assert.Empty(t, e.Null)

	assert.Equal(t, []string{"PriorityLow", "PriorityMid", "PriorityHigh"}, e.Enums)
	assert.Equal(t, []string{"1", "2", "3"}, e.Literals)
	assert.Equal(t, []string{"1", "2", "3"}, e.JSONSchemaEnum)

	assert.False(t, e.IsStringType())
	assert.True(t, e.IsIntType())
	assert.False(t, e.IsNullable())
}

func TestFind_NullableIntEnum(t *testing.T) {
	source := `package example

type Priority int //#enum

const (
	PriorityNull Priority = 0 //#null
	PriorityLow  Priority = 1
	PriorityMid  Priority = 2
	PriorityHigh Priority = 3
)`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)

	require.NoError(t, err)
	require.Len(t, enums, 1)

	e := enums["Priority"]
	assert.Equal(t, "PriorityNull", e.Null)
	assert.True(t, e.IsNullable())

	assert.Equal(t, []string{"PriorityNull", "PriorityLow", "PriorityMid", "PriorityHigh"}, e.Enums)
	assert.Equal(t, []string{"0", "1", "2", "3"}, e.Literals)
	// Null value should not be in JSONSchemaEnum
	assert.Equal(t, []string{"1", "2", "3"}, e.JSONSchemaEnum)
}

func TestFind_JSONSchemaFlag(t *testing.T) {
	source := `package example

type Color string //#enum,jsonschema

const (
	ColorRed   Color = "red"
	ColorGreen Color = "green"
	ColorBlue  Color = "blue"
)`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)

	require.NoError(t, err)
	require.Len(t, enums, 1)

	e := enums["Color"]
	assert.True(t, e.JSONSchema)
}

func TestFind_JSONSchemaWithWhitespace(t *testing.T) {
	source := `package example

type Color string //#enum, jsonschema

const (
	ColorRed Color = "red"
)`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)

	require.NoError(t, err)
	require.Len(t, enums, 1)

	e := enums["Color"]
	assert.True(t, e.JSONSchema)
}

func TestFind_MultipleEnums(t *testing.T) {
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)

type Priority int //#enum

const (
	PriorityLow  Priority = 1
	PriorityHigh Priority = 2
)`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)

	require.NoError(t, err)
	require.Len(t, enums, 2)
	require.Contains(t, enums, "Status")
	require.Contains(t, enums, "Priority")

	assert.Equal(t, "string", enums["Status"].Underlying)
	assert.Equal(t, "int", enums["Priority"].Underlying)
}

func TestFind_ExistingMethods(t *testing.T) {
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)

func (s Status) Valid() bool {
	return true
}

func (s Status) String() string {
	return string(s)
}`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)

	require.NoError(t, err)
	require.Len(t, enums, 1)

	e := enums["Status"]
	assert.Equal(t, "s", e.Recv)
	require.Len(t, e.KnownMethods, 2)

	methodNames := []string{e.KnownMethods[0].Name.Name, e.KnownMethods[1].Name.Name}
	assert.Contains(t, methodNames, "Valid")
	assert.Contains(t, methodNames, "String")
}

func TestFind_ExistingNullableMethods(t *testing.T) {
	source := `package example

type Priority int //#enum

const (
	PriorityNull Priority = 0 //#null
	PriorityLow  Priority = 1
)

func (p Priority) IsNull() bool {
	return p == PriorityNull
}

func (p Priority) MarshalJSON() ([]byte, error) {
	return nil, nil
}`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)

	require.NoError(t, err)

	e := enums["Priority"]
	require.Len(t, e.KnownMethods, 2)

	methodNames := []string{e.KnownMethods[0].Name.Name, e.KnownMethods[1].Name.Name}
	assert.Contains(t, methodNames, "IsNull")
	assert.Contains(t, methodNames, "MarshalJSON")
}

func TestFind_NoEnums(t *testing.T) {
	source := `package example

type Status string

const (
	StatusPending Status = "pending"
)`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)

	require.NoError(t, err)
	assert.Nil(t, enums)
}

func TestFind_EnumWithoutValues(t *testing.T) {
	source := `package example

type Status string //#enum`

	fset, pkg, astFile := parseSource(t, source)
	_, err := Find(fset, pkg, astFile)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no typed const enum values")
}

func TestFind_DuplicateNullValue(t *testing.T) {
	source := `package example

type Priority int //#enum

const (
	PriorityNull Priority = 0 //#null
	PriorityUnset Priority = -1 //#null
	PriorityLow  Priority = 1
)`

	fset, pkg, astFile := parseSource(t, source)
	_, err := Find(fset, pkg, astFile)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "second //#null enum encountered")
}

func TestFind_DuplicateEnumNames(t *testing.T) {
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusPending Status = "other"
)`

	fset, pkg, astFile := parseSource(t, source)
	_, err := Find(fset, pkg, astFile)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate enum name")
}

func TestFind_DuplicateLiteralValues(t *testing.T) {
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusOther   Status = "pending"
)`

	fset, pkg, astFile := parseSource(t, source)
	_, err := Find(fset, pkg, astFile)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate enum value")
}

func TestFind_InvalidPackage(t *testing.T) {
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
)`

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "test.go", source, parser.ParseComments)
	require.NoError(t, err)

	// Pass nil package
	_, err = Find(fset, nil, astFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or missing package name")

	// Pass package with empty name
	pkg := &ast.Package{Name: ""}
	_, err = Find(fset, pkg, astFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or missing package name")
}

func TestFind_CustomUintTypes(t *testing.T) {
	source := `package example

type Level uint8 //#enum

const (
	LevelLow  Level = 1
	LevelHigh Level = 2
)`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)

	require.NoError(t, err)

	e := enums["Level"]
	assert.Equal(t, "uint8", e.Underlying)
	assert.True(t, e.IsIntType())
}

func TestFind_ByteType(t *testing.T) {
	source := `package example

type Code byte //#enum

const (
	CodeA Code = 'A'
	CodeB Code = 'B'
)`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)

	require.NoError(t, err)

	e := enums["Code"]
	assert.Equal(t, "byte", e.Underlying)
	assert.True(t, e.IsIntType())
}

func TestEnum_JSONType(t *testing.T) {
	tests := []struct {
		underlying string
		want       string
	}{
		{"string", "string"},
		{"int", "number"},
		{"int8", "number"},
		{"int16", "number"},
		{"int32", "number"},
		{"int64", "number"},
		{"uint", "number"},
		{"uint8", "number"},
		{"uint16", "number"},
		{"uint32", "number"},
		{"uint64", "number"},
		{"byte", "number"},
		{"float32", "number"},
		{"float64", "number"},
		{"bool", "boolean"},
		{"[]string", "array"},
		{"map[string]int", "object"},
		{"struct{}", "object"},
		{"time.Time", "string"},
		{"custom", "string"},
	}

	for _, tt := range tests {
		t.Run(tt.underlying, func(t *testing.T) {
			e := &Enum{Underlying: tt.underlying}
			assert.Equal(t, tt.want, e.JSONType())
		})
	}
}

func TestEnum_LastIndex(t *testing.T) {
	e := &Enum{
		Enums: []string{"One", "Two", "Three"},
	}
	assert.Equal(t, 2, e.LastIndex())
}

func TestFind_PointerReceiverMethods(t *testing.T) {
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
)

func (s *Status) Valid() bool {
	return true
}`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)

	require.NoError(t, err)

	e := enums["Status"]
	assert.Equal(t, "s", e.Recv)
	require.Len(t, e.KnownMethods, 1)
	assert.Equal(t, "Valid", e.KnownMethods[0].Name.Name)
}

func TestFind_NonStringNonIntJSONSchemaEnum(t *testing.T) {
	source := `package example

type Size int32 //#enum

const (
	SizeSmall Size = 1
	SizeLarge Size = 2
)`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)

	require.NoError(t, err)

	e := enums["Size"]
	// For non-default types, should have type cast in JSONSchemaEnum
	assert.Equal(t, []string{"int32(1)", "int32(2)"}, e.JSONSchemaEnum)
}

func TestFind_CustomMarker(t *testing.T) {
	source := `package example

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "ACTIVE"
)

// UnmarshalJSON parses legacy bool forms.
//
//#custom
func (s *Status) UnmarshalJSON(data []byte) error {
	return nil
}

// MarshalJSON is the regular regeneration-target variant (no //#custom).
func (s Status) MarshalJSON() ([]byte, error) {
	return nil, nil
}`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)
	require.NoError(t, err)
	require.Contains(t, enums, "Status")

	e := enums["Status"]

	// UnmarshalJSON is custom-marked: present in CustomMethods, absent from KnownMethods.
	assert.True(t, e.CustomMethods["UnmarshalJSON"], "UnmarshalJSON must be recognized as custom")

	for _, km := range e.KnownMethods {
		assert.NotEqual(t, "UnmarshalJSON", km.Name.Name,
			"custom-marked UnmarshalJSON must not be in KnownMethods")
	}

	// MarshalJSON has no //#custom: it's a KnownMethod (replaceable) and
	// does NOT appear in CustomMethods.
	assert.False(t, e.CustomMethods["MarshalJSON"], "MarshalJSON has no //#custom, so it must not be custom")
	hasMarshalKnown := false
	for _, km := range e.KnownMethods {
		if km.Name.Name == "MarshalJSON" {
			hasMarshalKnown = true
			break
		}
	}
	assert.True(t, hasMarshalKnown, "non-custom MarshalJSON must be in KnownMethods")
}

func TestFind_CustomMarkerOnAlwaysGeneratedMethods(t *testing.T) {
	// Valid, Validate, Enums, EnumStrings are generated for every enum,
	// regardless of string/nullable/jsonschema flags. When marked //#custom
	// they must be tracked in CustomMethods and kept out of KnownMethods,
	// so the rewriter can skip the matching template and avoid duplicates.
	source := `package example

type Status string //#enum

const (
	StatusActive Status = "ACTIVE"
)

//#custom
func (s Status) Valid() bool {
	return s != ""
}

//#custom
func (s Status) Validate() error {
	return nil
}

//#custom
func (Status) Enums() []Status {
	return []Status{StatusActive}
}

//#custom
func (Status) EnumStrings() []string {
	return []string{"active"}
}`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)
	require.NoError(t, err)
	require.Contains(t, enums, "Status")

	e := enums["Status"]

	for _, name := range []string{"Valid", "Validate", "Enums", "EnumStrings"} {
		assert.True(t, e.CustomMethods[name], "%s must be recognized as custom", name)
		for _, km := range e.KnownMethods {
			assert.NotEqual(t, name, km.Name.Name,
				"custom-marked %s must not be in KnownMethods", name)
		}
	}
}

func TestFind_CustomMarkerIgnoredOnNonGeneratedMethod(t *testing.T) {
	// A //#custom marker on a method that the generator would never produce
	// (e.g. a user helper "Foo") must be a no-op: not tracked as custom,
	// not a known method, no side effects.
	source := `package example

type Status string //#enum

const (
	StatusA Status = "A"
)

//#custom
func (s Status) Foo() string {
	return string(s)
}`

	fset, pkg, astFile := parseSource(t, source)
	enums, err := Find(fset, pkg, astFile)
	require.NoError(t, err)

	e := enums["Status"]
	assert.False(t, e.CustomMethods["Foo"], "//#custom on non-generated method must be ignored")
	for _, km := range e.KnownMethods {
		assert.NotEqual(t, "Foo", km.Name.Name)
	}
}
