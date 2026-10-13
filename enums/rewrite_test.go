package enums

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// Valid indicates if s is any of the valid values for Status
func (s Status) Valid() bool {
	// Old implementation
	return s == StatusPending
}

// String implements the fmt.Stringer interface for Status
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
	assert.Contains(t, err.Error(), "enum type(s) with missing or outdated methods")
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

// Valid indicates if s is any of the valid values for Status
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
	assert.Contains(t, err.Error(), "enum type(s) with missing or outdated methods")
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

// Valid indicates if s is any of the valid values for Status
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusActive:
		return true
	}
	return false
}

// Validate returns an error if s is none of the valid values for Status
func (s Status) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("invalid example.Status: %q", s)
	}
	return nil
}

// Enums returns all valid values for Status
func (Status) Enums() []Status {
	return []Status{StatusPending, StatusActive}
}

// EnumStrings returns all valid values for Status as strings
func (Status) EnumStrings() []string {
	return []string{"pending", "active"}
}

// String implements the fmt.Stringer interface for Status
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
	assert.Contains(t, err.Error(), "enum type(s) with missing or outdated methods")
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

// Valid indicates if s is any of the valid values for Status
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusActive:
		return true
	}
	return false
}

// Validate returns an error if s is none of the valid values for Status
func (s Status) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("invalid example.Status: %q", s)
	}
	return nil
}

// Enums returns all valid values for Status
func (Status) Enums() []Status {
	return []Status{StatusPending, StatusActive}
}

// EnumStrings returns all valid values for Status as strings
func (Status) EnumStrings() []string {
	return []string{"pending", "active"}
}

// String implements the fmt.Stringer interface for Status
func (s Status) String() string {
	return string(s)
}
`

	err = os.WriteFile(testFile, []byte(sourceWithNewValue), 0644)
	require.NoError(t, err)

	// Validate - should now fail because methods don't include StatusCompleted
	err = ValidateRewrite(tmpDir, nil, false)
	require.Error(t, err, "validation should fail when new enum value is added but methods aren't updated")
	assert.Contains(t, err.Error(), "enum type(s) with missing or outdated methods")
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

// Valid indicates if s is any of the valid values for Status
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusActive:
		return true
	}
	return false
}

// Validate returns an error if s is none of the valid values for Status
func (s Status) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("invalid example.Status: %q", s)
	}
	return nil
}

// Enums returns all valid values for Status
func (Status) Enums() []Status {
	return []Status{StatusPending, StatusActive}
}

// EnumStrings returns all valid values for Status as strings
func (Status) EnumStrings() []string {
	return []string{"pending", "active"}
}

// String implements the fmt.Stringer interface for Status
func (s Status) String() string {
	return string(s)
}
`

	err = os.WriteFile(testFile, []byte(sourceWithChangedValue), 0644)
	require.NoError(t, err)

	// Validate - should fail because EnumStrings still has "active" instead of "running"
	err = ValidateRewrite(tmpDir, nil, false)
	require.Error(t, err, "validation should fail when enum value is changed but methods aren't updated")
	assert.Contains(t, err.Error(), "enum type(s) with missing or outdated methods")
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

// Valid indicates if s is any of the valid values for Status
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusActive, StatusCompleted:
		return true
	}
	return false
}

// Validate returns an error if s is none of the valid values for Status
func (s Status) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("invalid example.Status: %q", s)
	}
	return nil
}

// Enums returns all valid values for Status
func (Status) Enums() []Status {
	return []Status{StatusPending, StatusActive, StatusCompleted}
}

// EnumStrings returns all valid values for Status as strings
func (Status) EnumStrings() []string {
	return []string{"pending", "active", "completed"}
}

// String implements the fmt.Stringer interface for Status
func (s Status) String() string {
	return string(s)
}
`

	err = os.WriteFile(testFile, []byte(sourceWithRemovedValue), 0644)
	require.NoError(t, err)

	// Validate - should fail because methods still reference StatusCompleted
	err = ValidateRewrite(tmpDir, nil, false)
	require.Error(t, err, "validation should fail when enum value is removed but methods aren't updated")
	assert.Contains(t, err.Error(), "enum type(s) with missing or outdated methods")
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

// TestRewrite_DocCommentsWithPeriod verifies that generated methods whose
// doc comment lines got a period at the end, like by `godot -fix`, are still
// recognized as generated, else the next run would fail with
// "was not generated by go-enum" for every method.
func TestRewrite_DocCommentsWithPeriod(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	generated := readFile(t, dir, "status.go")

	withPeriods := regexp.MustCompile(`(?m)^(// .*[^.])$`).ReplaceAllString(generated, "$1.")
	require.NotEqual(t, generated, withPeriods)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(withPeriods), 0644))
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Equal(t, generated, readFile(t, dir, "status.go"))
}

// TestRewrite_AllMethodsCustom verifies that an enum whose methods are all
// marked with //#custom gets no generated methods and no imports, so the
// file stays byte-identical instead of being reformatted.
func TestRewrite_AllMethodsCustom(t *testing.T) {
	source := `package example

type Status string //#enum

const StatusA Status = "a"

//#custom
func (s Status) Valid() bool { return s == StatusA }

//#custom
func (s Status) Validate() error { return nil }

//#custom
func (Status) Enums() []Status { return []Status{StatusA} }

//#custom
func (Status) EnumStrings() []string { return []string{"a"} }

//#custom
func (s Status) String() string { return string(s) }
`
	dir := writeFiles(t, map[string]string{"status.go": source})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Equal(t, source, readFile(t, dir, "status.go"))
	require.NoError(t, ValidateRewrite(dir, nil, false))
}

// TestRewrite_AnnotationOfOtherTool verifies that a generated method with
// the comment of another tool in its doc comment, written by gofmt as
// // #nosec G115, is still replaced, else the run would fail with
// "was not generated by go-enum".
func TestRewrite_AnnotationOfOtherTool(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

type Status string //#enum

const StatusA Status = "a"
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	generated := readFile(t, dir, "status.go")
	annotated := strings.Replace(generated, "func (s Status) Valid() bool {", "//\n// #nosec G115\nfunc (s Status) Valid() bool {", 1)
	require.NotEqual(t, generated, annotated)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(annotated), 0644))
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Equal(t, generated, readFile(t, dir, "status.go"), "the annotation is removed with the replaced method")
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
// Valid indicates if s is any of the valid values for Status
func (s Status) Valid() bool {
	// hand-written liberal check: any non-empty value is valid
	return s != ""
}`,
			signature: ") Valid() bool",
		},
		{
			name: "Validate",
			methodDecl: `//#custom
// Validate returns an error if s is none of the valid values for Status
func (s Status) Validate() error {
	// hand-written: never errors
	return nil
}`,
			signature: ") Validate() error",
		},
		{
			name: "Enums",
			methodDecl: `//#custom
// Enums returns all valid values for Status
func (Status) Enums() []Status {
	// hand-written: returns only the first value
	return []Status{StatusActive}
}`,
			signature: ") Enums() []Status",
		},
		{
			name: "EnumStrings",
			methodDecl: `//#custom
// EnumStrings returns all valid values for Status as strings
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

// TestRewrite_ValuesFromOtherFiles verifies that enum values declared in
// other files of the package are part of the generated methods, which are
// written only into the file of the enum type.
func TestRewrite_ValuesFromOtherFiles(t *testing.T) {
	tmpDir := t.TempDir()
	typeFile := filepath.Join(tmpDir, "status.go")
	valuesFile := filepath.Join(tmpDir, "a_status_values.go")

	typeSource := `package example

type Status string //#enum

type Other int
`
	valuesSource := `package example

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)
`
	require.NoError(t, os.WriteFile(typeFile, []byte(typeSource), 0644))
	require.NoError(t, os.WriteFile(valuesFile, []byte(valuesSource), 0644))

	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	written, err := os.ReadFile(typeFile)
	require.NoError(t, err)
	result := string(written)

	assert.Contains(t, result, "type Status string //#enum\n", "//#enum marker must stay on the type line")
	assert.Contains(t, result, "func (s Status) Valid() bool")
	assert.Contains(t, result, "StatusPending,\n\t\tStatusActive:")

	values, err := os.ReadFile(valuesFile)
	require.NoError(t, err)
	assert.Equal(t, valuesSource, string(values), "file with only enum values must not be modified")

	// Running again must be a no-op (idempotent).
	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	secondPass, err := os.ReadFile(typeFile)
	require.NoError(t, err)
	assert.Equal(t, result, string(secondPass), "second Rewrite must be byte-identical (idempotent)")
	require.NoError(t, ValidateRewrite(tmpDir, nil, false))

	// Adding a value in the other file must make the methods outdated.
	require.NoError(t, os.WriteFile(valuesFile, []byte(valuesSource+`
const StatusDone Status = "done"
`), 0644))
	require.ErrorContains(t, ValidateRewrite(tmpDir, nil, false), "enum type(s) with missing or outdated methods")
}

