package enums

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseSource(t *testing.T, source string) (*token.FileSet, map[string]*ast.File, *ast.File) {
	fset, files := parseFiles(t, map[string]string{"test.go": source})
	return fset, files, files["test.go"]
}

func parseFiles(t *testing.T, sources map[string]string) (*token.FileSet, map[string]*ast.File) {
	fset := token.NewFileSet()
	files := make(map[string]*ast.File)
	for fileName, source := range sources {
		astFile, err := parser.ParseFile(fset, fileName, source, parser.ParseComments)
		require.NoError(t, err)
		files[fileName] = astFile
	}
	return fset, files
}

// findInFile returns the enums of the package declared in astFile by type name
func findInFile(fset *token.FileSet, files map[string]*ast.File, astFile *ast.File) (map[string]*Enum, error) {
	enums, err := Find(fset, files)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*Enum)
	for _, enum := range enums {
		if enum.File == fset.Position(astFile.Pos()).Filename {
			result[enum.Type] = enum
		}
	}
	return result, nil
}

func valueNames(e *Enum) []string {
	var names []string
	for _, v := range e.Values {
		names = append(names, v.Name)
	}
	return names
}

func stringExprs(e *Enum) []string {
	var exprs []string
	for _, v := range e.Values {
		exprs = append(exprs, v.StringExpr)
	}
	return exprs
}

func jsonSchemaEnum(e *Enum) []string {
	var exprs []string
	for _, v := range e.Values {
		if !v.Null {
			exprs = append(exprs, v.JSONExpr)
		}
	}
	return exprs
}

func methodNames(methods []*ast.FuncDecl) []string {
	var names []string
	for _, m := range methods {
		names = append(names, m.Name.Name)
	}
	return names
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

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)

	require.NoError(t, err)
	require.Len(t, enums, 1)
	require.Contains(t, enums, "Status")

	e := enums["Status"]
	assert.Equal(t, "test.go", e.File, "File must be the file name, not the package name")
	assert.Equal(t, 4, e.Line)
	assert.Equal(t, "example", e.Package)
	assert.Equal(t, "Status", e.Type)
	assert.Equal(t, "string", e.Underlying)
	assert.Equal(t, "s", e.Recv)
	assert.False(t, e.JSONSchema)
	assert.Empty(t, e.Null)

	assert.Equal(t, []string{"StatusPending", "StatusConfirmed", "StatusShipped", "StatusDelivered", "StatusCancelled"}, valueNames(e))
	assert.Equal(t, []string{`"pending"`, `"confirmed"`, `"shipped"`, `"delivered"`, `"cancelled"`}, stringExprs(e))
	assert.Equal(t, []string{`"pending"`, `"confirmed"`, `"shipped"`, `"delivered"`, `"cancelled"`}, jsonSchemaEnum(e))

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

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)

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

	assert.Equal(t, []string{"PriorityLow", "PriorityMid", "PriorityHigh"}, valueNames(e))
	// EnumStrings() of an int enum returns the numbers as strings
	assert.Equal(t, []string{`"1"`, `"2"`, `"3"`}, stringExprs(e))
	assert.Equal(t, []string{"1", "2", "3"}, jsonSchemaEnum(e))

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

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)

	require.NoError(t, err)
	require.Len(t, enums, 1)

	e := enums["Priority"]
	assert.Equal(t, "PriorityNull", e.Null)
	assert.True(t, e.IsNullable())

	assert.Equal(t, []string{"PriorityNull", "PriorityLow", "PriorityMid", "PriorityHigh"}, valueNames(e))
	assert.Equal(t, []string{`"0"`, `"1"`, `"2"`, `"3"`}, stringExprs(e))
	// Null value should not be in JSONSchemaEnum
	assert.Equal(t, []string{"1", "2", "3"}, jsonSchemaEnum(e))
}

func TestFind_JSONSchemaFlag(t *testing.T) {
	source := `package example

type Color string //#enum,jsonschema

const (
	ColorRed   Color = "red"
	ColorGreen Color = "green"
	ColorBlue  Color = "blue"
)`

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)

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

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)

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

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)

	require.NoError(t, err)
	require.Len(t, enums, 2)
	require.Contains(t, enums, "Status")
	require.Contains(t, enums, "Priority")

	assert.Equal(t, "string", enums["Status"].Underlying)
	assert.Equal(t, "int", enums["Priority"].Underlying)
}

func TestFind_ExistingMethods(t *testing.T) {
	// Methods generated before are recognized by the beginning
	// of the doc comment of the generated method, so that they
	// are replaced or removed when they are no longer generated
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)

// Valid indicates if st is any of the valid values for Status
func (st Status) Valid() bool {
	return true
}

// Label is not generated
func (s Status) Label() string {
	return string(s)
}

// String implements the fmt.Stringer interface for Status
func (st Status) String() string {
	return string(st)
}`

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)

	require.NoError(t, err)
	require.Len(t, enums, 1)

	e := enums["Status"]
	assert.Equal(t, "st", e.Recv, "receiver name of generated methods")
	assert.Equal(t, []string{"Valid", "String"}, methodNames(e.generated))
}

func TestFind_HandWrittenMethod(t *testing.T) {
	// A method that would be generated without the doc comment of the
	// generated method was written by hand. Replacing it would silently
	// lose the hand-written code.
	fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const StatusPending Status = "pending"

// String returns the status in upper case
func (s Status) String() string {
	return strings.ToUpper(string(s))
}
`)
	_, err := findInFile(fset, files, astFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "method Status.String in test.go:8 was not generated by go-enum")
	assert.Contains(t, err.Error(), "//#custom")
}

func TestFind_ExistingNullableMethods(t *testing.T) {
	source := `package example

type Priority int //#enum

const (
	PriorityNull Priority = 0 //#null
	PriorityLow  Priority = 1
)

// IsNull returns true if p is the null value PriorityNull
func (p Priority) IsNull() bool {
	return p == PriorityNull
}

// MarshalJSON implements encoding/json.Marshaler for Priority
// by returning the JSON null value for PriorityNull.
func (p Priority) MarshalJSON() ([]byte, error) {
	return nil, nil
}`

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)

	require.NoError(t, err)

	e := enums["Priority"]
	assert.Equal(t, []string{"IsNull", "MarshalJSON"}, methodNames(e.generated))
}

func TestFind_NoEnums(t *testing.T) {
	source := `package example

type Status string

const (
	StatusPending Status = "pending"
)`

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)

	require.NoError(t, err)
	assert.Empty(t, enums)
}

func TestFind_EnumWithoutValues(t *testing.T) {
	source := `package example

type Status string //#enum`

	fset, files, astFile := parseSource(t, source)
	_, err := findInFile(fset, files, astFile)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no typed const enum values")
	// The position must point to the file so that the enum can be located
	assert.Contains(t, err.Error(), "example.Status in test.go:3")
}

func TestFind_DuplicateNullValue(t *testing.T) {
	source := `package example

type Priority int //#enum

const (
	PriorityNull Priority = 0 //#null
	PriorityUnset Priority = -1 //#null
	PriorityLow  Priority = 1
)`

	fset, files, astFile := parseSource(t, source)
	_, err := findInFile(fset, files, astFile)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "second //#null enum encountered")
}

func TestFind_DuplicateLiteralValues(t *testing.T) {
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusOther   Status = "pending"
)`

	fset, files, astFile := parseSource(t, source)
	_, err := findInFile(fset, files, astFile)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate enum value")
	// The position must point to the duplicate, the first occurrence is referenced
	assert.Contains(t, err.Error(), "example.Status in test.go:7")
	assert.Contains(t, err.Error(), "StatusPending in test.go:6 and StatusOther")
}

func TestFind_CustomUintTypes(t *testing.T) {
	source := `package example

type Level uint8 //#enum

const (
	LevelLow  Level = 1
	LevelHigh Level = 2
)`

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)

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

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)

	require.NoError(t, err)

	e := enums["Code"]
	assert.Equal(t, "byte", e.Underlying)
	assert.True(t, e.IsIntType())
}

func TestEnum_JSONType(t *testing.T) {
	// The kind of the underlying basic type decides the JSON type and
	// which methods are generated, also for an underlying named type
	// like MyString, which can't be recognized by its name.
	tests := []struct {
		underlying, value string
		want              string
		isString, isInt   bool
	}{
		{"string", `"a"`, "string", true, false},
		{"MyString", `"a"`, "string", true, false},
		{"int", "1", "number", false, true},
		{"int8", "1", "number", false, true},
		{"uint64", "1", "number", false, true},
		{"byte", "1", "number", false, true},
		{"MyInt", "1", "number", false, true},
		{"float32", "1.5", "number", false, false},
		{"float64", "1.5", "number", false, false},
		{"bool", "true", "boolean", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.underlying, func(t *testing.T) {
			fset, files, astFile := parseSource(t, `package example

type (
	MyString string
	MyInt    int
)

type E `+tt.underlying+` //#enum

const E1 E = `+tt.value+`
`)
			enums, err := findInFile(fset, files, astFile)
			require.NoError(t, err)
			e := enums["E"]
			assert.Equal(t, tt.want, e.JSONType())
			assert.Equal(t, tt.isString, e.IsStringType())
			assert.Equal(t, tt.isInt, e.IsIntType())
		})
	}
}

func TestEnum_LastIndex(t *testing.T) {
	e := &Enum{
		Values: []*Value{{Name: "One"}, {Name: "Two"}, {Name: "Three"}},
	}
	assert.Equal(t, 2, e.LastIndex())
}

func TestFind_PointerReceiverMethods(t *testing.T) {
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
)

// Valid indicates if s is any of the valid values for Status
func (s *Status) Valid() bool {
	return true
}`

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)

	require.NoError(t, err)

	e := enums["Status"]
	assert.Equal(t, "s", e.Recv)
	assert.Equal(t, []string{"Valid"}, methodNames(e.generated))
}

