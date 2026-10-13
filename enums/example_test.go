package enums_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/ungerik/go-enum/enums"
)

// writePackage writes the source of status.go to a new
// package directory and returns the path of the file.
func writePackage(source string) (filePath string, cleanup func()) {
	dir, err := os.MkdirTemp("", "go-enum-example")
	if err != nil {
		panic(err)
	}
	filePath = filepath.Join(dir, "status.go")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		panic(err)
	}
	return filePath, func() { os.RemoveAll(dir) }
}

// A type marked with //#enum gets methods for all consts of the type.
// With a resultOut writer, like the -print flag of the command,
// the rewritten file is written to it instead of to the file.
func ExampleRewrite() {
	filePath, cleanup := writePackage(`package example

type Status string //#enum

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)
`)
	defer cleanup()

	err := enums.Rewrite(filePath, nil, os.Stdout, false)
	if err != nil {
		panic(err)
	}
	// Output:
	// package example
	//
	// import "fmt"
	//
	// type Status string //#enum
	//
	// const (
	// 	StatusActive   Status = "active"
	// 	StatusInactive Status = "inactive"
	// )
	//
	// // Valid indicates if s is any of the valid values for Status
	// func (s Status) Valid() bool {
	// 	switch s {
	// 	case
	// 		StatusActive,
	// 		StatusInactive:
	// 		return true
	// 	}
	// 	return false
	// }
	//
	// // Validate returns an error if s is none of the valid values for Status
	// func (s Status) Validate() error {
	// 	if !s.Valid() {
	// 		return fmt.Errorf("invalid value %#v for type example.Status", s)
	// 	}
	// 	return nil
	// }
	//
	// // Enums returns all valid values for Status
	// func (Status) Enums() []Status {
	// 	return []Status{
	// 		StatusActive,
	// 		StatusInactive,
	// 	}
	// }
	//
	// // EnumStrings returns all valid values for Status as strings
	// func (Status) EnumStrings() []string {
	// 	return []string{
	// 		"active",
	// 		"inactive",
	// 	}
	// }
	//
	// // String implements the fmt.Stringer interface for Status
	// func (s Status) String() string {
	// 	return string(s)
	// }
}

// A const marked with //#null is the null value of the enum, which adds
// methods for null checks, JSON null and SQL NULL.
func ExampleRewrite_nullable() {
	filePath, cleanup := writePackage(`package example

type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "active"
)
`)
	defer cleanup()

	var result bytes.Buffer
	err := enums.Rewrite(filePath, nil, &result, false)
	if err != nil {
		panic(err)
	}
	for line := range strings.Lines(result.String()) {
		if strings.HasPrefix(line, "func ") {
			fmt.Print(line)
		}
	}
	// Output:
	// func (s Status) Valid() bool {
	// func (s Status) Validate() error {
	// func (Status) Enums() []Status {
	// func (Status) EnumStrings() []string {
	// func (s Status) String() string {
	// func (s Status) IsNull() bool {
	// func (s Status) IsNotNull() bool {
	// func (s *Status) SetNull() {
	// func (s Status) MarshalJSON() ([]byte, error) {
	// func (s *Status) UnmarshalJSON(j []byte) error {
	// func (s *Status) Scan(value any) error {
	// func (s Status) Value() (driver.Value, error) {
}

// A method marked with //#custom in its doc comment
// is kept instead of being generated.
func ExampleRewrite_custom() {
	filePath, cleanup := writePackage(`package example

type Status string //#enum

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// String returns the status with the type name.
//
//#custom
func (s Status) String() string { return "Status(" + string(s) + ")" }
`)
	defer cleanup()

	var result bytes.Buffer
	err := enums.Rewrite(filePath, nil, &result, false)
	if err != nil {
		panic(err)
	}
	for line := range strings.Lines(result.String()) {
		if strings.HasPrefix(line, "func ") {
			fmt.Print(line)
		}
	}
	// Output:
	// func (s Status) Valid() bool {
	// func (s Status) Validate() error {
	// func (Status) Enums() []Status {
	// func (Status) EnumStrings() []string {
	// func (s Status) String() string { return "Status(" + string(s) + ")" }
}

// ValidateRewrite reports missing or outdated methods without changing
// files, like the -validate flag of the command for CI checks.
func ExampleValidateRewrite() {
	filePath, cleanup := writePackage(`package example

type Status string //#enum

const StatusActive Status = "active"
`)
	defer cleanup()

	fmt.Println(enums.ValidateRewrite(filePath, nil, false))

	err := enums.Rewrite(filePath, nil, nil, false)
	if err != nil {
		panic(err)
	}
	fmt.Println(enums.ValidateRewrite(filePath, nil, false))
	// Output:
	// found 1 enum type(s) with missing or outdated methods
	// <nil>
}

// The values of an enum are the consts of its type, also with iota.
// Consts marked with //#exclude, like sentinels, and aliases of other
// values are no enum values.
func ExampleFind() {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "status.go", `package example

type Status string //#enum

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
	StatusDefault  Status = StatusActive
	StatusUnknown  Status = "" //#exclude
)

type Priority int //#enum

const (
	PriorityLow Priority = iota + 1
	PriorityMedium
	PriorityHigh
)
`, parser.ParseComments)
	if err != nil {
		panic(err)
	}

	found, err := enums.Find(fset, map[string]*ast.File{"status.go": file})
	if err != nil {
		panic(err)
	}
	for _, enum := range found {
		fmt.Println(enum.Type)
		for _, value := range enum.Values {
			fmt.Printf("  %s %s\n", value.Name, value.StringExpr)
		}
	}
	// Output:
	// Status
	//   StatusActive "active"
	//   StatusInactive "inactive"
	// Priority
	//   PriorityLow "1"
	//   PriorityMedium "2"
	//   PriorityHigh "3"
}