// TestRewrite_ExcludeMarker verifies that consts marked with //#exclude
// are not valid values and that the marker of a single-line const stays
// on its line when the methods are inserted after that const.
func TestRewrite_ExcludeMarker(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	source := `package example

type Status string //#enum

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)

const StatusInvalid Status = "" //#exclude
`
	require.NoError(t, os.WriteFile(testFile, []byte(source), 0644))

	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	written, err := os.ReadFile(testFile)
	require.NoError(t, err)
	result := string(written)

	assert.Contains(t, result, "const StatusInvalid Status = \"\" //#exclude\n\n// Valid indicates")
	assert.Equal(t, 1, strings.Count(result, "StatusInvalid"), "excluded const must not be used in generated methods")

	// Running again must be a no-op (idempotent).
	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	secondPass, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, result, string(secondPass), "second Rewrite must be byte-identical (idempotent)")
	require.NoError(t, ValidateRewrite(tmpDir, nil, false))
}

// TestRewrite_NullMarkerOnSingleLineConst is a regression test: methods
// used to be inserted between a single-line const and its trailing //#null
// comment, which moved the marker behind the methods so that the next
// Rewrite no longer treated the enum as nullable.
func TestRewrite_NullMarkerOnSingleLineConst(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")

	source := `package example

type Status string //#enum

const StatusActive Status = "active"
const StatusNone Status = "" //#null
`
	require.NoError(t, os.WriteFile(testFile, []byte(source), 0644))

	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	written, err := os.ReadFile(testFile)
	require.NoError(t, err)
	result := string(written)

	assert.Contains(t, result, "const StatusNone Status = \"\" //#null\n")
	assert.Contains(t, result, "func (s Status) IsNull() bool")

	// Running again must be a no-op (idempotent).
	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	secondPass, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, result, string(secondPass), "second Rewrite must be byte-identical (idempotent)")
	require.NoError(t, ValidateRewrite(tmpDir, nil, false))
}

// TestRewrite_EnumsSharingInsertionPoint is a regression test: when the
// methods of several enums are inserted after the same declaration, like
// enum types declared in one type block with their values in another file,
// or values of several enums in one const block, the separate insertions
// at one position used to panic in go-astvisit.
func TestRewrite_EnumsSharingInsertionPoint(t *testing.T) {
	cases := map[string]map[string]string{
		"type block": {
			"types.go": `package example

type (
	A string //#enum
	B string //#enum
)
`,
			"values.go": `package example

const (
	A1 A = "a1"
	B1 B = "b1"
)
`,
		},
		"const block": {
			"ab.go": `package example

type A string //#enum

type B string //#enum

const (
	A1 A = "a1"
	B1 B = "b1"
)
`,
		},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			for fileName, source := range files {
				require.NoError(t, os.WriteFile(filepath.Join(tmpDir, fileName), []byte(source), 0644))
			}

			// Repeated because a random order would often match by chance
			for range 20 {
				var output bytes.Buffer
				require.NoError(t, Rewrite(tmpDir, nil, &output, false))
				result := output.String()
				assert.Equal(t, 1, strings.Count(result, "func (a A) Valid() bool"))
				assert.Equal(t, 1, strings.Count(result, "func (b B) Valid() bool"))
				assert.Less(t, strings.Index(result, "func (a A) Valid() bool"), strings.Index(result, "func (b B) Valid() bool"),
					"methods must be ordered deterministically")
			}

			// Generated output must be stable
			require.NoError(t, Rewrite(tmpDir, nil, nil, false))
			require.NoError(t, ValidateRewrite(tmpDir, nil, false))
		})
	}
}

// TestRewrite_IgnoredFilesNotCollected verifies that consts from files
// the go tool ignores are not collected, because the generated methods
// would reference consts that are undefined in a regular build.
func TestRewrite_IgnoredFilesNotCollected(t *testing.T) {
	tmpDir := t.TempDir()
	typeFile := filepath.Join(tmpDir, "status.go")
	require.NoError(t, os.WriteFile(typeFile, []byte(`package example

type Status string //#enum

const StatusActive Status = "active"
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "_old_status.go"), []byte(`package example

const StatusLegacy Status = "legacy"
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "status_test.go"), []byte(`package example

const StatusTestOnly Status = "test-only"
`), 0644))

	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	written, err := os.ReadFile(typeFile)
	require.NoError(t, err)
	assert.Contains(t, string(written), "func (s Status) Valid() bool")
	assert.NotContains(t, string(written), "StatusLegacy")
	assert.NotContains(t, string(written), "StatusTestOnly")
}

// TestRewrite_SingleSpecParenConstWithLineComment verifies that methods
// are inserted after the closing parenthesis of a const block with one
// spec, not after the line comment inside the parentheses.
func TestRewrite_SingleSpecParenConstWithLineComment(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "status.go")
	source := `package example

type Status string //#enum

const (
	StatusActive Status = "active" //#null
)
`
	require.NoError(t, os.WriteFile(testFile, []byte(source), 0644))

	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	written, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Contains(t, string(written), "\tStatusActive Status = \"active\" //#null\n)\n")
	require.NoError(t, ValidateRewrite(tmpDir, nil, false))
}

// requireBuilds fails the test if the package in dir does not build,
// because the generated methods must never break the build of a package.
func requireBuilds(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.24\n"), 0644))
	cmd := exec.Command("go", "build", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "package with generated methods must build:\n%s", output)
}

// TestRewrite_AliasInOtherFile verifies that an alias of an enum value
// in another file, which was never collected before values were read
// from all files, still generates a package that builds.
func TestRewrite_AliasInOtherFile(t *testing.T) {
	tmpDir := t.TempDir()
	typeFile := filepath.Join(tmpDir, "status.go")
	require.NoError(t, os.WriteFile(typeFile, []byte(`package example

type Status string //#enum

const (
	StatusActive  Status = "active"
	StatusPending Status = "pending"
)
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "defaults.go"), []byte(`package example

const StatusDefault Status = StatusPending
`), 0644))

	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	written, err := os.ReadFile(typeFile)
	require.NoError(t, err)
	assert.Contains(t, string(written), "StatusActive,\n\t\tStatusPending:")
	assert.NotContains(t, string(written), "StatusDefault")
	requireBuilds(t, tmpDir)
}

// TestRewrite_ConstrainedFilesNotCollected verifies that values and type
// declarations in files that are not compiled on every platform don't
// leak into the generated methods, which are compiled on every platform.
func TestRewrite_ConstrainedFilesNotCollected(t *testing.T) {
	tmpDir := t.TempDir()
	typeFile := filepath.Join(tmpDir, "status.go")
	require.NoError(t, os.WriteFile(typeFile, []byte(`package example

type Status string //#enum

const StatusActive Status = "active"
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "status_values.go"), []byte(`package example

const StatusDone Status = "done"
`), 0644))
	// The tests never run on plan9, so this file is never compiled
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "status_plan9.go"), []byte(`package example

const StatusPlan9 Status = "plan9"
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "legacy.go"), []byte(`//go:build ignore

package example

type Status int

const StatusLegacy Status = 1
`), 0644))

	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	written, err := os.ReadFile(typeFile)
	require.NoError(t, err)
	assert.Contains(t, string(written), "StatusActive,\n\t\tStatusDone:")
	assert.NotContains(t, string(written), "StatusPlan9")
	assert.NotContains(t, string(written), "StatusLegacy")
	requireBuilds(t, tmpDir)
}

// TestRewrite_CustomMethodInOtherFile verifies that a //#custom method
// in another file of the package is not generated again.
func TestRewrite_CustomMethodInOtherFile(t *testing.T) {
	tmpDir := t.TempDir()
	typeFile := filepath.Join(tmpDir, "status.go")
	require.NoError(t, os.WriteFile(typeFile, []byte(`package example

type Status string //#enum

const StatusActive Status = "active"
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "status_methods.go"), []byte(`package example

// String returns a label.
//
//#custom
func (s Status) String() string {
	return "Status " + string(s)
}
`), 0644))

	require.NoError(t, Rewrite(tmpDir, nil, nil, false))
	written, err := os.ReadFile(typeFile)
	require.NoError(t, err)
	assert.Contains(t, string(written), "func (s Status) Valid() bool")
	assert.NotContains(t, string(written), "String()")
	requireBuilds(t, tmpDir)
	require.NoError(t, ValidateRewrite(tmpDir, nil, false))
}