func TestFind_NonStringNonIntJSONSchemaEnum(t *testing.T) {
	source := `package example

type Size int32 //#enum

const (
	SizeSmall Size = 1
	SizeLarge Size = 2
)`

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)

	require.NoError(t, err)

	e := enums["Size"]
	// For non-default types, should have type cast in JSONSchemaEnum
	assert.Equal(t, []string{"int32(1)", "int32(2)"}, jsonSchemaEnum(e))
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

// MarshalJSON implements encoding/json.Marshaler for Status
// by returning the JSON null value for StatusNone.
func (s Status) MarshalJSON() ([]byte, error) {
	return nil, nil
}`

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	require.Contains(t, enums, "Status")

	e := enums["Status"]

	// UnmarshalJSON is custom-marked: present in CustomMethods, not replaced.
	assert.True(t, e.CustomMethods["UnmarshalJSON"], "UnmarshalJSON must be recognized as custom")

	// MarshalJSON was generated before: it's replaced by the generated
	// method and does NOT appear in CustomMethods.
	assert.False(t, e.CustomMethods["MarshalJSON"], "MarshalJSON has no //#custom, so it must not be custom")
	assert.Equal(t, []string{"MarshalJSON"}, methodNames(e.generated))
}

func TestFind_CustomMarkerOnAlwaysGeneratedMethods(t *testing.T) {
	// Valid, Validate, Enums, EnumStrings are generated for every enum,
	// regardless of string/nullable/jsonschema flags. When marked //#custom
	// they must be tracked in CustomMethods and not be replaced,
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

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	require.Contains(t, enums, "Status")

	e := enums["Status"]

	for _, name := range []string{"Valid", "Validate", "Enums", "EnumStrings"} {
		assert.True(t, e.CustomMethods[name], "%s must be recognized as custom", name)
	}
	assert.Empty(t, e.generated, "custom-marked methods must not be replaced")
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

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)

	e := enums["Status"]
	assert.False(t, e.CustomMethods["Foo"], "//#custom on non-generated method must be ignored")
	assert.Empty(t, e.generated)
}

func TestFind_HandWrittenMethodNeverGeneratedForType(t *testing.T) {
	// The generator never produces String for integer enums or Scan for
	// float enums, so such a method is hand-written even with the doc
	// comment of a generated method, and must be kept like with v1.1.0
	// instead of failing as changed generated method
	fset, files, astFile := parseSource(t, `package example

type Color int //#enum

const ColorRed Color = 1

// String implements the fmt.Stringer interface for Color
func (c Color) String() string {
	switch c {
	case ColorRed:
		return "red"
	}
	return "unknown"
}

type Rate float64 //#enum

const RateHalf Rate = 0.5

// Scan implements the database/sql.Scanner interface for Rate
func (r *Rate) Scan(value any) error {
	return nil
}
`)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	assert.Empty(t, enums["Color"].generated)
	assert.Empty(t, enums["Rate"].generated)
}

func TestFind_ValuesFromOtherFiles(t *testing.T) {
	// Enum values may be declared in any file of the package.
	// All of them must be valid values, ordered by file name for a
	// deterministic Enums() result, but insertAfter must stay in the
	// file of the enum type because only that file gets the methods inserted.
	fset, files := parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const StatusActive Status = "active"
`,
		"a_status.go": `package example

const StatusPending Status = "pending"
`,
		"z_status.go": `package example

const (
	StatusDone   Status = "done"
	StatusFailed Status = "failed"
)
`,
	})
	typeFile := files["status.go"]
	enums, err := findInFile(fset, files, typeFile)
	require.NoError(t, err)
	require.Contains(t, enums, "Status")

	e := enums["Status"]
	assert.Equal(t, []string{"StatusPending", "StatusActive", "StatusDone", "StatusFailed"}, valueNames(e))
	assert.Equal(t, []string{`"pending"`, `"active"`, `"done"`, `"failed"`}, stringExprs(e))
	assert.Same(t, typeFile.Decls[1], e.insertAfter, "methods must be inserted after the last enum const of the type's file")
}

func TestFind_ValuesOnlyInOtherFile(t *testing.T) {
	// Without enum consts in the type's file the methods
	// must be inserted after the type declaration.
	fset, files := parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum
`,
		"status_values.go": `package example

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)
`,
	})
	typeFile := files["status.go"]
	enums, err := findInFile(fset, files, typeFile)
	require.NoError(t, err)
	require.Contains(t, enums, "Status")

	e := enums["Status"]
	assert.Equal(t, []string{"StatusPending", "StatusActive"}, valueNames(e))
	assert.Same(t, typeFile.Decls[0], e.insertAfter)
}

func TestFind_ExcludeMarker(t *testing.T) {
	// Typed consts like sentinels or aliases of other values
	// must not become valid values. Without //#exclude the alias
	// would also fail the duplicate value check.
	source := `package example

type Status string //#enum,jsonschema

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
	StatusDefault Status = "pending" //#exclude
)

const StatusInvalid Status = "" //#exclude
`

	fset, files, astFile := parseSource(t, source)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	require.Contains(t, enums, "Status")

	e := enums["Status"]
	assert.Equal(t, []string{"StatusPending", "StatusActive"}, valueNames(e))
	assert.Equal(t, []string{`"pending"`, `"active"`}, stringExprs(e))
	assert.Equal(t, []string{`"pending"`, `"active"`}, jsonSchemaEnum(e))
	assert.Same(t, astFile.Decls[2], e.insertAfter, "excluded consts still count for the insertion position")
}

func TestFind_AllValuesExcluded(t *testing.T) {
	// An enum without valid values would generate an invalid Valid() switch.
	source := `package example

type Status string //#enum

const StatusInvalid Status = "" //#exclude
`

	fset, files, astFile := parseSource(t, source)
	_, err := findInFile(fset, files, astFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no typed const enum values")
}

func TestFind_DuplicateInOtherFile(t *testing.T) {
	// With values collected from all files of the package, a duplicate
	// is often in another file than the enum type. The error must
	// point to the files of the duplicates, not to the file of the type.
	fset, files := parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const StatusActive Status = "active"
`,
		"status_a.go": `package example

const StatusOther Status = "other"
`,
		"status_b.go": `package example

const StatusOtherToo Status = "other"
`,
	})
	_, err := findInFile(fset, files, files["status.go"])
	require.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate enum value "other" for type example.Status in status_b.go:3`)
	assert.Contains(t, err.Error(), "StatusOther in status_a.go:3 and StatusOtherToo")
}

func TestFind_IgnoredFilesNotCollected(t *testing.T) {
	// Find must not depend on the caller filtering files the go tool ignores:
	// consts from _test.go files or files starting with _ or . would be
	// undefined in a regular build of the generated methods.
	fset, files := parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const StatusActive Status = "active"
`,
		"status_test.go": `package example

const StatusTestOnly Status = "test-only"
`,
		"_old_status.go": `package example

const StatusLegacy Status = "legacy"
`,
		".hidden.go": `package example

const StatusHidden Status = "hidden"
`,
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusActive"}, valueNames(enums["Status"]))
}

func TestFind_ConstWithoutValue(t *testing.T) {
	// Code that is still being written can contain consts with
	// missing values. That must be an error pointing to the const
	// instead of a panic.
	fset, files := parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const StatusActive Status = "active"
`,
		"wip.go": `package example

const StatusWIP Status
`,
	})
	_, err := findInFile(fset, files, files["status.go"])
	require.Error(t, err)
	assert.Contains(t, err.Error(), "StatusWIP")
	assert.Contains(t, err.Error(), "wip.go:3")
}

func TestFind_NullInOtherFile(t *testing.T) {
	// A //#null const in another file would make the enum nullable
	// by an edit outside of the file of the enum type, which replaces
	// the generated methods of that file without notice.
	fset, files := parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const StatusActive Status = "active"
`,
		"status_none.go": `package example

const StatusNone Status = "" //#null
`,
	})
	_, err := findInFile(fset, files, files["status.go"])
	require.Error(t, err)
	assert.Contains(t, err.Error(), "StatusNone")
	assert.Contains(t, err.Error(), "status_none.go:3")
	assert.Contains(t, err.Error(), "status.go")
}

