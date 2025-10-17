package enums

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewrite_BasicStringEnum(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	source := `package example

// Status represents order status
type Status string //#enum

const (
	StatusPending   Status = "pending"
	StatusConfirmed Status = "confirmed"
	StatusShipped   Status = "shipped"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	// Run Rewrite
	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Check that methods were generated
	assert.Contains(t, result, "func (s Status) Valid() bool")
	assert.Contains(t, result, "func (s Status) Validate() error")
	assert.Contains(t, result, "func (Status) Enums() []Status")
	assert.Contains(t, result, "func (Status) EnumStrings() []string")
	assert.Contains(t, result, "func (s Status) String() string")

	// Check that it validates all enum values
	assert.Contains(t, result, "StatusPending")
	assert.Contains(t, result, "StatusConfirmed")
	assert.Contains(t, result, "StatusShipped")

	// Should not have nullable methods
	assert.NotContains(t, result, "IsNull")
	assert.NotContains(t, result, "MarshalJSON")
}

func TestRewrite_BasicIntEnum(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "priority.go")

	source := `package example

type Priority int //#enum

const (
	PriorityLow  Priority = 1
	PriorityMid  Priority = 2
	PriorityHigh Priority = 3
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Check that methods were generated
	assert.Contains(t, result, "func (p Priority) Valid() bool")
	assert.Contains(t, result, "func (p Priority) Validate() error")
	assert.Contains(t, result, "func (Priority) Enums() []Priority")

	// Should not have String method for int enums
	assert.NotContains(t, result, "func (p Priority) String() string")

	// Should have all enum values
	assert.Contains(t, result, "PriorityLow")
	assert.Contains(t, result, "PriorityMid")
	assert.Contains(t, result, "PriorityHigh")
}

func TestRewrite_NullableEnum(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "priority.go")

	source := `package example

type Priority int //#enum

const (
	PriorityNull Priority = 0 //#null
	PriorityLow  Priority = 1
	PriorityHigh Priority = 2
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Check nullable methods
	assert.Contains(t, result, "func (p Priority) IsNull() bool")
	assert.Contains(t, result, "func (p Priority) IsNotNull() bool")
	assert.Contains(t, result, "func (p *Priority) SetNull()")
	assert.Contains(t, result, "func (p Priority) MarshalJSON() ([]byte, error)")
	assert.Contains(t, result, "func (p *Priority) UnmarshalJSON(j []byte) error")
	assert.Contains(t, result, "func (p *Priority) Scan(value any) error")
	assert.Contains(t, result, "func (p Priority) Value() (driver.Value, error)")

	// Check null handling
	assert.Contains(t, result, "return p == PriorityNull")
	assert.Contains(t, result, "*p = PriorityNull")
}

func TestRewrite_NullableStringEnum(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	source := `package example

type Status string //#enum

const (
	StatusNull    Status = "" //#null
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Check nullable methods
	assert.Contains(t, result, "func (s Status) IsNull() bool")
	assert.Contains(t, result, "func (s Status) MarshalJSON() ([]byte, error)")
	assert.Contains(t, result, "func (s *Status) Scan(value any) error")

	// String enum nullable should have string-specific Scan implementation
	assert.Contains(t, result, "switch value := value.(type)")
	assert.Contains(t, result, "case string:")
	assert.Contains(t, result, "case []byte:")
}

func TestRewrite_JSONSchema(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "color.go")

	source := `package example

type Color string //#enum,jsonschema

const (
	ColorRed   Color = "red"
	ColorGreen Color = "green"
	ColorBlue  Color = "blue"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Check JSONSchema method
	assert.Contains(t, result, "func (Color) JSONSchema() *jsonschema.Schema")
	assert.Contains(t, result, `Type: "string"`)
	assert.Contains(t, result, "Enum: []any{")
	assert.Contains(t, result, `"red"`)
	assert.Contains(t, result, `"green"`)
	assert.Contains(t, result, `"blue"`)
}

func TestRewrite_NullableJSONSchema(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "size.go")

	source := `package example

type Size int //#enum,jsonschema

const (
	SizeNull  Size = 0 //#null
	SizeSmall Size = 1
	SizeLarge Size = 2
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Nullable JSON Schema should use oneOf
	assert.Contains(t, result, "OneOf: []*jsonschema.Schema{")
	assert.Contains(t, result, `Type: "number"`)
	assert.Contains(t, result, `{Type: "null"}`)
	assert.Contains(t, result, "Default: SizeNull")

	// Null value should not be in enum list
	assert.Contains(t, result, "1,")
	assert.Contains(t, result, "2,")
	// The literal "0" followed by comma should not appear in Enum array
	enumSection := result[strings.Index(result, "Enum: []any{"):]
	enumSection = enumSection[:strings.Index(enumSection, "},")]
	assert.NotContains(t, enumSection, "0,")
}

func TestRewrite_ReplaceExistingMethods(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)

func (s Status) Valid() bool {
	// Old implementation
	return s == StatusPending
}

func (s Status) String() string {
	// Old implementation
	return "OLD: " + string(s)
}
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Old implementations should be replaced
	assert.NotContains(t, result, "Old implementation")
	assert.NotContains(t, result, "OLD:")

	// New implementations should be present
	assert.Contains(t, result, "func (s Status) Valid() bool")
	assert.Contains(t, result, "switch s {")
	assert.Contains(t, result, "StatusPending")
	assert.Contains(t, result, "StatusActive")
}