// writeFiles writes the files of a package to a new temporary directory
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, source := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(source), 0644))
	}
	return dir
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return string(source)
}

// TestRewrite_IotaEnum verifies that all values of an iota enum
// are valid, not only the first one with an explicit type.
func TestRewrite_IotaEnum(t *testing.T) {
	dir := writeFiles(t, map[string]string{"kind.go": `package example

type Kind int //#enum

const (
	KindA Kind = iota + 1
	KindB
	KindC
)
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	result := readFile(t, dir, "kind.go")
	assert.Contains(t, result, "KindA,\n\t\tKindB,\n\t\tKindC:")
	assert.Contains(t, result, "\"1\",\n\t\t\"2\",\n\t\t\"3\",")
	requireBuilds(t, dir)
}

// TestRewrite_RemovesMethodsNoLongerGenerated verifies that removing
// //#null removes the nullable methods generated before together with
// their imports, else they would keep marshalling the old null value
// and the unused imports would break the build.
func TestRewrite_RemovesMethodsNoLongerGenerated(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	nullable := readFile(t, dir, "status.go")
	require.Contains(t, nullable, "func (s Status) IsNull() bool")
	require.Contains(t, nullable, `"database/sql/driver"`)

	notNullable := strings.Replace(nullable, `StatusNone   Status = "" //#null`, `StatusNone   Status = ""`, 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(notNullable), 0644))
	require.NoError(t, Rewrite(dir, nil, nil, false))
	result := readFile(t, dir, "status.go")
	for _, removed := range []string{"IsNull", "IsNotNull", "SetNull", "MarshalJSON", "UnmarshalJSON", "Scan", "Value()", `"bytes"`, `"encoding/json"`, `"database/sql/driver"`} {
		assert.NotContains(t, result, removed)
	}
	assert.Contains(t, result, "StatusNone,\n\t\tStatusActive:")
	requireBuilds(t, dir)
	require.NoError(t, ValidateRewrite(dir, nil, false))
}

// TestRewrite_CustomMethodImports verifies that imports are only added
// for generated methods that use them, so a //#custom UnmarshalJSON
// that doesn't use bytes does not get an unused bytes import.
func TestRewrite_CustomMethodImports(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

import "encoding/json"

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)

// UnmarshalJSON is hand-written.
//
//#custom
func (s *Status) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, (*string)(s))
}
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.NotContains(t, readFile(t, dir, "status.go"), `"bytes"`)
	requireBuilds(t, dir)
}

// TestRewrite_HandWrittenMethodsOfOtherUnderlyingType verifies that
// hand-written Scan and Value methods of a nullable float enum, for
// which no Scan and Value methods are generated, are kept.
func TestRewrite_HandWrittenMethodsOfOtherUnderlyingType(t *testing.T) {
	dir := writeFiles(t, map[string]string{"ratio.go": `package example

import (
	"database/sql/driver"
	"fmt"
)

type Ratio float64 //#enum

const (
	RatioNone Ratio = 0 //#null
	RatioHalf Ratio = 0.5
)

func (r *Ratio) Scan(value any) error {
	f, ok := value.(float64)
	if !ok {
		return fmt.Errorf("can't scan %T", value)
	}
	*r = Ratio(f)
	return nil
}

func (r Ratio) Value() (driver.Value, error) {
	return float64(r), nil
}
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	result := readFile(t, dir, "ratio.go")
	assert.Contains(t, result, "func (r *Ratio) Scan(value any) error {\n\tf, ok := value.(float64)")
	assert.Contains(t, result, "func (r Ratio) Value() (driver.Value, error) {\n\treturn float64(r), nil")
	requireBuilds(t, dir)
}

// TestRewrite_ReceiverNames verifies that generated methods compile
// when the receiver name would collide with a parameter name or
// existing methods use a blank receiver.
func TestRewrite_ReceiverNames(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"job.go": `package example

type JobState string //#enum

const (
	JobStateNone    JobState = "" //#null
	JobStateRunning JobState = "running"
)
`,
		"mode.go": `package example

type Mode int //#enum

const (
	ModeNone Mode = 0 //#null
	ModeFast Mode = 1
)

func (_ Mode) Describe() string { return "mode" }

func (value Mode) Label() string { return "mode" }
`,
	})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "job.go"), "func (j *JobState) UnmarshalJSON(data []byte) error")
	assert.Contains(t, readFile(t, dir, "mode.go"), "func (value *Mode) Scan(v any) error")
	// The error must name the type of the scanned value, not of the receiver
	assert.Contains(t, readFile(t, dir, "mode.go"), `return fmt.Errorf("can't scan SQL value of type %T as example.Mode", v)`)
	requireBuilds(t, dir)
}

// TestRewrite_TrailingCommentOfConstBlock verifies that a line comment
// after the closing parenthesis of the last enum const block stays
// on its line instead of moving behind the inserted methods.
func TestRewrite_TrailingCommentOfConstBlock(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

type Status string //#enum

const (
	StatusActive Status = "active"
) // end of Status values
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "status.go"), "\tStatusActive Status = \"active\"\n) // end of Status values\n\n// Valid indicates")
}

// TestRewrite_NoFileWrittenOnError verifies that an invalid enum in one
// file of a package fails before any file of the package is written,
// instead of leaving the package half generated.
func TestRewrite_NoFileWrittenOnError(t *testing.T) {
	valid := `package example

type Color string //#enum

const ColorRed Color = "red"
`
	dir := writeFiles(t, map[string]string{
		"a_color.go": valid,
		"b_status.go": `package example

type Status string //#enum

const (
	StatusNone Status = ""     //#null
	StatusNull Status = "null" //#null
)
`,
	})
	require.ErrorContains(t, Rewrite(dir, nil, nil, false), "second //#null")
	assert.Equal(t, valid, readFile(t, dir, "a_color.go"))

	// Value: protects=a package file with a syntax error fails the run, naming that file, before any file of the package is written;
	// fails_when=parsePackageDir skips or ignores files with parse errors, so the methods are generated from the remaining files;
	// why_new=the fixture above only has an invalid enum; coverage shows the parse error return of parsePackageDir never runs;
	// seam=none;
	// no_claim=does not check the exit code of the command, only the returned error
	dir = writeFiles(t, map[string]string{
		"a_color.go":  valid,
		"b_broken.go": "package example\n\nfunc broken() {\n",
	})
	require.ErrorContains(t, Rewrite(dir, nil, nil, false), "b_broken.go")
	assert.Equal(t, valid, readFile(t, dir, "a_color.go"), "a file that can't be parsed must not be skipped")
}

// TestRewrite_ImportedValueInOtherFile verifies that a value using an
// import of another file is referenced by name, because the import is
// not available in the file of the enum type.
func TestRewrite_ImportedValueInOtherFile(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"method.go": `package example

type Method string //#enum

const MethodPost Method = "POST"
`,
		"method_values.go": `package example

import "net/http"

const MethodGet Method = http.MethodGet
`,
	})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "method.go"), "\"POST\",\n\t\tstring(MethodGet),")
	requireBuilds(t, dir)
}

// TestRewrite_MainPackage verifies that enums of a main package are generated.
func TestRewrite_MainPackage(t *testing.T) {
	dir := writeFiles(t, map[string]string{"main.go": `package main

type Status string //#enum

const StatusActive Status = "active"

func main() {}
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "main.go"), "func (s Status) Valid() bool")
	requireBuilds(t, dir)
}