func TestFind_TypeDeclaredInSeveralFiles(t *testing.T) {
	// An enum type declared once per platform file must not use
	// the values of the other platforms, else they would be duplicates
	// or undefined on this platform. Values in a file without build
	// constraints are compiled for every platform and must be used.
	fset, files := parseFiles(t, map[string]string{
		"sig_linux.go": `package example

type Sig int //#enum

const (
	SigA         Sig = 1
	SigLinuxOnly Sig = 2
)
`,
		"sig_darwin.go": `package example

type Sig int //#enum

const (
	SigA          Sig = 1
	SigDarwinOnly Sig = 3
)
`,
		"sig_shared.go": `package example

const SigShared Sig = 4
`,
	})
	enums, err := findInFile(fset, files, files["sig_linux.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"SigA", "SigLinuxOnly", "SigShared"}, valueNames(enums["Sig"]))

	enums, err = findInFile(fset, files, files["sig_darwin.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"SigA", "SigDarwinOnly", "SigShared"}, valueNames(enums["Sig"]))

	// Value: protects=a second run on per-platform declarations of an enum type with generated methods only uses the methods of its own file;
	// fails_when=findMethods reads the methods of the other platform's declaration, which compatible() excludes for linux and darwin, so the other platform's Valid fails as "would be generated again";
	// why_new=the fixture above has no methods and no other test declares an enum type in two files with methods;
	// seam=none;
	// no_claim=does not build the per-platform methods
	valid := `
// Valid indicates if s is any of the valid values for Sig
func (s Sig) Valid() bool {
	return true
}
`
	fset, files = parseFiles(t, map[string]string{
		"sig_linux.go":  "package example\n\ntype Sig int //#enum\n\nconst SigLinux Sig = 1\n" + valid,
		"sig_darwin.go": "package example\n\ntype Sig int //#enum\n\nconst SigDarwin Sig = 1\n" + valid,
	})
	for _, fileName := range []string{"sig_linux.go", "sig_darwin.go"} {
		enums, err = findInFile(fset, files, files[fileName])
		require.NoError(t, err, fileName)
		generated := enums["Sig"].generated
		require.Len(t, generated, 1, fileName)
		assert.Equal(t, fileName, fset.Position(generated[0].Pos()).Filename, "only the methods of the own declaration are replaced")
	}
}

func TestFind_TypeDeclaredInIgnoredFile(t *testing.T) {
	// Another declaration of the enum type in a file that is never
	// compiled must not stop collecting the values of the other files,
	// else Valid() would silently reject them.
	fset, files := parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const StatusA Status = "a"
`,
		"status_values.go": `package example

const (
	StatusB Status = "b"
	StatusC Status = "c"
)
`,
		"legacy.go": `//go:build ignore

package example

type Status int
`,
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusA", "StatusB", "StatusC"}, valueNames(enums["Status"]))
}

func TestFind_ConstrainedFilesNotCollected(t *testing.T) {
	// The generated methods in the file of the enum type are compiled
	// for every platform, so they must not reference values from files
	// that the go tool only compiles for some platforms or build tags.
	fset, files := parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const StatusActive Status = "active"
`,
		"status_windows.go": `package example

const StatusWindows Status = "windows"
`,
		"status_linux_arm64.go": `package example

const StatusLinuxARM64 Status = "linux-arm64"
`,
		"status_ignored.go": `//go:build ignore

package example

const StatusIgnored Status = "ignored"
`,
		"status_not_windows.go": `// Copyright notice

//go:build !windows

package example

const StatusNotWindows Status = "not-windows"
`,
		"status_plus_build.go": `// +build tag

package example

const StatusPlusBuild Status = "plus-build"
`,
		// unix is a build tag but not a file name constraint
		"status_unix.go": `package example

const StatusUnix Status = "unix"
`,
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusActive", "StatusUnix"}, valueNames(enums["Status"]))

	// Values in the file of the enum type are compiled together
	// with the generated methods, whatever its build constraints
	fset, files = parseFiles(t, map[string]string{
		"status_windows.go": `//go:build windows

package example

type Status string //#enum

const StatusWindows Status = "windows"
`,
		"status_values.go": `package example

const StatusShared Status = "shared"
`,
	})
	enums, err = findInFile(fset, files, files["status_windows.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusShared", "StatusWindows"}, valueNames(enums["Status"]))
}

func TestFind_ImportedValueInOtherFile(t *testing.T) {
	// Imported packages are not loaded, so a value that uses an import
	// can't be evaluated. It is referenced by name in the generated
	// methods, which works in every file of the package, while the
	// import of another file would not be available in the file of the
	// enum type.
	for name, values := range map[string]string{
		"import": `package example

import "net/http"

const MethodGet Method = http.MethodGet
`,
		"dot import": `package example

import . "net/http"

const MethodGetValue Method = MethodGet
`,
	} {
		t.Run(name, func(t *testing.T) {
			fset, files := parseFiles(t, map[string]string{
				"method.go": `package example

type Method string //#enum

const MethodPost Method = "POST"
`,
				"method_values.go": values,
			})
			enums, err := findInFile(fset, files, files["method.go"])
			require.NoError(t, err)
			e := enums["Method"]
			valueName := e.Values[1].Name
			assert.Equal(t, []string{`"POST"`, "string(" + valueName + ")"}, stringExprs(e))
			assert.Equal(t, []string{`"POST"`, "string(" + valueName + ")"}, jsonSchemaEnum(e))
		})
	}

	t.Run("int", func(t *testing.T) {
		fset, files, astFile := parseSource(t, `package example

import "syscall"

type Signal int //#enum

const (
	SignalNone Signal = 0
	SignalKill Signal = Signal(syscall.SIGKILL)
)
`)
		enums, err := findInFile(fset, files, astFile)
		require.NoError(t, err)
		assert.Equal(t, []string{`"0"`, "fmt.Sprint(int(SignalKill))"}, stringExprs(enums["Signal"]))
	})
}

func TestFind_InvalidConstMarker(t *testing.T) {
	// Only exactly //#null and //#exclude at the end of the const line
	// are markers. Other uses of the reserved marker words must fail,
	// because a silently ignored //#exclude would make a sentinel valid.
	invalid := map[string]string{
		"space":         `StatusInvalid Status = "" // #exclude`,
		"trailing text": `StatusInvalid Status = "" //#exclude legacy`,
		"other marker":  `StatusInvalid Status = "" //#custom`,
		"doc comment": `//#exclude
	StatusInvalid Status = ""`,
	}
	for name, constSpec := range invalid {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const (
	StatusActive Status = "active"
	`+constSpec+`
)
`)
			_, err := findInFile(fset, files, astFile)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid marker")
			assert.Contains(t, err.Error(), "StatusInvalid")
		})
	}

	t.Run("doc comment of single const", func(t *testing.T) {
		fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const StatusActive Status = "active"

//#exclude
const StatusInvalid Status = ""
`)
		_, err := findInFile(fset, files, astFile)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid marker")
	})

	t.Run("regular comments", func(t *testing.T) {
		// Comments of other tools like gosec or editors also start
		// with # and must not break generation
		fset, files, astFile := parseSource(t, `package example

type Status string //#enum

//#region Status values
const (
	// StatusActive is
	// # not a marker but a doc heading
	StatusActive Status = "active" // #1 most used
	StatusNone   Status = ""       //#null
	StatusOld    Status = "old"    //#exclude
	// #nosec G101 -- not a credential
	StatusToken  Status = "token"  //#nosec G101
	StatusBearer Status = "bearer" // #nosec G101
	StatusArrow  Status = "arrow"  // #→ see StatusActive
)
`)
		enums, err := findInFile(fset, files, astFile)
		require.NoError(t, err)
		assert.Equal(t, []string{"StatusActive", "StatusNone", "StatusToken", "StatusBearer", "StatusArrow"}, valueNames(enums["Status"]))
	})

	t.Run("misspelled markers", func(t *testing.T) {
		// Only the exact reserved marker words are checked to keep the
		// rules simple, so misspellings are ignored like other comments
		fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const (
	StatusActive Status = "active"
	StatusA      Status = "a" //#exlude
	StatusB      Status = "b" //#nul
	StatusC      Status = "c" //#Exclude
)
`)
		enums, err := findInFile(fset, files, astFile)
		require.NoError(t, err)
		assert.Equal(t, []string{"StatusActive", "StatusA", "StatusB", "StatusC"}, valueNames(enums["Status"]))
	})
}

func TestFind_InvalidConstMarkerInOtherFile(t *testing.T) {
	// Sentinels in other files must fail the same way,
	// else the misused marker would make them valid.
	fset, files := parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const StatusActive Status = "active"
`,
		"sentinel.go": `package example

const StatusInvalid Status = "" // #exclude
`,
	})
	_, err := findInFile(fset, files, files["status.go"])
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid marker")
	assert.Contains(t, err.Error(), "sentinel.go:3")
}

func TestFind_InvalidCustomMarker(t *testing.T) {
	// A misspelled //#custom must fail instead of letting the generator
	// replace the hand-written method. gofmt rewrites //#custom in doc
	// comments to // #custom, so both forms are valid.
	for name, doc := range map[string]string{
		"trailing text": "//#custom legacy JSON",
		"other marker":  "//#exclude",
	} {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const StatusActive Status = "active"

`+doc+`
func (s Status) Valid() bool {
	return true
}
`)
			_, err := findInFile(fset, files, astFile)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid marker")
			assert.Contains(t, err.Error(), "Valid")
		})
	}

	for name, doc := range map[string]string{
		"custom":       "//#custom",
		"gofmt custom": "// #custom",
	} {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const StatusActive Status = "active"

// Valid is hand-written.
//
// # Heading
//
// #nosec G115 -- comment of another tool
`+doc+`
func (s Status) Valid() bool {
	return true
}
`)
			enums, err := findInFile(fset, files, astFile)
			require.NoError(t, err)
			assert.True(t, enums["Status"].CustomMethods["Valid"])
		})
	}

	t.Run("comment of other tool on generated method", func(t *testing.T) {
		fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const StatusActive Status = "active"

// Valid indicates if s is any of the valid values for Status
//
//#nosec G115
func (s Status) Valid() bool {
	return true
}
`)
		enums, err := findInFile(fset, files, astFile)
		require.NoError(t, err)
		assert.False(t, enums["Status"].CustomMethods["Valid"])
		assert.Equal(t, []string{"Valid"}, methodNames(enums["Status"].generated))
	})
}

func TestFind_MethodsInOtherFiles(t *testing.T) {
	// The generated methods are only written to the file of the enum type.
	// A method in another file that the generator would produce can't be
	// replaced, so generating it again would not compile.
	typeFile := `package example

type Status string //#enum

const StatusActive Status = "active"
`
	t.Run("custom", func(t *testing.T) {
		fset, files := parseFiles(t, map[string]string{
			"status.go": typeFile,
			"status_methods.go": `package example

// Valid is hand-written.
//
// #custom
func (s Status) Valid() bool {
	return s != ""
}

// Label is not generated.
func (s Status) Label() string {
	return string(s)
}
`,
		})
		enums, err := findInFile(fset, files, files["status.go"])
		require.NoError(t, err)
		e := enums["Status"]
		assert.Equal(t, map[string]bool{"Valid": true}, e.CustomMethods)
		assert.Empty(t, e.generated, "methods of other files can't be replaced")
		assert.Equal(t, "s", e.Recv)
	})

	t.Run("not custom", func(t *testing.T) {
		// Silently keeping the method could leave a stale copy
		// of generated code that misses new enum values
		fset, files := parseFiles(t, map[string]string{
			"status.go": typeFile,
			"status_methods.go": `package example

func (s Status) Enums() []Status {
	return []Status{StatusActive}
}
`,
		})
		_, err := findInFile(fset, files, files["status.go"])
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Status.Enums")
		assert.Contains(t, err.Error(), "status_methods.go:3")
		assert.Contains(t, err.Error(), "//#custom")
	})

	t.Run("misspelled marker", func(t *testing.T) {
		// A misspelled //#custom is ignored like other comments,
		// so the method is unmarked and fails as such
		fset, files := parseFiles(t, map[string]string{
			"status.go": typeFile,
			"status_methods.go": `package example

//#custum
func (s Status) Valid() bool {
	return true
}
`,
		})
		_, err := findInFile(fset, files, files["status.go"])
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Status.Valid")
		assert.Contains(t, err.Error(), "//#custom")
	})
}

func TestFind_BlankIdentifierConst(t *testing.T) {
	// A blank const of the enum type reserves a value
	// but can't be used in the generated methods.
	fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const (
	StatusActive Status = "active"
	_            Status = "reserved"
)
`)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusActive"}, valueNames(enums["Status"]))
}

