/*
Package enums provides enum detection, extraction, and code generation from Go AST.

It scans Go source files for types marked with the //#enum comment,
extracts all related information including enum values, methods,
and configuration flags, then generates type-safe validation and utility methods.

The main entry point is Rewrite, which processes Go source files and generates
enum methods by finding enum types (via Find), applying code generation templates,
and updating the source files with the generated methods.
*/
package enums

import (
	"go/ast"
	"strings"
)

// Enum represents a discovered enum type with all its metadata.
// It contains information about the enum type, its values, and configuration.
type Enum struct {
	// File is the source file name where the enum is defined
	File string
	// Line is the line number where the enum type is declared
	Line int
	// Package is the package name
	Package string
	// Type is the enum type name
	Type string
	// Underlying is the underlying type (e.g., "string", "int")
	Underlying string
	// Recv is the method receiver name (auto-generated or from existing methods)
	Recv string
	// Enums is the list of enum constant names
	Enums []string
	// Literals is the list of enum constant values as strings
	Literals []string
	// JSONSchemaEnum is the list of values for JSON Schema enum field
	JSONSchemaEnum []string
	// Null is the name of the null enum value (if //#null is used)
	Null string
	// JSONSchema indicates if ,jsonschema flag was set
	JSONSchema bool

	// LastEnumDecl is the AST declaration of the last enum const
	LastEnumDecl ast.Decl
	// KnownMethods are existing enum methods that will be replaced
	KnownMethods []*ast.FuncDecl
}

// IsStringType returns true if the underlying type is string.
func (e *Enum) IsStringType() bool {
	return e.Underlying == "string"
}

// IsIntType returns true if the underlying type is an integer type.
func (e *Enum) IsIntType() bool {
	return e.Underlying == "byte" ||
		strings.HasPrefix(e.Underlying, "int") ||
		strings.HasPrefix(e.Underlying, "uint")
}

// IsNullable returns true if the enum has a null value defined.
func (e *Enum) IsNullable() bool {
	return e.Null != ""
}

// LastIndex returns the index of the last enum value.
func (e *Enum) LastIndex() int {
	return len(e.Enums) - 1
}

// JSONType returns the JSON Schema type for this enum's underlying type.
func (e *Enum) JSONType() string {
	switch {
	case e.IsStringType(), e.Underlying == "time.Time":
		return "string"
	case e.IsIntType(), strings.HasPrefix(e.Underlying, "float"):
		return "number"
	case e.Underlying == "bool":
		return "boolean"
	case strings.HasPrefix(e.Underlying, "[]"):
		return "array"
	case strings.HasPrefix(e.Underlying, "map["), strings.HasPrefix(e.Underlying, "struct{"):
		return "object"
	default:
		return "string"
	}
}