// TestRewrite_KeepsHandWrittenMethodWithStandardDoc verifies that a
// hand-written method with the same name and doc comment as a generated
// method, which is not generated for the enum, is not removed as a method
// that is no longer generated. The doc comments of generated methods like
// "UnmarshalJSON implements encoding/json.Unmarshaler" are common phrases.
func TestRewrite_KeepsHandWrittenMethodWithStandardDoc(t *testing.T) {
	dir := writeFiles(t, map[string]string{"samesite.go": `package example

import "fmt"

type SameSite string //#enum

const (
	SameSiteStrict SameSite = "strict"
	SameSiteLax    SameSite = "lax"
)

// UnmarshalJSON implements encoding/json.Unmarshaler
func (ss *SameSite) UnmarshalJSON(j []byte) error {
	switch string(j) {
	case "true", ` + "`" + `"strict"` + "`" + `:
		*ss = SameSiteStrict
	case "false", ` + "`" + `"lax"` + "`" + `:
		*ss = SameSiteLax
	default:
		return fmt.Errorf("invalid SameSite %s", j)
	}
	return nil
}
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	result := readFile(t, dir, "samesite.go")
	assert.Contains(t, result, "func (ss *SameSite) UnmarshalJSON(j []byte) error {\n\tswitch string(j) {")
	assert.Contains(t, result, "func (ss SameSite) Valid() bool")
	requireBuilds(t, dir)
}

// TestRewrite_WindowsLineEndings verifies that a file with CRLF line endings,
// like in a Windows checkout with git's core.autocrlf, is not reported as
// outdated or rewritten when its methods are up to date, and that generated
// methods are written with the line endings of the file.
func TestRewrite_WindowsLineEndings(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	upToDate := strings.ReplaceAll(readFile(t, dir, "status.go"), "\n", "\r\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(upToDate), 0644))

	require.NoError(t, ValidateRewrite(dir, nil, false))
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Equal(t, upToDate, readFile(t, dir, "status.go"), "up to date file must not be rewritten")

	outdated := strings.Replace(upToDate, `StatusActive Status = "active"`+"\r\n", `StatusActive Status = "active"`+"\r\n\tStatusDone   Status = \"done\"\r\n", 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(outdated), 0644))
	require.NoError(t, Rewrite(dir, nil, nil, false))
	result := readFile(t, dir, "status.go")
	assert.Contains(t, result, "StatusActive,\r\n\t\tStatusDone:")
	assert.Equal(t, strings.Count(result, "\n"), strings.Count(result, "\r\n"), "all lines must end with CRLF")
	requireBuilds(t, dir)
}

// TestRewrite_KeepsImportGroups verifies that generating methods without
// adding an import keeps the import groups of the file, because grouping
// imports belongs to gofmt/goimports, not to the enum generator.
// Only when imports are added, they are grouped like goimports does.
func TestRewrite_KeepsImportGroups(t *testing.T) {
	// gofmt sorted, but goimports would separate the standard library
	imports := `import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"strings"
)

var (
	_ = fmt.Sprint
	_ = strings.ToUpper
	_ = assert.Equal
)
`
	t.Run("no import added", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "status.go")
		source := "package example\n\n" + imports + `
type Status int //#enum

const StatusActive Status = 1
`
		require.NoError(t, os.WriteFile(testFile, []byte(source), 0644))

		require.NoError(t, Rewrite(tmpDir, nil, nil, false))
		written, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Contains(t, string(written), "func (s Status) Valid() bool")
		assert.Contains(t, string(written), imports, "import groups must not change")
	})

	t.Run("import added", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "status.go")
		source := "package example\n\n" + imports + `
type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`
		require.NoError(t, os.WriteFile(testFile, []byte(source), 0644))

		require.NoError(t, Rewrite(tmpDir, nil, nil, false))
		written, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Contains(t, string(written), `import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/stretchr/testify/assert"
)`, "added imports must be grouped like goimports does")
	})
}

// TestRewrite_NullablePackageStringTypeValue verifies that Value returns
// the basic string type, because database/sql rejects a driver.Value
// of another named string type of the package.
func TestRewrite_NullablePackageStringTypeValue(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

type MyString string

type Status MyString //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "status.go"), "return string(s), nil")
	requireBuilds(t, dir)
}

// TestRewrite_ReceiverCollisions verifies that the receiver name of
// existing methods is not used if it is also an enum const, the name
// of a package used by the generated methods or not a valid name,
// because the generated methods would be wrong or not compile.
func TestRewrite_ReceiverCollisions(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"axis.go": `package example

type axis byte //#enum

const (
	x axis = 'x'
	y axis = 'y'
)

func (x axis) Other() axis { return x }
`,
		"format.go": `package example

type Format string //#enum

const FormatJSON Format = "json"

func (fmt Format) Extension() string { return "." + string(fmt) }
`,
		"underscore.go": `package example

type _kind int //#enum

const _kindA _kind = 1
`,
	})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	// switch x { case x, y: } would be true for every value
	assert.NotContains(t, readFile(t, dir, "axis.go"), "func (x axis) Valid() bool")
	assert.NotContains(t, readFile(t, dir, "format.go"), "func (fmt Format) Validate() error")
	requireBuilds(t, dir)
}

// TestRewrite_RecursivePathSkipsDirsLikeGoTool verifies that recursive
// mode only rewrites the packages of the module like ./... of the go tool,
// never vendored or nested modules or ignored directories.
func TestRewrite_RecursivePathSkipsDirsLikeGoTool(t *testing.T) {
	dir := t.TempDir()
	source := `package p

type Status string //#enum

const StatusActive Status = "active"
`
	want := map[string]bool{
		"a/a.go":              true,
		"a/b/b.go":            true,
		"testdata/t.go":       false,
		"_x/x.go":             false,
		".x/x.go":             false,
		"node_modules/n/n.go": false,
		"vendor/v/v.go":       false,
		"nested/n.go":         false,
	}
	for name := range want {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(source), 0644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nested", "go.mod"), []byte("module nested\n"), 0644))

	require.NoError(t, Rewrite(dir+"/...", nil, nil, false))
	for name, generated := range want {
		assert.Equal(t, generated, strings.Contains(readFile(t, dir, name), "Valid() bool"), name)
	}

	// Value: protects=a directory path without Go files fails, so -validate with a mistyped go:generate or CI path can't pass silently;
	// fails_when=a package dir without Go files is skipped also without "...", like the dirs below a recursive path;
	// why_new=only the recursive skip of dirs without Go files is tested, the "no Go files" error is never reached;
	// seam=none;
	// no_claim=does not check the exit code of the command, only the returned error
	require.ErrorContains(t, ValidateRewrite(dir, nil, false), "no Go files")
}

// TestRewrite_SingleFile verifies that a file path only rewrites that file
// and that a path that is no Go file of the package fails instead of
// silently doing nothing.
func TestRewrite_SingleFile(t *testing.T) {
	source := `package example

type %s string //#enum

const %sA %s = "a"
`
	dir := writeFiles(t, map[string]string{
		"one.go":      fmt.Sprintf(source, "One", "One", "One"),
		"two.go":      fmt.Sprintf(source, "Two", "Two", "Two"),
		"two_test.go": "package example\n",
	})
	require.NoError(t, Rewrite(filepath.Join(dir, "one.go"), nil, nil, false))
	assert.Contains(t, readFile(t, dir, "one.go"), "Valid() bool")
	assert.NotContains(t, readFile(t, dir, "two.go"), "Valid() bool")

	// Files the go tool doesn't compile with the package are no package
	// files, instead of silently generating nothing
	require.ErrorContains(t, ValidateRewrite(filepath.Join(dir, "two_test.go"), nil, false), "two_test.go is not a Go file of the package")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), nil, 0644))
	require.ErrorContains(t, ValidateRewrite(filepath.Join(dir, "README.md"), nil, false), "README.md is not a Go file of the package")

	// On case-insensitive file systems a differently cased path is the same
	// file, which must get the methods instead of creating another file.
	// Only runs on case-insensitive file systems like the default of macOS,
	// not on Linux CI.
	t.Run("case-insensitive", func(t *testing.T) {
		upper := filepath.Join(dir, "TWO.go")
		if _, err := os.Stat(upper); err != nil {
			t.Skip("case-sensitive file system")
		}
		require.Error(t, ValidateRewrite(upper, nil, false), "two.go is outdated")
		require.NoError(t, Rewrite(upper, nil, nil, false))
		assert.Contains(t, readFile(t, dir, "two.go"), "Valid() bool")
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		assert.Equal(t, []string{"README.md", "one.go", "two.go", "two_test.go"}, names)
	})
}

// TestRewrite_LineDirective verifies that //line directives, which only
// change reported positions, don't move the methods to another file.
func TestRewrite_LineDirective(t *testing.T) {
	dir := writeFiles(t, map[string]string{"parser.go": "package example\n\n//line parser.y:4\ntype Kind int //#enum\n\nconst KindA Kind = 1\n"})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "parser.go"), "func (k Kind) Valid() bool")
}

// TestRewrite_BlockCommentAfterDeclaration verifies that methods are
// inserted after a block comment that starts on the last line of the
// declaration, not inside of it.
func TestRewrite_BlockCommentAfterDeclaration(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

type Status string //#enum

const (
	StatusA Status = "a"
) /* values end here,
add new values above */
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	result := readFile(t, dir, "status.go")
	assert.Contains(t, result, "add new values above */\n\n// Valid indicates")
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Equal(t, result, readFile(t, dir, "status.go"), "second Rewrite must be byte-identical")
	requireBuilds(t, dir)
}

// TestRewrite_MixedLineEndings verifies that a file with LF line endings
// and a single CRLF line is not converted to CRLF line endings.
func TestRewrite_MixedLineEndings(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": "package example\n\n// Status is pasted\r\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n"})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	result := readFile(t, dir, "status.go")
	assert.Contains(t, result, "Valid() bool")
	assert.LessOrEqual(t, strings.Count(result, "\r\n"), 1, "LF lines must not be converted to CRLF")
}

// TestRewrite_HandWrittenMethodWithSimilarDoc verifies that a hand-written
// method is not replaced because its doc comment starts like the doc
// comment of the generated method, which would silently drop its code.
func TestRewrite_HandWrittenMethodWithSimilarDoc(t *testing.T) {
	for name, method := range map[string]string{
		"similar doc": `// Validate returns an error if s is not active
func (s Status) Validate() error {
	if s != StatusActive {
		return fmt.Errorf("read-only")
	}
	return nil
}`,
		"longer doc": `// Validate returns an error if s is none of the valid values for Status
//
// Archived statuses are read-only.
func (s Status) Validate() error {
	if s != StatusActive {
		return fmt.Errorf("read-only")
	}
	return nil
}`,
		"same doc without placeholder": `// UnmarshalJSON implements encoding/json.Unmarshaler
func (s *Status) UnmarshalJSON(j []byte) error {
	*s = Status(strings.ToLower(string(j)))
	return s.Validate()
}`,
	} {
		t.Run(name, func(t *testing.T) {
			source := `package example

import (
	"fmt"
	"strings"
)

var _ = strings.ToLower

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)