func TestFind_AliasOfOtherEnumValue(t *testing.T) {
	// A const initialized with another enum const has the same value,
	// so it is not another enum value. As value it would generate a
	// duplicate switch case that does not compile. Packages with such
	// aliases in other files worked before values were collected from
	// all files, so aliases must not need a //#exclude marker.
	fset, files := parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const (
	StatusActive  Status = "active"
	StatusPending Status = "pending"
	StatusFirst   Status = StatusActive
)
`,
		"defaults.go": `package example

const (
	StatusDefault  Status = StatusPending
	StatusFallback Status = (StatusDefault)
)
`,
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusActive", "StatusPending"}, valueNames(enums["Status"]))
	assert.Equal(t, []string{`"active"`, `"pending"`}, stringExprs(enums["Status"]))

	// Other expressions with enum consts are evaluated to new values
	fset, files = parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const StatusActive Status = "active"
`,
		"derived.go": `package example

const StatusActiveOld Status = StatusActive + "-old"
`,
	})
	enums, err = findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	// Ordered by file name
	assert.Equal(t, []string{`"active-old"`, `"active"`}, stringExprs(enums["Status"]))

	// The null value is used by the generated methods like IsNull()
	// and must be an enum value, so it is not skipped as alias but
	// fails as duplicate value instead of silently being no value
	fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const (
	StatusEmpty Status = ""
	StatusNone  Status = StatusEmpty //#null
	StatusNull  Status = "null"      //#null
)
`)
	_, err = findInFile(fset, files, astFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate enum value "" for type example.Status in test.go:7 (used by both StatusEmpty in test.go:6 and StatusNone)`)
}

func TestFind_DuplicateValueWrittenDifferently(t *testing.T) {
	// Equal constant values written differently would also
	// generate duplicate switch cases that do not compile.
	for name, values := range map[string][3]string{
		"hex int":    {"int", "1", "0x1"},
		"shift":      {"uint8", "1 << 5", "32"},
		"named":      {"int", "one", "1"},
		"raw string": {"string", `"a"`, "`a`"},
	} {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, `package example

const one = 1

type Code `+values[0]+` //#enum

const (
	CodeA Code = `+values[1]+`
	CodeB Code = `+values[2]+`
)
`)
			_, err := findInFile(fset, files, astFile)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "duplicate enum value")
		})
	}
}

func TestFind_IotaAndImplicitRepetition(t *testing.T) {
	// Consts without type and value repeat the previous expression
	// with the next iota. They are enum values like the first const,
	// else Valid() would silently reject them.
	fset, files, astFile := parseSource(t, `package example

type Flag uint8 //#enum

const (
	FlagA Flag = 1 << iota
	FlagB
	FlagC
)

const FlagD = Flag(0x10)
`)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	e := enums["Flag"]
	assert.Equal(t, []string{"FlagA", "FlagB", "FlagC", "FlagD"}, valueNames(e))
	// EnumStrings() must return the values, not the source code
	assert.Equal(t, []string{`"1"`, `"2"`, `"4"`, `"16"`}, stringExprs(e))
	assert.Equal(t, []string{"uint8(1)", "uint8(2)", "uint8(4)", "uint8(16)"}, jsonSchemaEnum(e))
}

func TestFind_InvalidEnumMarker(t *testing.T) {
	// The reserved word enum must be used exactly as //#enum marker,
	// else the type would silently generate no methods
	for name, typeDecl := range map[string]string{
		"space":          "type Status string // #enum",
		"missing comma":  "type Status string //#enum jsonschema",
		"doc comment":    "//#enum\ntype Status string",
		"type block doc": "//#enum\ntype (\n\tStatus string\n)",
	} {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, `package example

`+typeDecl+`

const StatusActive Status = "active"
`)
			_, err := findInFile(fset, files, astFile)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid marker comment")
			assert.Contains(t, err.Error(), "type Status")
		})
	}

	// Only the reserved words are picked up, other words are ignored
	// like other comments to keep the rules simple
	for name, typeDecl := range map[string]string{
		"typo":   "type Status string //#enmu",
		"plural": "type Status string //#enums",
	} {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, `package example

`+typeDecl+`

const StatusActive Status = "active"
`)
			enums, err := findInFile(fset, files, astFile)
			require.NoError(t, err)
			assert.Empty(t, enums)
		})
	}
	t.Run("unknown flag", func(t *testing.T) {
		fset, files, astFile := parseSource(t, `package example

type Status string //#enum,jsonschem,jsonschema

const StatusActive Status = "active"
`)
		enums, err := findInFile(fset, files, astFile)
		require.NoError(t, err)
		require.Contains(t, enums, "Status")
		assert.True(t, enums["Status"].JSONSchema)
	})
}

func TestFind_InvalidUnderlyingType(t *testing.T) {
	// The kind of the underlying type decides which methods are
	// generated, so it must be known. Imported packages are not
	// loaded, so an imported underlying type can't be used.
	for name, typeDecl := range map[string]string{
		"imported": "type Timeout time.Duration //#enum",
		"alias":    "type Timeout = int //#enum",
		// Complex numbers can't be marshaled as JSON
		"complex": "type Timeout complex128 //#enum",
	} {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, `package example

import "time"

`+typeDecl+`

const TimeoutShort Timeout = 1
`)
			_, err := findInFile(fset, files, astFile)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "enum type Timeout in test.go:5")
		})
	}
}

func TestFind_ExcludeInOtherFile(t *testing.T) {
	// Sentinels and aliases can be declared in any file of the package
	fset, files := parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const StatusActive Status = "active"
`,
		"status_legacy.go": `package example

const StatusLegacy Status = "legacy" //#exclude
`,
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusActive"}, valueNames(enums["Status"]))
}

func TestFind_ConstValueCountMismatch(t *testing.T) {
	// Files that are never compiled can have fewer values than names,
	// which must be an error instead of a panic
	fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const StatusA, StatusB Status = "a"
`)
	_, err := findInFile(fset, files, astFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "StatusB")
	assert.Contains(t, err.Error(), "needs a value")
}

func TestFind_NullOnMultiNameConst(t *testing.T) {
	// One //#null marker for several names would make
	// the first name the null value and silently drop the others
	fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const StatusA, StatusB Status = "", "b" //#null
`)
	_, err := findInFile(fset, files, astFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cant use //#null for multiple enums in test.go:5")
}

// Value: protects=only the names of a multi-name const spec that are of the enum type become values, not maxRetries in `StatusA, maxRetries = Status("a"), 3`;
// fails_when=findValues drops the per-name isEnumConst check because the spec already has an enum const, so other consts become values;
// why_new=no test has a const spec mixing enum and other consts; coverage shows the skip of a non-enum name in findValues never runs;
// seam=none;
// no_claim=does not build the generated methods, only checks the values found
func TestFind_MultiNameConstWithOtherTypes(t *testing.T) {
	// A const spec can declare an enum const together with consts of other
	// types. As enum values, an int const would make Valid() not compile and
	// an untyped string const would silently become a valid value.
	fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const (
	StatusA, maxRetries = Status("a"), 3
	StatusB, label      = Status("b"), "label"
)
`)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusA", "StatusB"}, valueNames(enums["Status"]))
}

func TestFind_EnumTypeInIgnoredFile(t *testing.T) {
	// Like the go tool, files starting with _ are ignored,
	// so no methods are generated for their enum types
	fset, files := parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const StatusActive Status = "active"
`,
		"_legacy.go": `package example

type Legacy string //#enum

const LegacyA Legacy = "a"
`,
	})
	enums, err := Find(fset, files)
	require.NoError(t, err)
	require.Len(t, enums, 1)
	assert.Equal(t, "Status", enums[0].Type)
}

func TestFind_FloatValues(t *testing.T) {
	// EnumStrings() must list the values as they are printed,
	// also for float32 which go/types stores with float32 precision
	fset, files, astFile := parseSource(t, `package example

type Ratio float32 //#enum

const (
	RatioTenth Ratio = 0.1
	RatioHalf  Ratio = 0.5
)
`)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	assert.Equal(t, []string{`"0.1"`, `"0.5"`}, stringExprs(enums["Ratio"]))
}

func TestFind_MarkerOnConstWithoutEnumType(t *testing.T) {
	// A const with an explicit value but without type in a group is untyped,
	// so a //#null there would silently not make the enum nullable
	fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const (
	StatusActive Status = "active"
	StatusNone          = "" //#null
)
`)
	_, err := findInFile(fset, files, astFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "StatusNone")
	assert.Contains(t, err.Error(), "test.go:7")
}

