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

func TestValidateRewrite_MissingMethods(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	// Enum without generated methods
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	// Run ValidateRewrite - should fail
	err = ValidateRewrite(tmpDir, nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing or outdated enum method")
}

func TestValidateRewrite_UpToDateMethods(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	// First generate the methods
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	// Generate methods
	err = Rewrite(tmpDir, nil, nil, false)
	require.NoError(t, err)

	// Now validate - should succeed
	err = ValidateRewrite(tmpDir, nil, false)
	require.NoError(t, err)
}

func TestValidateRewrite_OutdatedMethods(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	// Enum with outdated methods
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)

func (s Status) Valid() bool {
	// Old implementation - only checks for pending
	return s == StatusPending
}
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	// Run ValidateRewrite - should fail because method is outdated
	err = ValidateRewrite(tmpDir, nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing or outdated enum method")
}

func TestValidateRewrite_NoEnums(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "plain.go")

	// File without enums
	source := `package example

type Status string

const (
	StatusPending Status = "pending"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	// Run ValidateRewrite - should succeed (no enums to validate)
	err = ValidateRewrite(tmpDir, nil, false)
	require.NoError(t, err)
}

func TestValidateRewrite_DoesNotModifyFiles(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	// Enum without generated methods
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	// Run ValidateRewrite - should fail but not modify file
	err = ValidateRewrite(tmpDir, nil, false)
	require.Error(t, err)

	// Read file back - should be unchanged
	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, source, string(content), "ValidateRewrite should not modify files")
}

func TestValidateRewrite_MultipleEnums(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "types.go")

	// Multiple enums, one with methods, one without
	source := `package example

import "fmt"

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)

func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusActive:
		return true
	}
	return false
}

func (s Status) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("invalid example.Status: %q", s)
	}
	return nil
}

func (Status) Enums() []Status {
	return []Status{StatusPending, StatusActive}
}

func (Status) EnumStrings() []string {
	return []string{"pending", "active"}
}

func (s Status) String() string {
	return string(s)
}

type Priority int //#enum

const (
	PriorityLow  Priority = 1
	PriorityHigh Priority = 2
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	// Run ValidateRewrite - should fail because Priority is missing methods
	err = ValidateRewrite(tmpDir, nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing or outdated enum method")
}

func TestValidateRewrite_VerboseOutput(t *testing.T) {
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
	err = ValidateRewrite(tmpDir, &verbose, false)
	require.Error(t, err)

	verboseStr := verbose.String()
	// Verbose output should contain useful information
	assert.NotEmpty(t, verboseStr)
}

func TestValidateRewrite_OutdatedAfterAddingEnumValue(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	// First, create enum with 2 values and generate methods
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	// Generate methods
	err = Rewrite(tmpDir, nil, nil, false)
	require.NoError(t, err)

	// Validate - should pass
	err = ValidateRewrite(tmpDir, nil, false)
	require.NoError(t, err, "validation should pass after initial generation")

	// Now add a third enum value
	sourceWithNewValue := `package example

import "fmt"

type Status string //#enum

const (
	StatusPending   Status = "pending"
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusActive:
		return true
	}
	return false
}

func (s Status) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("invalid example.Status: %q", s)
	}
	return nil
}

func (Status) Enums() []Status {
	return []Status{StatusPending, StatusActive}
}

func (Status) EnumStrings() []string {
	return []string{"pending", "active"}
}

func (s Status) String() string {
	return string(s)
}
`

	err = os.WriteFile(testFile, []byte(sourceWithNewValue), 0644)
	require.NoError(t, err)

	// Validate - should now fail because methods don't include StatusCompleted
	err = ValidateRewrite(tmpDir, nil, false)
	require.Error(t, err, "validation should fail when new enum value is added but methods aren't updated")
	assert.Contains(t, err.Error(), "missing or outdated enum method")
}

func TestValidateRewrite_OutdatedAfterChangingEnumValue(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	// First, create enum and generate methods
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	// Generate methods
	err = Rewrite(tmpDir, nil, nil, false)
	require.NoError(t, err)

	// Validate - should pass
	err = ValidateRewrite(tmpDir, nil, false)
	require.NoError(t, err, "validation should pass after initial generation")

	// Now change one of the enum values
	sourceWithChangedValue := `package example

import "fmt"

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "running"
)

func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusActive:
		return true
	}
	return false
}

func (s Status) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("invalid example.Status: %q", s)
	}
	return nil
}

func (Status) Enums() []Status {
	return []Status{StatusPending, StatusActive}
}

func (Status) EnumStrings() []string {
	return []string{"pending", "active"}
}

func (s Status) String() string {
	return string(s)
}
`

	err = os.WriteFile(testFile, []byte(sourceWithChangedValue), 0644)
	require.NoError(t, err)

	// Validate - should fail because EnumStrings still has "active" instead of "running"
	err = ValidateRewrite(tmpDir, nil, false)
	require.Error(t, err, "validation should fail when enum value is changed but methods aren't updated")
	assert.Contains(t, err.Error(), "missing or outdated enum method")
}

func TestValidateRewrite_OutdatedAfterRemovingEnumValue(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	// First, create enum with 3 values and generate methods
	source := `package example

type Status string //#enum

const (
	StatusPending   Status = "pending"
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)
`

	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)

	// Generate methods
	err = Rewrite(tmpDir, nil, nil, false)
	require.NoError(t, err)

	// Validate - should pass
	err = ValidateRewrite(tmpDir, nil, false)
	require.NoError(t, err, "validation should pass after initial generation")

	// Now remove one enum value
	sourceWithRemovedValue := `package example

import "fmt"

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)

func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusActive, StatusCompleted:
		return true
	}
	return false
}

func (s Status) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("invalid example.Status: %q", s)
	}
	return nil
}

func (Status) Enums() []Status {
	return []Status{StatusPending, StatusActive, StatusCompleted}
}

func (Status) EnumStrings() []string {
	return []string{"pending", "active", "completed"}
}

func (s Status) String() string {
	return string(s)
}
`

	err = os.WriteFile(testFile, []byte(sourceWithRemovedValue), 0644)
	require.NoError(t, err)

	// Validate - should fail because methods still reference StatusCompleted
	err = ValidateRewrite(tmpDir, nil, false)
	require.Error(t, err, "validation should fail when enum value is removed but methods aren't updated")
	assert.Contains(t, err.Error(), "missing or outdated enum method")
}