var _ = fmt.Sprint

` + method + "\n"
			dir := writeFiles(t, map[string]string{"status.go": source})
			err := Rewrite(dir, nil, nil, false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "//#custom")
			assert.Equal(t, source, readFile(t, dir, "status.go"), "file must not be changed")
		})
	}
}

// TestRewrite_NoLongerGeneratedMethods verifies that methods that are
// no longer generated are removed if they are unchanged, also when the
// null value is excluded or deleted at the same time, and that a changed
// method that is no longer generated is an error instead of being kept
// silently with references to values that may no longer exist.
func TestRewrite_NoLongerGeneratedMethods(t *testing.T) {
	source := `package example

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`
	nullableMethods := []string{"IsNull", "IsNotNull", "SetNull", "MarshalJSON", "UnmarshalJSON", "Scan", "Value()", `"database/sql/driver"`}

	t.Run("null value excluded", func(t *testing.T) {
		dir := writeFiles(t, map[string]string{"status.go": source})
		require.NoError(t, Rewrite(dir, nil, nil, false))
		excluded := strings.Replace(readFile(t, dir, "status.go"), `"" //#null`, `"" //#exclude`, 1)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(excluded), 0644))
		require.NoError(t, Rewrite(dir, nil, nil, false))
		result := readFile(t, dir, "status.go")
		for _, removed := range nullableMethods {
			assert.NotContains(t, result, removed)
		}
		requireBuilds(t, dir)
		require.NoError(t, ValidateRewrite(dir, nil, false))
	})

	t.Run("null value deleted", func(t *testing.T) {
		dir := writeFiles(t, map[string]string{"status.go": source})
		require.NoError(t, Rewrite(dir, nil, nil, false))
		deleted := strings.Replace(readFile(t, dir, "status.go"), "\tStatusNone   Status = \"\" //#null\n", "", 1)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(deleted), 0644))
		require.NoError(t, Rewrite(dir, nil, nil, false))
		result := readFile(t, dir, "status.go")
		for _, removed := range nullableMethods {
			assert.NotContains(t, result, removed)
		}
		requireBuilds(t, dir)
	})

	t.Run("changed method", func(t *testing.T) {
		dir := writeFiles(t, map[string]string{"status.go": strings.Replace(source, "//#enum", "//#enum,jsonschema", 1)})
		require.NoError(t, Rewrite(dir, nil, nil, false))
		// Removing ,jsonschema while adding a value: the JSONSchema method
		// can't be recognized as unchanged anymore
		changed := strings.Replace(readFile(t, dir, "status.go"), "//#enum,jsonschema", "//#enum", 1)
		changed = strings.Replace(changed, "\tStatusActive Status = \"active\"\n", "\tStatusActive Status = \"active\"\n\tStatusDone   Status = \"done\"\n", 1)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(changed), 0644))
		err := Rewrite(dir, nil, nil, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Status.JSONSchema")
		assert.Contains(t, err.Error(), "//#custom")
	})

	// Value: protects=removing ,jsonschema, also together with //#null, removes the unchanged JSONSchema method and its import so the package builds;
	// fails_when=wasGenerated misses the JSONSchema variant (with the former null value), so removing the flag fails as a changed method;
	// why_new="changed method" only covers the error; JSONSchema has template conditions, so only wasGenerated can recognize it;
	// seam=none;
	// no_claim=does not cover a hand-edited JSONSchema method, which must stay an error
	t.Run("jsonschema flag removed", func(t *testing.T) {
		for name, remove := range map[string][]string{
			"flag":          {"//#enum,jsonschema", "//#enum"},
			"flag and null": {"//#enum,jsonschema", "//#enum", `"" //#null`, `""`},
		} {
			t.Run(name, func(t *testing.T) {
				dir := writeFiles(t, map[string]string{"status.go": strings.Replace(source, "//#enum", "//#enum,jsonschema", 1)})
				require.NoError(t, Rewrite(dir, nil, nil, false))
				require.Contains(t, readFile(t, dir, "status.go"), "JSONSchema()")
				removed := strings.NewReplacer(remove...).Replace(readFile(t, dir, "status.go"))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(removed), 0644))
				require.NoError(t, Rewrite(dir, nil, nil, false))
				result := readFile(t, dir, "status.go")
				assert.NotContains(t, result, "JSONSchema")
				assert.NotContains(t, result, "jsonschema")
				requireBuilds(t, dir)
				require.NoError(t, ValidateRewrite(dir, nil, false))
			})
		}
	})
}

// TestRewrite_UntypedConversionOfImportedValue verifies that untyped consts
// converted to the enum type from an imported value, which go/types can't
// evaluate without the imported package, are enum values.
func TestRewrite_UntypedConversionOfImportedValue(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"method.go": `package example

import "net/http"

type Method string //#enum

const (
	MethodGet    = Method(http.MethodGet)
	MethodCustom = Method("CUSTOM")
)
`,
		"method_post.go": `package example

import "net/http"

const MethodPost = Method(http.MethodPost)
`,
	})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	result := readFile(t, dir, "method.go")
	assert.Contains(t, result, "MethodGet,\n\t\tMethodCustom,\n\t\tMethodPost:")
	assert.Contains(t, result, "string(MethodGet),")
	requireBuilds(t, dir)
}

// TestRewrite_AliasedImport verifies that an aliased import of a package
// used by the generated methods is not taken as import of the package name
// used by them, because the generated methods would not compile.
func TestRewrite_AliasedImport(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

import f "fmt"

var _ = f.Sprint

type Status string //#enum

const StatusActive Status = "active"
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	generated := readFile(t, dir, "status.go")
	require.Contains(t, generated, "\t\"fmt\"\n")
	requireBuilds(t, dir)

	// Up to date methods with only the aliased import are outdated
	aliasedOnly := strings.Replace(generated, "\t\"fmt\"\n", "", 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(aliasedOnly), 0644))
	require.Error(t, ValidateRewrite(dir, nil, false))
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Equal(t, generated, readFile(t, dir, "status.go"))
}

// TestRewrite_SameBuildConstraints verifies that files with the same build
// constraints as the file of the enum type are compiled together with it,
// so their values and //#custom methods are used.
func TestRewrite_SameBuildConstraints(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"status.go": `//go:build !plan9

package example

type Status string //#enum

const StatusA Status = "a"
`,
		"more.go": `//go:build !plan9

package example

const StatusB Status = "b"
`,
	})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "status.go"), "StatusB,\n\t\tStatusA:", "values ordered by file name")
	requireBuilds(t, dir)
}

// TestRewrite_CustomMethodInConstrainedFiles verifies that a method marked
// with //#custom per platform is not generated in the file of the enum type,
// else the package would not compile on any platform.
func TestRewrite_CustomMethodInConstrainedFiles(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"status.go": `package example

type Status string //#enum

const StatusA Status = "a"
`,
		"status_plan9.go": `package example

//#custom
func (s Status) String() string { return "plan9 " + string(s) }
`,
		"status_other.go": `//go:build !plan9

package example

//#custom
func (s Status) String() string { return string(s) }
`,
	})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.NotContains(t, readFile(t, dir, "status.go"), "String() string")
	requireBuilds(t, dir)
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