func TestFind_MarkerOnValueWithoutEnumType(t *testing.T) {
	// //#null and //#exclude at a var or at a const that is not of an enum
	// type would be ignored silently, like the enum not being nullable
	for name, source := range map[string]string{
		"var":          "var StatusNone Status = \"\" //#null\n",
		"untyped":      "const StatusNone = \"\" //#null\n",
		"other type":   "const StatusNone int = 0 //#exclude\n",
		"var doc":      "//#null\nvar StatusNone Status\n",
		"repeated int": "const (\n\tA int = iota\n\tStatusNone //#exclude\n)\n",
	} {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n\n"+source)
			_, err := findInFile(fset, files, astFile)
			require.ErrorContains(t, err, "marker")
			assert.Contains(t, err.Error(), "test.go:")
		})
	}

	// Markers at consts of an enum type, also implicitly repeated, converted
	// or in files that are not compiled with the enum type, are valid
	//
	// Value: protects=//#exclude at an untyped conversion to the enum type in a file of some builds, like MethodLinuxAny, is valid;
	// fails_when=constTypeName loses its conversion form, so the const in status_linux.go is taken as no const of an enum type and fails;
	// why_new=the conversion MethodAny is in the file of the enum type, whose enum consts are allowed without constTypeName;
	// seam=none;
	// no_claim=does not check that a //#null there fails, only that //#exclude is accepted
	fset, files := parseFiles(t, map[string]string{
		"status.go":       "package example\n\nimport \"net/http\"\n\ntype Method string //#enum\n\nconst (\n\tMethodGet = Method(http.MethodGet)\n\tMethodAny = Method(\"*\") //#exclude\n)\n\ntype Kind int //#enum\n\nconst (\n\tKindA Kind = iota\n\tKindB //#exclude\n\tKindC\n)\n",
		"status_linux.go": "package example\n\nconst MethodLinux Method = \"LINUX\" //#exclude\n\nconst MethodLinuxAny = Method(\"*\") //#exclude\n\nconst MethodLinuxParen = (Method)(\"+\") //#exclude\n",
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"MethodGet"}, valueNames(enums["Method"]))
	assert.Equal(t, []string{"KindA", "KindC"}, valueNames(enums["Kind"]))

	// Such a file is not type checked with the enum type, so an untyped
	// const computed from an enum const needs the type for its marker
	fset, files = parseFiles(t, map[string]string{
		"kind.go":       "package example\n\ntype Kind int //#enum\n\nconst KindA Kind = 1\n",
		"kind_linux.go": "package example\n\nconst KindLinux = KindA + 1 //#exclude\n",
	})
	_, err = Find(fset, files)
	require.ErrorContains(t, err, "KindLinux in kind_linux.go:3 is no const of an enum type")
	assert.Contains(t, err.Error(), "declare it with the enum type if it is one")
}

func TestFind_ReportsAllErrors(t *testing.T) {
	// All errors of a package are reported at once,
	// so that a run shows everything that has to be fixed
	fset, files := parseFiles(t, map[string]string{
		"a.go": "package example\n\ntype A string //#enum\n",
		"b.go": "package example\n\ntype B string // #enum\n",
	})
	_, err := Find(fset, files)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "example.A in a.go:3")
	assert.Contains(t, err.Error(), "type B in b.go:3")
}

func TestFind_BlankNullConst(t *testing.T) {
	// The null value is used by name in the generated methods
	fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const (
	StatusActive Status = "active"
	_            Status = "" //#null
)
`)
	_, err := findInFile(fset, files, astFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "can't be blank")
}

func TestFind_MethodInConstrainedFile(t *testing.T) {
	// A method in a file with build constraints is compiled together with
	// the generated methods on that platform, so generating it again
	// would not compile there
	fset, files := parseFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const StatusActive Status = "active"
`,
		"status_plan9.go": `package example

func (s Status) Validate() error {
	return nil
}
`,
	})
	_, err := findInFile(fset, files, files["status.go"])
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Status.Validate in status_plan9.go:3")
	assert.Contains(t, err.Error(), "//#custom")
}

func TestFind_PerPlatformTypeIgnoresOtherPlatformMethods(t *testing.T) {
	// A //#custom method of another platform's file must not stop
	// generating that method for this platform's declaration of the type
	fset, files := parseFiles(t, map[string]string{
		"status_linux.go":  "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
		"status_darwin.go": "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
		"label_linux.go":   "package example\n\n//#custom\nfunc (s Status) String() string { return \"label\" }\n",
	})
	enums, err := findInFile(fset, files, files["status_darwin.go"])
	require.NoError(t, err)
	assert.False(t, enums["Status"].CustomMethods["String"])
}