// TestRewriteNoOpDoesNotReorderImports verifies that running Rewrite on a
// file whose enum methods are already up to date does NOT modify the file —
// even when the file's imports are not in the order goimports would produce.
// Import ordering belongs to gofmt/goimports, not to the enum generator.
//
// Regression test: previously, even a no-op enum rewrite would unconditionally
// run FormatFileWithImports, which reorders imports as a side effect.
func TestRewriteNoOpDoesNotReorderImports(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	// Generate methods first so the file has up-to-date enum methods.
	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)
`
	err := os.WriteFile(testFile, []byte(source), 0644)
	require.NoError(t, err)
	require.NoError(t, Rewrite(tmpDir, nil, nil, false))

	// Read the freshly-generated file, then deliberately scramble its
	// import order so it is no longer goimports-sorted.
	generated, err := os.ReadFile(testFile)
	require.NoError(t, err)

	// Replace the generated import block with an intentionally unsorted one.
	// `fmt` should sort before any third-party import alphabetically; we put
	// a third-party import before it, then add an unused alias to ensure the
	// imports group is non-trivial. Use only stdlib imports to avoid needing
	// extra deps in the test module.
	scrambled := bytes.Replace(
		generated,
		[]byte(`import "fmt"`),
		[]byte("import (\n\t\"strings\"\n\n\t\"fmt\"\n)\n\nvar _ = strings.ToUpper"),
		1,
	)
	require.NotEqual(t, generated, scrambled, "test setup must actually scramble imports")
	require.NoError(t, os.WriteFile(testFile, scrambled, 0644))

	// Run Rewrite again. Methods are already up to date, so the file must
	// be left byte-identical — no import reordering allowed.
	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	after, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, string(scrambled), string(after), "no-op Rewrite must not reorder imports")

	// ValidateRewrite must also accept the file (no false positives from
	// the unsorted imports).
	require.NoError(t, ValidateRewrite(tmpDir, nil, false))
}

// TestRewrite_CustomMarkerPreservesMethod verifies that a method tagged with
// //#custom in its doc comment is neither replaced nor regenerated — it stays
// verbatim, and no duplicate of it appears in the generated output.
func TestRewrite_CustomMarkerPreservesMethod(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	// Nullable string enum with a hand-written UnmarshalJSON that would
	// normally be regenerated, but is tagged //#custom.
	source := `package example

import (
	"bytes"
	"encoding/json"
)

type Status string //#enum