// TestRewrite_PrintErrorKeepsOtherErrors verifies that an error writing
// the result with -print doesn't hide the errors of the packages before.
func TestRewrite_PrintErrorKeepsOtherErrors(t *testing.T) {
	dir := t.TempDir()
	for name, source := range map[string]string{
		"p0": "package p0\n\ntype Status string //#enum\n\nconst (\n\tStatusA Status = \"a\"\n\tStatusB Status = \"a\"\n)\n",
		"p1": "package p1\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
	} {
		require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name, "status.go"), []byte(source), 0644))
	}
	err := Rewrite(dir+"/...", nil, errWriter{}, false)
	require.ErrorContains(t, err, "broken pipe")
	assert.Contains(t, err.Error(), "duplicate enum value")
}

// TestRewrite_ReplacesFileThroughRename verifies that a rewritten file is
// replaced by the temporary file, so that an interrupted write can't leave
// a truncated source file, which a hard link to the old file shows.
func TestRewrite_ReplacesFileThroughRename(t *testing.T) {
	source := "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n"
	dir := writeFiles(t, map[string]string{"status.go": source})
	link := filepath.Join(t.TempDir(), "status.go")
	if err := os.Link(filepath.Join(dir, "status.go"), link); err != nil {
		t.Skip("can't create a hard link:", err)
	}
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "status.go"), "func (s Status) Valid() bool")
	linked, err := os.ReadFile(link)
	require.NoError(t, err)
	assert.Equal(t, source, string(linked), "the file must be replaced, not written in place")
}

// TestRewrite_KeepsFileMode verifies that a rewritten file keeps its file
// mode and that no temporary files are left in the package directory.
func TestRewrite_KeepsFileMode(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

type Status string //#enum

const StatusA Status = "a"
`})
	// 0644 is no default mode of os.CreateTemp, so the mode must be set
	require.NoError(t, os.Chmod(filepath.Join(dir, "status.go"), 0644))
	require.NoError(t, Rewrite(dir, nil, nil, false))
	info, err := os.Stat(filepath.Join(dir, "status.go"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary files must be left")

	// Value: protects=a package file that is a symlink stays a symlink and its target gets the methods, like with os.WriteFile of go-astvisit before;
	// fails_when=writeFile renames the temporary file over the link instead of over the resolved target file;
	// why_new=the test above only rewrites a regular file;
	// seam=none;
	// no_claim=skipped where symlinks can't be created; a hard link is still replaced by a new file
	t.Run("symlink", func(t *testing.T) {
		target := writeFiles(t, map[string]string{"status.go": "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n"})
		dir := t.TempDir()
		link := filepath.Join(dir, "status.go")
		if err := os.Symlink(filepath.Join(target, "status.go"), link); err != nil {
			t.Skip("can't create symlink:", err)
		}
		require.NoError(t, Rewrite(dir, nil, nil, false))
		info, err := os.Lstat(link)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink, "symlink must not be replaced by a regular file")
		assert.Contains(t, readFile(t, target, "status.go"), "func (s Status) Valid() bool")
		entries, err := os.ReadDir(target)
		require.NoError(t, err)
		assert.Len(t, entries, 1, "no temporary files must be left")
	})
}

// TestRewrite_MethodsGeneratedByPreviousVersion verifies that the methods
// of a file generated by go-enum v1.1.0 are recognized as generated and
// replaced, so that upgrading go-enum doesn't fail with hand-written methods.
//
// Value: protects=files generated by go-enum v1.1.0 are regenerated like a file without methods, without hand-written method errors;
// fails_when=a template's doc comment or the UnmarshalJSON code changes, so v1.1.0 methods fail as hand-written on upgrade;
// why_new=other tests recognize methods generated with the same templates; only some v1.1.0 docs are literal fixtures;
// seam=none;
// no_claim=does not freeze the generated output, intentional output changes still pass
func TestRewrite_MethodsGeneratedByPreviousVersion(t *testing.T) {
	source := `package example

type Status string //#enum,jsonschema

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`
	// Generated by go-enum v1.1.0 from source
	v110 := `package example

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
)

type Status string //#enum,jsonschema

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)

// Valid indicates if s is any of the valid values for Status
func (s Status) Valid() bool {
	switch s {
	case
		StatusNone,
		StatusActive:
		return true
	}
	return false
}

// Validate returns an error if s is none of the valid values for Status
func (s Status) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("invalid value %#v for type example.Status", s)
	}
	return nil
}

// Enums returns all valid values for Status
func (Status) Enums() []Status {
	return []Status{
		StatusNone,
		StatusActive,
	}
}

// EnumStrings returns all valid values for Status as strings
func (Status) EnumStrings() []string {
	return []string{
		"",
		"active",
	}
}

// String implements the fmt.Stringer interface for Status
func (s Status) String() string {
	return string(s)
}

// IsNull returns true if s is the null value StatusNone
func (s Status) IsNull() bool {
	return s == StatusNone
}

// IsNotNull returns true if s is not the null value StatusNone
func (s Status) IsNotNull() bool {
	return s != StatusNone
}

// SetNull sets the null value StatusNone at s
func (s *Status) SetNull() {
	*s = StatusNone
}

// MarshalJSON implements encoding/json.Marshaler for Status
// by returning the JSON null value for StatusNone.
func (s Status) MarshalJSON() ([]byte, error) {
	if s == StatusNone {
		return []byte("null"), nil
	}
	return json.Marshal(string(s))
}

// UnmarshalJSON implements encoding/json.Unmarshaler
func (s *Status) UnmarshalJSON(j []byte) error {
	if bytes.Equal(j, []byte("null")) {
		*s = StatusNone
		return nil
	}
	return json.Unmarshal(j, (*string)(s))
}

// Scan implements the database/sql.Scanner interface for Status
func (s *Status) Scan(value any) error {
	switch value := value.(type) {
	case string:
		*s = Status(value)
	case []byte:
		*s = Status(value)
	case nil:
		*s = StatusNone
	default:
		return fmt.Errorf("can't scan SQL value of type %T as example.Status", value)
	}
	return nil
}

// Value implements the driver database/sql/driver.Valuer interface for Status
func (s Status) Value() (driver.Value, error) {
	if s == StatusNone {
		return nil, nil
	}
	return string(s), nil
}

// JSONSchema returns a github.com/invopop/jsonschema.Schema for Status
func (Status) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{
				Type: "string",
				Enum: []any{
					"active",
				},
			},
			{Type: "null"},
		},
		Default: StatusNone,
	}
}
`
	dir := writeFiles(t, map[string]string{"status.go": v110})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	fresh := writeFiles(t, map[string]string{"status.go": source})
	require.NoError(t, Rewrite(fresh, nil, nil, false))
	assert.Equal(t, readFile(t, fresh, "status.go"), readFile(t, dir, "status.go"))
}

// TestRewrite_GenerationErrorNamesFile verifies that an error while generating
// the methods of a file names that file, so that the user can locate it.
func TestRewrite_GenerationErrorNamesFile(t *testing.T) {
	sources := map[string]string{
		"a_kind.go": "package example\n\ntype Kind int //#enum\n\nconst KindA Kind = 1\n",
		// Not gofmt formatted, the methods are inserted into the struct
		"b_status.go": "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"; type X struct {\n}\n",
	}
	dir := writeFiles(t, sources)
	err := Rewrite(dir, nil, nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "b_status.go")

	// Value: protects=no file of a package is written if generating the methods of one of its enums fails;
	// fails_when=Rewrite writes the files of the package one by one while generating, so a_kind.go gets methods before b_status.go fails;
	// why_new=the assertions above only check the error message;
	// seam=none;
	// no_claim=does not check other packages of a recursive run
	for name, source := range sources {
		assert.Equal(t, source, readFile(t, dir, name), name)
	}
}

// TestRewrite_LocalVariableNamedLikeImport verifies that a local variable
// named like a package used by generated methods doesn't keep the import
// of that package after the methods are removed.
func TestRewrite_LocalVariableNamedLikeImport(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	notNullable := strings.Replace(readFile(t, dir, "status.go"), `"" //#null`, `""`, 1) + `
func local() {
	json := struct{ Marshal func() }{}
	_ = json.Marshal
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(notNullable), 0644))
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.NotContains(t, readFile(t, dir, "status.go"), `"encoding/json"`)
	requireBuilds(t, dir)
}