func TestFind_CgoFileValuesNotCollected(t *testing.T) {
	// Files importing "C" are only compiled with cgo enabled,
	// so their values would be undefined with CGO_ENABLED=0
	fset, files := parseFiles(t, map[string]string{
		"status.go":     "package example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		"values_cgo.go": "package example\n\nimport \"C\"\n\nconst StatusB Status = 2\n",
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusA"}, valueNames(enums["Status"]))
}

func TestFind_FileNameConstraintLikeGoBuild(t *testing.T) {
	// Like go/build, only the part of the file name before the first dot
	// can have a GOOS or GOARCH suffix, so values.gen_linux.go is compiled
	// on every platform
	fset, files := parseFiles(t, map[string]string{
		"status.go":           "package example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		"values.gen_linux.go": "package example\n\nconst StatusB Status = 2\n",
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusA", "StatusB"}, valueNames(enums["Status"]))
}

func TestFind_EquivalentBuildConstraints(t *testing.T) {
	// Values in files the go tool compiles in every build of the file of
	// the enum type must be used however their build constraints are
	// written, else Valid() would reject them in those builds
	fset, files := parseFiles(t, map[string]string{
		"status_linux.go":   "//go:build linux\n\npackage example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		"values.go":         "//go:build linux\n\npackage example\n\nconst StatusB Status = 2\n",
		"values_android.go": "package example\n\nconst StatusC Status = 3\n",
		"values_unix.go":    "//go:build unix\n\npackage example\n\nconst StatusD Status = 4\n",
		"values_amd64.go":   "package example\n\nconst StatusE Status = 5\n",
		// Like with go/build only linux, amd64 is not a GOOS
		"values_amd64_linux.go": "package example\n\nconst StatusF Status = 6\n",
	})
	enums, err := findInFile(fset, files, files["status_linux.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusA", "StatusB", "StatusF", "StatusD"}, valueNames(enums["Status"]), "android and amd64 are not in every linux build")

	// GOOS=android also compiles the files for linux
	fset, files = parseFiles(t, map[string]string{
		"status_android.go": "package example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		"values_linux.go":   "package example\n\nconst StatusB Status = 2\n",
	})
	enums, err = findInFile(fset, files, files["status_android.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusA", "StatusB"}, valueNames(enums["Status"]))
}

func TestFind_CustomMethodNotInEveryBuild(t *testing.T) {
	// A //#custom method in another file stops generating the method in
	// every build of the file of the enum type, so the builds without the
	// custom method would silently have no such method
	status := "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n"
	custom := "package example\n\n//#custom\nfunc (s Status) String() string { return \"custom\" }\n"
	fset, files := parseFiles(t, map[string]string{
		"status.go":       status,
		"string_linux.go": custom,
	})
	_, err := Find(fset, files)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Status.String is marked with //#custom only in string_linux.go, which the go tool does not compile in every build of status.go")

	fset, files = parseFiles(t, map[string]string{
		"status.go":       status,
		"string_linux.go": custom,
		"string_other.go": "//go:build !linux\n\n" + custom,
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.True(t, enums["Status"].CustomMethods["String"])

	// A file compiled in some builds of a constrained file of the enum type
	// would also have the method twice if it was generated
	fset, files = parseFiles(t, map[string]string{
		"status_linux.go":       status,
		"string_linux_amd64.go": custom,
	})
	_, err = Find(fset, files)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Status.String is marked with //#custom only in string_linux_amd64.go")
}

func TestFind_InvalidBuildConstraint(t *testing.T) {
	// The go tool fails for an invalid build constraint, so its
	// values can't be silently used or ignored
	fset, files := parseFiles(t, map[string]string{
		"status.go": "package example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		"values.go": "//go:build linux &&\n\npackage example\n\nconst StatusB Status = 2\n",
	})
	_, err := Find(fset, files)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid build constraint in values.go:1")
}

func TestFind_BuildSettingsOfOneBuild(t *testing.T) {
	// A build has one GOOS and one GOARCH, which can set more tags like
	// GOOS=ios sets darwin, so methods in files for another GOARCH are not
	// compiled with the enum type and values for a GOOS that is set
	// together with the one of the enum type are always compiled with it
	fset, files := parseFiles(t, map[string]string{
		"status_amd64.go":   "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
		"validate_arm64.go": "package example\n\nfunc (s Status) Validate() error { return nil }\n",
	})
	_, err := Find(fset, files)
	require.NoError(t, err)

	for typeFile, valueFile := range map[string]string{
		"status_ios.go":     "values_darwin.go",
		"status_illumos.go": "values_solaris.go",
		// Like with go/build only linux, windows is not a GOARCH
		"status_linux.go": "values_windows_linux.go",
	} {
		fset, files = parseFiles(t, map[string]string{
			typeFile:  "package example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
			valueFile: "package example\n\nconst StatusB Status = 2\n",
		})
		enums, err := findInFile(fset, files, files[typeFile])
		require.NoError(t, err, typeFile)
		assert.Equal(t, []string{"StatusA", "StatusB"}, valueNames(enums["Status"]), typeFile)
	}

	// Another GOOS than the ones of the build constraints can be windows
	fset, files = parseFiles(t, map[string]string{
		"status.go": "package example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		"b.go":      "//go:build unix\n\npackage example\n\nconst StatusB Status = 2\n",
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusA"}, valueNames(enums["Status"]))

	// Not every unix GOOS is linux
	fset, files = parseFiles(t, map[string]string{
		"status.go":       "//go:build unix\n\npackage example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		"values_linux.go": "package example\n\nconst StatusB Status = 2\n",
	})
	enums, err = findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusA"}, valueNames(enums["Status"]))

	// Tags with _ or . are no GOOS or GOARCH, like the GOAMD64 tag amd64.v3,
	// so a hand-written method in such a file is compiled with the enum type
	// on linux/amd64 and would be generated again
	for _, tag := range []string{"static_linux", "amd64 && amd64.v3"} {
		fset, files = parseFiles(t, map[string]string{
			"status_linux.go": "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
			"string.go":       "//go:build " + tag + "\n\npackage example\n\nfunc (s Status) String() string { return \"s\" }\n",
		})
		_, err = Find(fset, files)
		require.ErrorContains(t, err, "would be generated again", tag)
	}
}

func TestFind_ManyBuildTags(t *testing.T) {
	// GOOS tags are one setting of a build, so a type file for many GOOS
	// values still uses the values of files without build constraints
	// and its own //#custom methods
	goos := "//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || js || linux || netbsd || openbsd || plan9 || solaris\n\n"
	fset, files := parseFiles(t, map[string]string{
		"status.go": goos + "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n\n//#custom\nfunc (s Status) String() string { return \"s\" }\n",
		"values.go": "//go:build !windows\n\npackage example\n\nconst StatusB Status = \"b\"\n",
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusA", "StatusB"}, valueNames(enums["Status"]))

	// Every combination of independent tags is evaluated, so with more than
	// maxBuildTags of them, values are not used, because they could be
	// missing in a build, and methods are compared, because they could be
	// compiled with the enum type
	var tags []string
	for i := range maxBuildTags + 1 {
		tags = append(tags, fmt.Sprintf("t%d", i))
	}
	fset, files = parseFiles(t, map[string]string{
		"status_linux.go": "package example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		// True in every build
		"values.go": "//go:build " + strings.Join(tags, " || ") + " || !t0\n\npackage example\n\nconst StatusB Status = 2\n",
		// Never compiled with linux
		"validate.go": "//go:build windows && " + strings.Join(tags, " && ") + "\n\npackage example\n\nfunc (s Status) Validate() error { return nil }\n",
	})
	_, err = Find(fset, files)
	require.ErrorContains(t, err, "Status.Validate in validate.go:5 would be generated again")
	delete(files, "validate.go")
	enums, err = findInFile(fset, files, files["status_linux.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusA"}, valueNames(enums["Status"]))

	// Files without build constraints or with the same ones as the file of
	// the enum type are compiled with it without evaluating the tags
	many := "//go:build " + strings.Join(tags, " || ") + "\n\n"
	fset, files = parseFiles(t, map[string]string{
		"status.go": many + "package example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		"b.go":      "package example\n\nconst StatusB Status = 2\n",
		"c.go":      many + "package example\n\nconst StatusC Status = 3\n",
	})
	enums, err = findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusB", "StatusC", "StatusA"}, valueNames(enums["Status"]))

	// Also the number of evaluated builds of all settings is limited,
	// so that huge build constraints can't make the generator hang
	fset, files = parseFiles(t, map[string]string{
		"status.go": "package example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		"values.go": "//go:build (" + strings.Join(tags[:maxBuildTags], " || ") + " || !t0) && (" +
			strings.TrimPrefix(goos, "//go:build ")[:len(goos)-len("//go:build ")-2] + " || !linux) && (386 || amd64 || arm || arm64 || loong64 || mips || mips64 || ppc64 || riscv64 || s390x || wasm || !amd64)\n\npackage example\n\nconst StatusB Status = 2\n",
	})
	enums, err = findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusA"}, valueNames(enums["Status"]), "true in every build, but too many builds to evaluate")
}

func TestFind_PlusBuildLines(t *testing.T) {
	// Like with go/build, // +build lines are ignored with a //go:build line,
	// in the package doc comment, and after a /* */ comment
	fset, files := parseFiles(t, map[string]string{
		"status.go": "package example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		"b.go":      "//go:build windows\n// +build linux\n\npackage example\n\nconst StatusB Status = 2\n",
		"c.go":      "//go:build unix || !unix\n// +build windows\n\npackage example\n\nconst StatusC Status = 3\n",
		"d.go":      "// +build windows\npackage example\n\nconst StatusD Status = 4\n",
		"e.go":      "/* License */\n\n// +build windows\n\npackage example\n\nconst StatusE Status = 5\n",
		"f.go":      "// +build windows\n\npackage example\n\nconst StatusF Status = 6\n",
		"g.go":      "// +build windows\n/* License */\n\npackage example\n\nconst StatusG Status = 7\n",
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusC", "StatusD", "StatusE", "StatusG", "StatusA"}, valueNames(enums["Status"]))
}

func TestFind_ReleaseAndCompilerTags(t *testing.T) {
	// The generated methods use any, so they are only compiled with Go 1.18
	// or later, where the go1.18 tag is set, and a Go version sets the tags
	// of all earlier versions. The gc tag is not set with gccgo.
	fset, files := parseFiles(t, map[string]string{
		"status.go": "package example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		"b.go":      "//go:build go1.18\n\npackage example\n\nconst StatusB Status = 2\n",
		"c.go":      "//go:build go1.30\n\npackage example\n\nconst StatusC Status = 3\n",
		"d.go":      "//go:build gc\n\npackage example\n\nconst StatusD Status = 4\n",
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusB", "StatusA"}, valueNames(enums["Status"]))

	fset, files = parseFiles(t, map[string]string{
		"status.go": "//go:build go1.22\n\npackage example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		"b.go":      "//go:build go1.21\n\npackage example\n\nconst StatusB Status = 2\n",
		"c.go":      "//go:build go1.23\n\npackage example\n\nconst StatusC Status = 3\n",
	})
	enums, err = findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusB", "StatusA"}, valueNames(enums["Status"]))

	// Go 1.16 can't compile the generated methods, so a file for go1.16 is
	// compiled with Go 1.18, and go1.018 is no release tag
	fset, files = parseFiles(t, map[string]string{
		"status.go": "//go:build go1.16\n\npackage example\n\ntype Status int //#enum\n\nconst StatusA Status = 1\n",
		"b.go":      "//go:build go1.17\n\npackage example\n\nconst StatusB Status = 2\n",
		"c.go":      "//go:build go1.19\n\npackage example\n\nconst StatusC Status = 3\n",
		"d.go":      "//go:build go1.018\n\npackage example\n\nconst StatusD Status = 4\n",
	})
	enums, err = findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusB", "StatusA"}, valueNames(enums["Status"]))

	// The compiler is either gc or gccgo
	fset, files = parseFiles(t, map[string]string{
		"status.go": "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
		"b.go":      "//go:build gc || gccgo\n\npackage example\n\nconst StatusB Status = \"b\"\n",
		"c.go":      "//go:build !(gc && gccgo)\n\npackage example\n\nconst StatusC Status = \"c\"\n",
		"string.go": "//go:build gc\n\npackage example\n\nfunc (s Status) String() string { return \"gc\" }\n",
	})
	_, err = Find(fset, files)
	require.ErrorContains(t, err, "Status.String in string.go:5 would be generated again")
	delete(files, "string.go")
	enums, err = findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusB", "StatusC", "StatusA"}, valueNames(enums["Status"]))
}

func TestFind_IgnoredFiles(t *testing.T) {
	// Files with //go:build ignore are never compiled with the package, like
	// a script for go run in a main package or an old version of a file, so
	// they must not cause errors. A script for go run is compiled alone,
	// so its enum types only have its own values.
	fset, files := parseFiles(t, map[string]string{
		"main.go":       "package main\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n\nfunc main() {}\n",
		"gen.go":        "//go:build ignore\n\npackage main\n\ntype Status string //#enum\n\nconst StatusB Status = \"b\"\n\nfunc main() {}\n",
		"old_status.go": "//go:build ignore\n\npackage main\n\nfunc (s Status) String() string { return \"old\" }\n",
	})
	enums, err := findInFile(fset, files, files["main.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusA"}, valueNames(enums["Status"]))
	enums, err = findInFile(fset, files, files["gen.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusB"}, valueNames(enums["Status"]))
}

func TestFind_GeneratedMethodOfOtherUnderlyingType(t *testing.T) {
	// A method that is never generated for the underlying type but has the
	// doc comment and code of a generated method, like String after changing
	// the underlying type from string to int, which converts the integer to a
	// rune, is an error, because it can also be a correct hand-written method,
	// like String of a rune enum, that must not be removed silently
	for name, source := range map[string]string{
		"int": `package example

type Status int //#enum

const StatusA Status = 1

// String implements the fmt.Stringer interface for Status
func (s Status) String() string {
	return string(s)
}
`,
		"rune": `package example

type Status rune //#enum

const StatusA Status = 'A'

// String implements the fmt.Stringer interface for Status
func (s Status) String() string {
	return string(s)
}
`,
	} {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, source)
			_, err := findInFile(fset, files, astFile)
			require.ErrorContains(t, err, "method Status.String in test.go:8 has the code of a method generated by go-enum for another underlying type than "+name)

			// Kept with //#custom
			fset, files, astFile = parseSource(t, strings.Replace(source, "func (s Status) String()", "//#custom\nfunc (s Status) String()", 1))
			enums, err := findInFile(fset, files, astFile)
			require.NoError(t, err)
			assert.Empty(t, enums["Status"].generated)
		})
	}
}

func TestFind_MarkerOnExpressionTypedEnumConst(t *testing.T) {
	// go/types gives these consts the enum type through their expression,
	// so they are enum values and their markers must not be rejected as
	// markers of consts without enum type
	for name, consts := range map[string]string{
		"derived":                "\tKindB = KindA + 1 //#exclude\n",
		"alias":                  "\tKindDefault = KindA //#exclude\n",
		"parenthesized":          "\tKindC = (Kind)(3) //#exclude\n",
		"other name in the spec": "\tmaxRetries, KindB = 3, Kind(2) //#exclude\n",
	} {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, "package example\n\ntype Kind int //#enum\n\nconst (\n\tKindA Kind = 1\n"+consts+")\n")
			enums, err := findInFile(fset, files, astFile)
			require.NoError(t, err)
			assert.Equal(t, []string{"KindA"}, valueNames(enums["Kind"]))
		})
	}
}

func TestFind_MarkersOfConstsInFilesOfSomeBuilds(t *testing.T) {
	// Values in files that are not compiled in every build of the file of
	// the enum type are not used, so their markers are not checked with
	// the values, but a //#null there or a misused marker must not be
	// ignored silently
	status := "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n"
	for source, message := range map[string]string{
		"package example\n\nconst StatusNone Status = \"\" //#null\n":             "the //#null enum const StatusNone in status_linux.go:3 must be declared in status.go, the file of the enum type",
		"package example\n\nconst StatusOld Status = \"old\" //#exclude legacy\n": "invalid marker comment \"//#exclude legacy\" for enum const StatusOld in status_linux.go:3",
	} {
		fset, files := parseFiles(t, map[string]string{"status.go": status, "status_linux.go": source})
		_, err := Find(fset, files)
		require.ErrorContains(t, err, message)
	}
}

func TestFind_ConstUsingEnumConversionInOtherType(t *testing.T) {
	// A const that only uses a conversion to the enum type, like a string
	// of an enum value, is no enum const, so its unknown value from an
	// import must not fail generation for the package
	fset, files, astFile := parseSource(t, `package example

import "net/http"

type Method string //#enum

const (
	MethodGet     = Method(http.MethodGet)
	DefaultHeader = "X-" + string(Method(http.MethodGet))
)
`)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"MethodGet"}, valueNames(enums["Method"]))
}

func TestFind_MethodsOfOtherDeclarationWithCompatibleTags(t *testing.T) {
	// Custom build tags like oss and ent are exclusive by convention, which
	// build constraints can't express, so the methods of the other
	// declaration of the enum type must be ignored instead of failing
	// as methods that would be generated again
	fset, files := parseFiles(t, map[string]string{
		"status_oss.go": "//go:build oss\n\npackage example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
		"status_ent.go": "//go:build ent\n\npackage example\n\ntype Status string\n\nfunc (s Status) Valid() bool { return true }\n",
	})
	enums, err := findInFile(fset, files, files["status_oss.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusA"}, valueNames(enums["Status"]))
}

func TestFind_MarkerWordInFunctionBody(t *testing.T) {
	// Comments in function bodies are no markers,
	// so a marker word there must not fail the package
	fset, files, astFile := parseSource(t, "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n\nfunc f() {\n\t// #exclude archived values below\n\t_ = StatusA\n}\n")
	_, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
}

func TestFind_InternalTestFiles(t *testing.T) {
	// go test compiles the internal test files of the package together with
	// the generated methods, so a method or a package-level name there that
	// collides with them would only fail in test builds, while their values
	// are not compiled in regular builds and are no enum values
	status := "package example\n\ntype Status string //#enum\n\nconst (\n\tStatusNone   Status = \"\" //#null\n\tStatusActive Status = \"active\"\n)\n"
	for source, message := range map[string]string{
		"package example\n\nfunc (s Status) String() string { return \"test\" }\n": "method Status.String in status_test.go:3 of a test file would be declared twice in test builds with the method generated in status.go",
		"package example\n\nconst driver = \"sqlite3\"\n":                          "driver in status_test.go:3 has the name of the package database/sql/driver",
	} {
		fset, files := parseFiles(t, map[string]string{"status.go": status, "status_test.go": source})
		_, err := Find(fset, files)
		require.ErrorContains(t, err, message)
	}

	// Markers in test files are not read, like before v1.2.0
	fset, files := parseFiles(t, map[string]string{
		"status.go":      status,
		"status_test.go": "package example\n\ntype testKind int //#enum\n\nconst testKindA testKind = 1\n\nconst StatusTestNull Status = \"\" //#null\n\nvar debugStatus = StatusActive //#exclude\n",
	})
	found, err := Find(fset, files)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "Status", found[0].Type)

	// Other methods and the external test package are fine
	fset, files = parseFiles(t, map[string]string{
		"status.go":          status,
		"status_test.go":     "package example\n\nconst StatusTestOnly Status = \"test\"\n\nfunc (s Status) Label() string { return \"label\" }\n",
		"status_ext_test.go": "package example_test\n\nconst driver = \"sqlite3\"\n",
	})
	enums, err := findInFile(fset, files, files["status.go"])
	require.NoError(t, err)
	assert.Equal(t, []string{"StatusNone", "StatusActive"}, valueNames(enums["Status"]))
}

func TestFind_UnevaluableConstsOfEnumType(t *testing.T) {
	// Consts of the enum type that use an import can't be evaluated, and
	// must fail instead of silently not being enum values: also when built
	// from an untyped conversion of an import, with && and ! of a bool enum,
	// or with the builtins min and max
	for name, consts := range map[string]string{
		"derived from converted import": "type Flag int //#enum\n\nconst (\n\tFlagRead = Flag(os.O_RDONLY)\n\tx        = FlagRead + 1\n)\n",
		"or of converted imports":       "type Flag int //#enum\n\nconst (\n\tFlagRead = Flag(os.O_RDONLY)\n\tx        = FlagRead | Flag(os.O_WRONLY)\n)\n",
		"not of bool enum":              "type Flag bool //#enum\n\nconst (\n\tFlagOn = Flag(os.O_RDONLY == 0)\n\tx      = !Flag(os.O_RDONLY == 0)\n)\n",
		"and of bool enum":              "type Flag bool //#enum\n\nconst (\n\tFlagOn = Flag(os.O_RDONLY == 0)\n\tx      = FlagOn && Flag(os.O_WRONLY == 1)\n)\n",
		"or of bool enum":               "type Flag bool //#enum\n\nconst (\n\tFlagOn = Flag(os.O_RDONLY == 0)\n\tx      = FlagOn || Flag(os.O_WRONLY == 1)\n)\n",
		"max":                           "type Flag int //#enum\n\nconst (\n\tFlagB Flag = 2\n\tx          = max(Flag(os.O_RDONLY), FlagB)\n)\n",
		"min":                           "type Flag int //#enum\n\nconst (\n\tFlagB Flag = 2\n\tx          = min(FlagB, Flag(os.O_RDONLY))\n)\n",
		"null alias":                    "type Flag int //#enum\n\nconst (\n\tFlagRead = Flag(os.O_RDONLY)\n\tx        = FlagRead //#null\n)\n",
	} {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, "package example\n\nimport \"os\"\n\n"+consts)
			_, err := findInFile(fset, files, astFile)
			require.ErrorContains(t, err, "enum const x in test.go:")
			assert.Contains(t, err.Error(), "can't be evaluated because it uses an import")
			assert.NotContains(t, err.Error(), "is no const of an enum type", "a marker of x is no stray marker")
		})
	}

	// //#exclude marks such a const as no enum value, and an alias needs
	// no marker, so their markers are no stray markers of another type
	fset, files, astFile := parseSource(t, "package example\n\nimport \"os\"\n\ntype Flag int //#enum\n\nconst (\n\tFlagRead    = Flag(os.O_RDONLY)\n\tFlagWrite   = Flag(os.O_WRONLY)\n\tFlagRW      = FlagRead | FlagWrite //#exclude\n\tFlagDefault = FlagRead //#exclude\n)\n")
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"FlagRead", "FlagWrite"}, valueNames(enums["Flag"]))

	// An alias of such a const is no enum value, without error,
	// and neither is a const that uses an import without the enum type
	fset, files, astFile = parseSource(t, "package example\n\nimport \"os\"\n\ntype Flag int //#enum\n\nconst (\n\tFlagRead    = Flag(os.O_RDONLY)\n\tFlagDefault = FlagRead\n)\n\nconst maxLen = len(os.DevNull)\n")
	enums, err = findInFile(fset, files, astFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"FlagRead"}, valueNames(enums["Flag"]))
}