func TestRewrite_MultipleEnums(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "types.go")

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
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Both enums should have methods
	assert.Contains(t, result, "func (s Status) Valid() bool")
	assert.Contains(t, result, "func (p Priority) Valid() bool")
	assert.Contains(t, result, "func (Status) Enums() []Status")
	assert.Contains(t, result, "func (Priority) Enums() []Priority")
}

func TestRewrite_NoEnumsFound(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "plain.go")

	source := `package example

type Status string

const (
	StatusPending Status = "pending"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// No methods should be generated
	assert.Empty(t, result)
}

func TestRewrite_WriteToFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	// Run Rewrite without resultOut (writes to file)
	err = Rewrite(tmpDir, nil, nil, false)
	require.NoError(t, err)

	// Read the file back
	content, err := os.ReadFile(testFile)
	require.NoError(t, err)

	result := string(content)

	// Check that methods were written to file
	assert.Contains(t, result, "func (s Status) Valid() bool")
	assert.Contains(t, result, "const (")
	assert.Contains(t, result, "StatusPending")
}

func TestRewrite_VerboseOutput(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var verbose bytes.Buffer
	var output bytes.Buffer
	err = Rewrite(tmpDir, &verbose, &output, false)
	require.NoError(t, err)

	verboseStr := verbose.String()

	// Verbose output should contain useful information
	// (actual content depends on astvisit implementation)
	assert.NotEmpty(t, verboseStr)
}

func TestRewrite_DebugMode(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, true)
	require.NoError(t, err)

	result := output.String()

	// Debug mode should add debug comments
	// (actual format depends on astvisit implementation)
	assert.Contains(t, result, "Replacement for Status")
}

func TestRewrite_EnumStrings(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Check EnumStrings method
	assert.Contains(t, result, "func (Status) EnumStrings() []string")
	assert.Contains(t, result, `"pending"`)
	assert.Contains(t, result, `"active"`)
}

func TestRewrite_IntEnumStrings(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "priority.go")

	source := `package example

type Priority int //#enum

const (
	PriorityLow  Priority = 1
	PriorityHigh Priority = 2
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Int enums should have string representations without quotes in EnumStrings
	assert.Contains(t, result, "func (Priority) EnumStrings() []string")
	assert.Contains(t, result, `"1"`)
	assert.Contains(t, result, `"2"`)
}

func TestRewrite_ValidateError(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Validate should return proper error with package and type name
	assert.Contains(t, result, "func (s Status) Validate() error")
	assert.Contains(t, result, "example.Status")
	assert.Contains(t, result, "fmt.Errorf")
}

func TestRewrite_ImportsAdded(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Should add fmt import
	assert.Contains(t, result, `"fmt"`)
}

func TestRewrite_NullableImportsAdded(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "priority.go")

	source := `package example

type Priority int //#enum

const (
	PriorityNull Priority = 0 //#null
	PriorityLow  Priority = 1
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Should add nullable-related imports
	assert.Contains(t, result, `"bytes"`)
	assert.Contains(t, result, `"encoding/json"`)
	assert.Contains(t, result, `"database/sql/driver"`)
}

func TestRewrite_JSONSchemaImportAdded(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "color.go")

	source := `package example

type Color string //#enum,jsonschema

const (
	ColorRed Color = "red"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	var output bytes.Buffer
	err = Rewrite(tmpDir, nil, &output, false)
	require.NoError(t, err)

	result := output.String()

	// Should add jsonschema import
	assert.Contains(t, result, `"github.com/invopop/jsonschema"`)
}