// TestRewrite_NullValueNamedLikeParam verifies that a null value with the
// name of a parameter of a generated method doesn't get shadowed by it.
func TestRewrite_NullValueNamedLikeParam(t *testing.T) {
	dir := writeFiles(t, map[string]string{"kind.go": `package example

type kind string //#enum

const (
	value kind = "" //#null
	j     kind = "j"
	other kind = "other"
)
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	requireBuilds(t, dir)
}

// TestRewrite_ParamNamesTaken verifies that the parameters of generated
// methods don't shadow enum values or the enum type, also when the
// alternative parameter name is taken too, else the methods don't compile.
func TestRewrite_ParamNamesTaken(t *testing.T) {
	for name, source := range map[string]string{
		"values": `package example

type kind string //#enum

const (
	v     kind = "" //#null
	value kind = "value"
	j     kind = "j"
	data  kind = "data"
)
`,
		"type": `package example

type value string //#enum

const (
	valueNone value = "" //#null
	valueA    value = "a"
)
`,
		"underlying type": `package example

type j string

type Status j //#enum

const (
	StatusNone Status = "" //#null
	StatusA    Status = "a"
)
`,
		"generic underlying type": `package example

type j[T any] string

type Status j[int] //#enum

const (
	StatusNone Status = "" //#null
	StatusA    Status = "a"
)
`,
		// Value: protects=a generic underlying type with several type arguments, like j[int, int], is not shadowed by a parameter j of UnmarshalJSON;
		// fails_when=baseTypeName only handles one type argument (ast.IndexExpr), so the parameter j shadows the type and the package doesn't build;
		// why_new=the row above has one type argument; coverage shows the ast.IndexListExpr case of baseTypeName never runs;
		// seam=none;
		// no_claim=does not cover a parenthesized underlying type like (j)
		"generic underlying type with type arguments": `package example

type j[K comparable, V any] string

type Status j[int, int] //#enum

const (
	StatusNone Status = "" //#null
	StatusA    Status = "a"
)
`,
		// The receiver and the parameters of the generated methods must not
		// shadow a type argument of the underlying type, which they print
		"type argument named like a parameter": `package example

type box[T any] string

type j string

type Status box[j] //#enum

const (
	StatusNone Status = "" //#null
	StatusA    Status = "a"
)
`,
		"type argument named like the receiver": `package example

type box[T any] string

type k string

type Key box[k] //#enum

const (
	KeyNone Key = "" //#null
	KeyA    Key = "a"
)
`,
		"alternative with _": `package example

type kind string //#enum

const (
	v_    kind = "" //#null
	value kind = "value"
	v     kind = "v"
)
`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{"kind.go": source})
			require.NoError(t, Rewrite(dir, nil, nil, false))
			requireBuilds(t, dir)
		})
	}
}

// TestRewrite_LocalVariableNamedLikeAssumedImport verifies that a local
// variable named like the assumed package name of an import without import
// name, like json of github.com/evanphx/json-patch, whose package name is
// jsonpatch, is not taken as a use of that import, which would be a false
// conflict with the encoding/json package of the generated methods.
func TestRewrite_LocalVariableNamedLikeAssumedImport(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

import "github.com/evanphx/json-patch"

var _ = jsonpatch.MergePatch

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)

func local() {
	json := struct{ Valid func([]byte) bool }{}
	_ = json.Valid
}
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "status.go"), `"encoding/json"`)
}

// TestRewrite_RecursivePathAtModuleRoot verifies that ./... at a module
// root, the documented CI check, is not skipped like a nested module,
// else -validate would pass without checking anything.
func TestRewrite_RecursivePathAtModuleRoot(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"go.mod":    "module example\n\ngo 1.24\n",
		"status.go": "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
	})
	require.ErrorContains(t, ValidateRewrite(dir+"/...", nil, false), "with missing or outdated methods")
	require.NoError(t, Rewrite(dir+"/...", nil, nil, false))
	assert.Contains(t, readFile(t, dir, "status.go"), "func (s Status) Valid() bool")

	// Value: protects=./... through a symlinked checkout checks the packages of the symlink target;
	// fails_when=filepath.WalkDir gets the symlinked root, which it doesn't follow, so no package is checked and -validate passes;
	// why_new=the checks above use the real path;
	// seam=none;
	// no_claim=skipped where symlinks can't be created
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skip("can't create symlink:", err)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kind.go"), []byte("package example\n\ntype Kind int //#enum\n\nconst KindA Kind = 1\n"), 0644))
	require.ErrorContains(t, ValidateRewrite(link+"/...", nil, false), "found 1 enum type(s)")
}

// TestRewrite_FileInReadOnlyDirectory verifies that a writable file in a
// directory without write permission is still rewritten, in place like
// before v1.2.0, because no temporary file can be created next to it.
func TestRewrite_FileInReadOnlyDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write in read-only directories")
	}
	dir := writeFiles(t, map[string]string{"status.go": "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n"})
	require.NoError(t, os.Chmod(dir, 0555))
	t.Cleanup(func() { os.Chmod(dir, 0755) })
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "status.go"), "func (s Status) Valid() bool")
}

// TestRewrite_InternalTestFileMethod verifies that the internal test files
// of the package are read, so a method there that would be generated fails
// instead of breaking go test after the methods were written.
func TestRewrite_InternalTestFileMethod(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"status.go":      "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
		"status_test.go": "package example\n\nfunc (s Status) String() string { return \"test\" }\n",
	})
	require.ErrorContains(t, Rewrite(dir, nil, nil, false), "status_test.go:3 of a test file would be declared twice in test builds")
	assert.NotContains(t, readFile(t, dir, "status.go"), "func (s Status) Valid() bool")

	// The go tool ignores files starting with _ or . also as test files
	dir = writeFiles(t, map[string]string{
		"status.go":       "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
		"_status_test.go": "package example\n\nfunc (s Status) String() string { return \"test\" }\n",
		".status_test.go": "package example\n\nfunc (s Status) String() string { return \"test\" }\n",
	})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "status.go"), "func (s Status) Valid() bool")
}

// TestRewrite_InternalTestFileWithSyntaxError verifies that an internal test
// file that doesn't parse, like one being edited, doesn't stop generating the
// methods, because test files are only read to check for collisions.
func TestRewrite_InternalTestFileWithSyntaxError(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"status.go":      "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
		"status_test.go": "package example\n\nfunc TestX(t *testing.T) {\n",
	})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "status.go"), "func (s Status) Valid() bool")

	// Also with a //go:build line being edited
	dir = writeFiles(t, map[string]string{
		"status.go":      "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
		"status_test.go": "//go:build linux &&\n\npackage example\n",
	})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "status.go"), "func (s Status) Valid() bool")

	// A test-only directory still has test files, which aren't parsed
	// beyond their package clause because there are no enums to check
	dir = writeFiles(t, map[string]string{"status_test.go": "package example\n\nfunc TestX(t *testing.T) {\n"})
	require.ErrorContains(t, ValidateRewrite(dir, nil, false), "no non-test Go files in")
	require.NoError(t, ValidateRewrite(dir+"/...", nil, false))
}

// TestRewrite_NeverBuiltFileWithSyntaxError verifies that, like with go
// build, a file that is compiled in no build, like a template for go run
// with //go:build ignore, is not parsed and doesn't stop generating the
// methods, while a file of some builds that doesn't parse still fails.
func TestRewrite_NeverBuiltFileWithSyntaxError(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"status.go": "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
		"gen.go":    "//go:build ignore\n\npackage main\n\nfunc main() {\n\t{{.Template}}\n}\n",
	})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.Contains(t, readFile(t, dir, "status.go"), "func (s Status) Valid() bool")

	dir = writeFiles(t, map[string]string{
		"status.go":    "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n",
		"gen_linux.go": "package example\n\nfunc main() {\n\t{{.Template}}\n}\n",
	})
	require.ErrorContains(t, Rewrite(dir, nil, nil, false), "gen_linux.go")

	// An enum type in a file with //go:build ignore gets its methods,
	// so such a file that doesn't parse must fail
	dir = writeFiles(t, map[string]string{
		"status.go": "//go:build ignore\n\npackage main\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n\nfunc main() {\n",
	})
	require.ErrorContains(t, Rewrite(dir, nil, nil, false), "status.go")
}

// TestRewrite_RecursiveReportsAllPackages verifies that an error in one
// package of ./... doesn't hide the other packages: -validate reports all
// errors and outdated enums, and the other packages are rewritten.
func TestRewrite_RecursiveReportsAllPackages(t *testing.T) {
	enum := "package %s\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n"
	dir := t.TempDir()
	for name, source := range map[string]string{
		"p1": fmt.Sprintf(enum, "p1"),
		"p2": "package p2\n\ntype Status string //#enum\n\nconst (\n\tStatusA Status = \"a\"\n\tStatusB Status = \"a\"\n)\n",
		"p3": fmt.Sprintf(enum, "p3"),
	} {
		require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name, "status.go"), []byte(source), 0644))
	}
	err := ValidateRewrite(dir+"/...", nil, false)
	require.ErrorContains(t, err, "duplicate enum value")
	require.ErrorContains(t, err, "found 2 enum type(s) with missing or outdated methods")

	require.ErrorContains(t, Rewrite(dir+"/...", nil, nil, false), "duplicate enum value")
	assert.Contains(t, readFile(t, dir, "p1/status.go"), "func (s Status) Valid() bool")
	assert.NotContains(t, readFile(t, dir, "p2/status.go"), "func (s Status) Valid() bool")
	assert.Contains(t, readFile(t, dir, "p3/status.go"), "func (s Status) Valid() bool")

	// Value: protects=a file of one package of ./... that can't be written fails the run naming it, and the other packages are still rewritten;
	// fails_when=rewrite returns the write error of a package at once, so the packages after it are left without methods;
	// why_new=the fixture above only fails before writing; coverage shows the write error of rewrite never runs;
	// seam=none;
	// no_claim=skipped as root, which can write read-only files; does not check other write errors than permission
	t.Run("read-only file", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can write read-only files")
		}
		dir := t.TempDir()
		for _, name := range []string{"p0", "p1"} {
			require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, name, "status.go"), fmt.Appendf(nil, enum, name), 0644))
		}
		// p0 is rewritten before p1
		require.NoError(t, os.Chmod(filepath.Join(dir, "p0", "status.go"), 0444))
		require.ErrorContains(t, Rewrite(dir+"/...", nil, nil, false), filepath.Join("p0", "status.go"))
		assert.NotContains(t, readFile(t, dir, "p0/status.go"), "func (s Status) Valid() bool")
		assert.Contains(t, readFile(t, dir, "p1/status.go"), "func (s Status) Valid() bool")
	})

	// A package that doesn't parse, like one being edited, also fails
	// the run without leaving the other packages without methods
	t.Run("syntax error", func(t *testing.T) {
		dir := t.TempDir()
		for name, source := range map[string]string{
			"p0": "package p0\n\nfunc broken(\n",
			"p1": fmt.Sprintf(enum, "p1"),
		} {
			require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, name, "status.go"), []byte(source), 0644))
		}
		require.ErrorContains(t, Rewrite(dir+"/...", nil, nil, false), filepath.Join("p0", "status.go"))
		assert.Contains(t, readFile(t, dir, "p1/status.go"), "func (s Status) Valid() bool")
	})
}

// TestValidateRewrite_RecursivePathWithoutPackages verifies that ./... over
// a tree without Go files fails like a single directory without Go files,
// so a mistyped CI path can't pass -validate silently.
func TestValidateRewrite_RecursivePathWithoutPackages(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "docs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "README.md"), []byte("# docs\n"), 0644))
	require.ErrorContains(t, ValidateRewrite(dir+"/...", nil, false), "no Go files in")

	// Value: protects=a directory with only _test.go files has no package files to generate into, so -validate of it fails instead of passing, while ./... of a test-only module passes like go vet;
	// fails_when=rewrite counts any parsed file as a package file, so a dir of test files passes -validate with nothing checked, or ./... fails for a test-only module;
	// why_new=the "no Go files" checks above and in TestRewrite_RecursivePathSkipsDirsLikeGoTool use dirs without any .go file;
	// seam=none;
	// no_claim=does not check the exit code of the command, only the returned error
	dir = writeFiles(t, map[string]string{"status_test.go": "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n"})
	require.ErrorContains(t, ValidateRewrite(dir, nil, false), "no non-test Go files in")
	require.NoError(t, ValidateRewrite(dir+"/...", nil, false))

	// A package that doesn't parse has Go files, so only its error is reported
	dir = writeFiles(t, map[string]string{"broken.go": "package example\n\nfunc {\n"})
	err := ValidateRewrite(dir+"/...", nil, false)
	require.ErrorContains(t, err, "broken.go")
	assert.NotContains(t, err.Error(), "or below")
}

// TestRewrite_TypeExpressionAsUnderlyingType verifies that methods generated
// for an underlying type that is no identifier, like a generic type with
// type arguments or a parenthesized type, are recognized on the next run,
// else go-enum would reject its own output.
func TestRewrite_TypeExpressionAsUnderlyingType(t *testing.T) {
	for name, underlying := range map[string]string{
		"type arguments":        "type str[T any] string\n\ntype Status str[int] //#enum\n",
		"nested type arguments": "type str[T any] string\n\ntype Status str[map[string][]byte] //#enum\n",
		"deeply nested":         "type str[T any] string\n\ntype list[T any] []T\n\ntype Status str[list[list[map[[2]int][]byte]]] //#enum\n",
		"slice type argument":   "type str[T any] string\n\ntype Status str[[]byte] //#enum\n",
		"parentheses":           "type Status (string) //#enum\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{"status.go": "package example\n\n" + underlying + "\nconst (\n\tStatusNone   Status = \"\" //#null\n\tStatusActive Status = \"active\"\n)\n"})
			require.NoError(t, Rewrite(dir, nil, nil, false))
			generated := readFile(t, dir, "status.go")
			require.NoError(t, ValidateRewrite(dir, nil, false))
			require.NoError(t, Rewrite(dir, nil, nil, false))
			assert.Equal(t, generated, readFile(t, dir, "status.go"))
			requireBuilds(t, dir)
		})
	}
}

// TestRewrite_ReceiverNamedLikeUnderlyingType verifies that the receiver
// name of an existing method is not used when it equals the name of the
// underlying type, which it would shadow in MarshalJSON.
func TestRewrite_ReceiverNamedLikeUnderlyingType(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": "package example\n\ntype status string\n\ntype Status status //#enum\n\nconst (\n\tStatusNone   Status = \"\" //#null\n\tStatusActive Status = \"active\"\n)\n\nfunc (status Status) Label() string { return string(status) }\n"})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	requireBuilds(t, dir)
}

// TestRewrite_ImportsRemovedAndAddedInOneRun verifies that one run can remove
// the import of methods that are no longer generated and add the import of
// new methods, else the file would not compile.
func TestRewrite_ImportsRemovedAndAddedInOneRun(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

type Kind string //#enum,jsonschema

const KindA Kind = "a"

type Status string //#enum

const (
	StatusNone   Status = ""
	StatusActive Status = "active"
)
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	require.Contains(t, readFile(t, dir, "status.go"), `"github.com/invopop/jsonschema"`)
	source := strings.Replace(readFile(t, dir, "status.go"), "//#enum,jsonschema", "//#enum", 1)
	source = strings.Replace(source, `StatusNone   Status = ""`, `StatusNone   Status = "" //#null`, 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(source), 0644))
	require.NoError(t, Rewrite(dir, nil, nil, false))
	result := readFile(t, dir, "status.go")
	assert.NotContains(t, result, `"github.com/invopop/jsonschema"`)
	assert.Contains(t, result, `"encoding/json"`)
	requireBuilds(t, dir)
}

// TestRewrite_NullAddedAndJSONSchemaRemoved verifies that a JSONSchema method
// generated without null value is removed when the null value is added in the
// same change, else the run fails with "was changed since".
func TestRewrite_NullAddedAndJSONSchemaRemoved(t *testing.T) {
	dir := writeFiles(t, map[string]string{"status.go": `package example

type Status string //#enum,jsonschema

const (
	StatusNone   Status = ""
	StatusActive Status = "active"
)
`})
	require.NoError(t, Rewrite(dir, nil, nil, false))
	require.Contains(t, readFile(t, dir, "status.go"), "JSONSchema()")
	source := strings.Replace(readFile(t, dir, "status.go"), "//#enum,jsonschema", "//#enum", 1)
	source = strings.Replace(source, `StatusNone   Status = ""`, `StatusNone   Status = "" //#null`, 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(source), 0644))
	require.NoError(t, Rewrite(dir, nil, nil, false))
	assert.NotContains(t, readFile(t, dir, "status.go"), "JSONSchema()")
	requireBuilds(t, dir)
}

// TestRewrite_ReadOnlyFile verifies that a read-only file is not rewritten,
// like with os.WriteFile, because read-only can mean not checked out.
func TestRewrite_ReadOnlyFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write read-only files")
	}
	source := "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n"
	dir := writeFiles(t, map[string]string{"status.go": source})
	require.NoError(t, os.Chmod(filepath.Join(dir, "status.go"), 0444))
	require.Error(t, Rewrite(dir, nil, nil, false))
	assert.Equal(t, source, readFile(t, dir, "status.go"))
}