func TestFind_NullMarkerAfterFailedEnum(t *testing.T) {
	// A //#null const in the file of the enum type must not get a second,
	// misleading error to move it there when the enum fails for another reason
	fset, files, astFile := parseSource(t, "package example\n\ntype Status string //#enum\n\nconst (\n\tStatusA    Status = \"a\"\n\tStatusB    Status = \"a\"\n\tStatusNone Status = \"\" //#null\n)\n")
	_, err := findInFile(fset, files, astFile)
	require.ErrorContains(t, err, "duplicate enum value")
	assert.NotContains(t, err.Error(), "must be declared in")

	// Also when the enum fails before its values are read
	for _, typ := range []string{"complex128", "= string"} {
		fset, files, astFile := parseSource(t, "package example\n\ntype Status "+typ+" //#enum\n\nconst (\n\tStatusA    Status = \"a\"\n\tStatusNone Status = \"\" //#null\n)\n")
		_, err := findInFile(fset, files, astFile)
		require.Error(t, err, typ)
		assert.NotContains(t, err.Error(), "must be declared in", typ)
	}

	// A //#null const of a type declared per platform names all its files
	fset, files = parseFiles(t, map[string]string{
		"signal_linux.go": "package example\n\ntype Signal int //#enum\n\nconst SignalA Signal = 1\n",
		"signal_other.go": "//go:build !linux\n\npackage example\n\ntype Signal int //#enum\n\nconst SignalB Signal = 2\n",
		"extra_linux.go":  "//go:build linux && cgo\n\npackage example\n\nconst SignalNone Signal = 0 //#null\n",
	})
	_, err = Find(fset, files)
	require.ErrorContains(t, err, "must be declared in signal_linux.go or signal_other.go, the file of the enum type")
}