const (
	StatusNone     Status = "" //#null
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

// UnmarshalJSON accepts legacy bool true/false plus the regular string form.
//
//#custom
func (s *Status) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("true")) {
		*s = StatusActive
		return nil
	}
	if bytes.Equal(data, []byte("false")) {
		*s = StatusNone
		return nil
	}
	return json.Unmarshal(data, (*string)(s))
}
`

	require.NoError(t, os.WriteFile(testFile, []byte(source), 0644))

	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	written, err := os.ReadFile(testFile)
	require.NoError(t, err)
	result := string(written)

	// The hand-written method body must survive untouched.
	assert.Contains(t, result, `if bytes.Equal(data, []byte("true")) {`,
		"custom UnmarshalJSON body must be preserved")
	assert.Contains(t, result, `*s = StatusActive`,
		"custom branch must be preserved")

	// The //#custom marker must remain in the doc comment so re-generation
	// continues to recognize it. gofmt may normalize "//#custom" to
	// "// #custom" (with a space); both are accepted by isCustom.
	assert.True(t,
		strings.Contains(result, "//#custom") || strings.Contains(result, "// #custom"),
		"//#custom marker must remain in doc comment (possibly gofmt-normalised)")

	// Exactly one UnmarshalJSON must exist — no duplicate from template.
	assert.Equal(t, 1, strings.Count(result, ") UnmarshalJSON("),
		"must not generate a second UnmarshalJSON")

	// Generated companions (non-custom) must still appear.
	assert.Contains(t, result, "func (s Status) IsNull() bool")
	assert.Contains(t, result, "func (s Status) IsNotNull() bool")
	assert.Contains(t, result, "func (s *Status) SetNull()")
	assert.Contains(t, result, "func (s Status) MarshalJSON() ([]byte, error)")
	assert.Contains(t, result, "func (s *Status) Scan(value any) error")
	assert.Contains(t, result, "func (s Status) Value() (driver.Value, error)")
	assert.Contains(t, result, "func (s Status) Valid() bool")

	// Running again must be a no-op (idempotent).
	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	secondPass, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, string(written), string(secondPass),
		"second Rewrite must be byte-identical (idempotent)")

	// ValidateRewrite must also accept the file — no false "outdated" error.
	require.NoError(t, ValidateRewrite(tmpDir, nil, false))
}

// TestRewrite_CustomMarkerOnAlwaysGeneratedMethods verifies that `//#custom`
// on any of the always-generated methods (Valid, Validate, Enums, EnumStrings)
// preserves the hand-written body and does not produce a duplicate. These
// methods live in their own templates and must each be individually skippable.
func TestRewrite_CustomMarkerOnAlwaysGeneratedMethods(t *testing.T) {
	cases := []struct {
		name       string
		methodDecl string
		signature  string
	}{
		{
			name: "Valid",
			methodDecl: `//#custom
func (s Status) Valid() bool {
	// hand-written liberal check: any non-empty value is valid
	return s != ""
}`,
			signature: ") Valid() bool",
		},
		{
			name: "Validate",
			methodDecl: `//#custom
func (s Status) Validate() error {
	// hand-written: never errors
	return nil
}`,
			signature: ") Validate() error",
		},
		{
			name: "Enums",
			methodDecl: `//#custom
func (Status) Enums() []Status {
	// hand-written: returns only the first value
	return []Status{StatusActive}
}`,
			signature: ") Enums() []Status",
		},
		{
			name: "EnumStrings",
			methodDecl: `//#custom
func (Status) EnumStrings() []string {
	// hand-written: lowercases every value
	return []string{"active", "inactive"}
}`,
			signature: ") EnumStrings() []string",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			testFile := filepath.Join(tmpDir, "status.go")

			source := `package example

type Status string //#enum

const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

` + tc.methodDecl + `
`
			require.NoError(t, os.WriteFile(testFile, []byte(source), 0644))

			require.NoError(t, Rewrite(tmpDir, nil, nil, false))
			written, err := os.ReadFile(testFile)
			require.NoError(t, err)
			result := string(written)

			// Exactly one declaration of the custom method — no duplicate.
			assert.Equal(t, 1, strings.Count(result, tc.signature),
				"must not generate a second %s", tc.name)

			// Marker must survive (gofmt may normalise it).
			assert.True(t,
				strings.Contains(result, "//#custom") || strings.Contains(result, "// #custom"),
				"//#custom marker must remain in doc comment")

			// The other three always-generated methods must still be emitted.
			allSigs := []string{") Valid() bool", ") Validate() error", ") Enums() []Status", ") EnumStrings() []string"}
			for _, other := range allSigs {
				if other == tc.signature {
					continue
				}
				assert.Equal(t, 1, strings.Count(result, other),
					"non-custom %s must be generated exactly once", other)
			}

			// Second Rewrite must be idempotent.
			require.NoError(t, Rewrite(tmpDir, nil, nil, false))
			secondPass, err := os.ReadFile(testFile)
			require.NoError(t, err)
			assert.Equal(t, string(written), string(secondPass),
				"second Rewrite must be byte-identical (idempotent)")

			// ValidateRewrite must accept the file.
			require.NoError(t, ValidateRewrite(tmpDir, nil, false))
		})
	}
}