func TestFind_AnnotationOfOtherToolAfterGofmt(t *testing.T) {
	// gofmt rewrites //#nosec G115 in a doc comment to // #nosec G115,
	// which must still be ignored when recognizing a generated method
	fset, files, astFile := parseSource(t, `package example

type Status string //#enum

const StatusActive Status = "active"

// Valid indicates if s is any of the valid values for Status
//
// #nosec G115
func (s Status) Valid() bool {
	return true
}
`)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"Valid"}, methodNames(enums["Status"].generated))
}

func TestFind_AliasOfConvertedImportedValue(t *testing.T) {
	// An alias of a value converted from an import has the same value,
	// which would be a duplicate case in Valid()
	fset, files, astFile := parseSource(t, `package example

import "net/http"

type Method string //#enum

const (
	MethodGet     = Method(http.MethodGet)
	MethodDefault Method = MethodGet
)
`)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"MethodGet"}, valueNames(enums["Method"]))
}

func TestFind_ReceiverFallbacksExhausted(t *testing.T) {
	// Enum values can use every fixed receiver name candidate
	fset, files, astFile := parseSource(t, `package example

type E int //#enum

const (
	e    E = 1
	recv E = 2
)
`)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	assert.NotEmpty(t, enums["E"].Recv)
	assert.NotContains(t, []string{"e", "recv"}, enums["E"].Recv)
}

func TestFind_NamesUsedByGeneratedMethods(t *testing.T) {
	// Declarations or imports of the package with the names of the packages
	// used by the generated methods would make them refer to the wrong thing
	for name, source := range map[string]string{
		"enum value": `package example

type Status string //#enum

const fmt Status = "fmt"
`,
		"package var": `package example

var json = 1

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`,
		"aliased import": `package example

import json "example.com/fastjson"

var _ = json.Marshal

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`,
		"func": `package example

func json() {}

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`,
		"type": `package example

type fmt int

type Status string //#enum

const StatusA Status = "a"
`,
		// goimports assumes the package name json for the import path
		"unaliased import": `package example

import "github.com/goccy/go-json"

var _ = json.Marshal

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`,
	} {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, source)
			_, err := findInFile(fset, files, astFile)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "used by the generated methods")
		})
	}

	// A declaration in a file with build constraints is compiled together
	// with the generated methods in some builds, which would not compile there.
	// With several names the error is about the first in the files.
	fset, files := parseFiles(t, map[string]string{
		"status.go":         "package example\n\ntype Status string //#enum\n\nconst (\n\tStatusNone Status = \"\" //#null\n\tStatusA Status = \"a\"\n)\n",
		"storage_linux.go":  "package example\n\nconst (\n\tjson   = 1\n\tdriver = \"overlay2\"\n\tbytes  = 2\n)\n",
		"storage_darwin.go": "package example\n\nconst fmt = 1\n",
	})
	for range 10 {
		_, err := Find(fset, files)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fmt in storage_darwin.go:3 has the name of the package fmt")
	}

	// Value: protects=the unaliased json import of an enum file is checked by the names used in that file, also after another enum file without json;
	// fails_when=the names used by the first enum file are cached for the package, so the json use of the second file is missed and its methods use go-json;
	// why_new=the unaliased import case above has one file, so a cache per package instead of per file gives the same names there;
	// seam=none;
	// no_claim=does not build the package, only checks the Find error
	fset, files = parseFiles(t, map[string]string{
		"a.go": "package example\n\ntype Kind int //#enum\n\nconst KindA Kind = 1\n",
		"b.go": "package example\n\nimport \"github.com/goccy/go-json\"\n\nvar _ = json.Marshal\n\ntype Status string //#enum\n\nconst (\n\tStatusNone   Status = \"\" //#null\n\tStatusActive Status = \"active\"\n)\n",
	})
	_, err := Find(fset, files)
	require.ErrorContains(t, err, `import "github.com/goccy/go-json" in b.go:3 has the name of the package encoding/json used by the generated methods`)

	// Only the packages used by the methods that are not //#custom count,
	// like for custom JSON methods with another JSON package, and an
	// import without import name only if its assumed name is used
	for name, source := range map[string]string{
		"method": `package example

type Status string //#enum

const StatusA Status = "a"

func (Status) fmt() {}
`,
		"custom methods": `package example

import "github.com/goccy/go-json"

const driver = "postgres"

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)

//#custom
func (s Status) MarshalJSON() ([]byte, error) { return json.Marshal(string(s)) }

//#custom
func (s *Status) UnmarshalJSON(j []byte) error { return json.Unmarshal(j, (*string)(s)) }

//#custom
func (s Status) Value() (any, error) { return string(s), nil }
`,
		// Two imports can't both have the name json,
		// so json-patch's package name can't be json
		"assumed name of the generator package": `package example

import (
	"encoding/json"

	"github.com/evanphx/json-patch"
)

var (
	_ = json.Valid
	_ = jsonpatch.MergePatch
)

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`,
		"other package name": `package example

import "github.com/evanphx/json-patch"

var _ = jsonpatch.MergePatch

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`,
	} {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, source)
			_, err := findInFile(fset, files, astFile)
			require.NoError(t, err)
		})
	}
}

func TestForAllBuilds_ManyReleaseTags(t *testing.T) {
	// The number of builds is checked before the options of the settings are
	// built, which would need memory quadratic in the number of release tags
	var tags []string
	for i := range 1000 {
		tags = append(tags, fmt.Sprintf("go1.%d", i+1))
	}
	x, err := constraint.Parse("//go:build " + strings.Join(tags, " || "))
	require.NoError(t, err)
	allocs := testing.AllocsPerRun(3, func() {
		forAllBuilds(func(func(constraint.Expr) bool) bool { return true }, x)
	})
	assert.Less(t, allocs, 1000.0)
}

func TestAssumedPackageName(t *testing.T) {
	// Like goimports, because a wrong name would miss or invent
	// a conflict with a package used by the generated methods
	for importPath, name := range map[string]string{
		"encoding/json":                 "json",
		"github.com/goccy/go-json":      "json",
		"gopkg.in/yaml.v3":              "yaml",
		"github.com/x/json/v2":          "json",
		"github.com/evanphx/json-patch": "json",
		"v2":                            "v2",
	} {
		assert.Equal(t, name, assumedPackageName(importPath), importPath)
	}
}

func TestFind_TypeDeclaredTwice(t *testing.T) {
	// The error must name the actual problem, not an alias
	fset, files := parseFiles(t, map[string]string{
		"a.go": "package example\n\ntype S string //#enum\n\nconst SA S = \"a\"\n",
		"b.go": "package example\n\ntype S string //#enum\n",
	})
	_, err := Find(fset, files)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declared more than once")
}

func TestFind_StrayMarkerComments(t *testing.T) {
	// A marker that is not attached to a const or type would be ignored
	// silently, so a sentinel would become valid or nullability get lost
	for name, consts := range map[string]string{
		"exclude above blank line": `const (
	StatusA Status = "a"
	//#exclude

	StatusInvalid Status = ""
)`,
		"null after block": `const (
	StatusA    Status = "a"
	StatusNone Status = ""
) //#null`,
	} {
		t.Run(name, func(t *testing.T) {
			fset, files, astFile := parseSource(t, "package example\n\ntype Status string //#enum\n\n"+consts+"\n")
			_, err := findInFile(fset, files, astFile)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "marker comment")
		})
	}
}

func TestFind_ImportedValueInExpression(t *testing.T) {
	// A const of the enum type from an expression with an import has an
	// invalid type without the imported package and would be dropped
	fset, files, astFile := parseSource(t, `package example

import "net/http"

type Method string //#enum

const (
	MethodGet  = Method(http.MethodGet)
	MethodGetX = Method(http.MethodGet) + "X"
)
`)
	_, err := findInFile(fset, files, astFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MethodGetX")

	// Other consts using the enum type are fine
	fset, files, astFile = parseSource(t, `package example

import "unsafe"

type Method string //#enum

const MethodGet Method = "GET"

const methodSize = unsafe.Sizeof(MethodGet)
`)
	enums, err := findInFile(fset, files, astFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"MethodGet"}, valueNames(enums["Method"]))

	// Value: protects=a const of the enum type by its outermost negation, enum operand or shifted operand fails instead of being dropped as a value;
	// fails_when=hasEnumType loses its unary, ident or shift case, or the bool comparison or shift count case, so it errs on consts of other types;
	// why_new=only a conversion plus a string is checked above; coverage shows the unary, ident, shift and comparison cases of hasEnumType never run;
	// seam=none;
	// no_claim=does not build the package, only checks the Find error and the values
	for expr, wantErr := range map[string]bool{
		// A dropped const of the enum type would silently not be a valid value
		"-Kind(syscall.SIGKILL)":     true,
		"KindA + syscall.SIGKILL":    true,
		"Kind(syscall.SIGKILL) << 1": true,
		// An untyped bool, and a shift with the enum type only as shift
		// count, are no consts of the enum type, so they must not fail
		`Method(http.MethodGet) == "GET"`: false,
		"1 << Kind(syscall.SIGKILL)":      false,
		"len(Method(http.MethodGet))":     false,
	} {
		t.Run(expr, func(t *testing.T) {
			fset, files, astFile := parseSource(t, "package example\n\nimport (\n\t\"net/http\"\n\t\"syscall\"\n)\n\ntype Kind int //#enum\n\nconst KindA Kind = 1\n\ntype Method string //#enum\n\nconst MethodGet Method = \"GET\"\n\nconst x = "+expr+"\n")
			enums, err := findInFile(fset, files, astFile)
			if wantErr {
				require.ErrorContains(t, err, "x in test.go:16 can't be evaluated because it uses an import")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []string{"KindA"}, valueNames(enums["Kind"]))
			assert.Equal(t, []string{"MethodGet"}, valueNames(enums["Method"]))
		})
	}
}
