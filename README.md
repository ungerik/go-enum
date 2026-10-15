# go-enum

[![Go Reference](https://pkg.go.dev/badge/github.com/ungerik/go-enum.svg)](https://pkg.go.dev/github.com/ungerik/go-enum) [![Go Report Card](https://goreportcard.com/badge/github.com/ungerik/go-enum)](https://goreportcard.com/report/github.com/ungerik/go-enum) [![Go](https://github.com/ungerik/go-enum/actions/workflows/go.yml/badge.svg)](https://github.com/ungerik/go-enum/actions/workflows/go.yml) [![Go version](https://img.shields.io/github/go-mod/go-version/ungerik/go-enum)](https://github.com/ungerik/go-enum) [![license](https://img.shields.io/badge/license-MIT-red.svg?style=flat)](https://github.com/ungerik/go-enum/blob/main/LICENSE)

Go code generator for type-safe enums with validation, JSON marshaling, and database support.

## Features

- **Type-Safe Enums**: Define enums using Go const declarations with a special comment marker
- **Automatic Code Generation**: Generates validation, conversion, and utility methods
- **Nullable Support**: Optional null value handling with proper JSON and SQL marshaling
- **String/Int Types**: Works with both string and integer-based enums
- **JSON Schema**: Optional JSON Schema generation for API documentation
- **Database Integration**: `database/sql.Scanner` and `driver.Valuer` implementations for nullable enums
- **AST-Based**: Uses Go's AST for safe, precise code generation
- **In-Place Updates**: Intelligently updates existing methods without breaking your code
- **Customizable**: Preserve hand-written methods with `//#custom` when the generated version doesn't fit

## Installation

```bash
go install github.com/ungerik/go-enum@latest
```

## Quick Start

### 1. Define Your Enum

Add a type with the `//#enum` comment and const values.
The values are all consts of exactly that type, also with `iota`
or conversions like `Status("pending")`:

```go
package example

// Status represents order status
type Status string //#enum

const (
	StatusPending   Status = "pending"
	StatusConfirmed Status = "confirmed"
	StatusShipped   Status = "shipped"
	StatusDelivered Status = "delivered"
	StatusCancelled Status = "cancelled"
)
```

### 2. Generate Code

Run go-enum in your package directory:

```bash
go-enum
```

Or specify a path, like `./...` for all packages below the current directory:

```bash
go-enum ./path/to/package
```

### 3. Use Generated Methods

The tool generates these methods automatically:

```go
// Validation
status := StatusPending
if status.Valid() {
	// status is a valid enum value
}

err := status.Validate()
if err != nil {
	// status is invalid
}

// Fall back to a default value if status is invalid
status = status.ValidOr(StatusPending)

// Get all enum values
allStatuses := Status("").Enums()
// Returns: []Status{StatusPending, StatusConfirmed, ...}

// Get enum values as strings
statusStrings := Status("").EnumStrings()
// Returns: []string{"pending", "confirmed", ...}
```

## Advanced Features

### Nullable Enums

Add a null value with the `//#null` comment:

```go
type Priority int //#enum

const (
	PriorityNull Priority = 0 //#null
	PriorityLow  Priority = 1
	PriorityMid  Priority = 2
	PriorityHigh Priority = 3
)
```

Generated methods for nullable enums:

```go
priority := PriorityNull

// Check for null
if priority.IsNull() {
	// Handle null case
}

if priority.IsNotNull() {
	// Handle non-null case
}

if priority.ValidNotNull() {
	// Handle a valid, non-null value
}

// Fall back to a default value if priority is null or invalid
priority = priority.ValidNotNullOr(PriorityMid)

// Set to null
priority.SetNull()

// JSON marshaling: null value becomes JSON null
data, _ := json.Marshal(priority) // Returns: []byte("null")

// JSON unmarshaling: JSON null becomes null value
json.Unmarshal([]byte("null"), &priority) // Sets priority to PriorityNull

// Database support
var p Priority
err := db.QueryRow("SELECT priority FROM tasks WHERE id = ?", id).Scan(&p)
// NULL values in database become PriorityNull

_, err = db.Exec("INSERT INTO tasks (priority) VALUES (?)", priority)
// PriorityNull values become NULL in database
```

### Excluding Values (`//#exclude`)

A const of the enum type with a `//#exclude` line comment is not an enum
value: it is not listed by `Enums()`, `EnumStrings()` and `JSONSchema()`
and not used in `Valid()`. Use it for sentinels and aliases:

```go
type Code string //#enum

const (
	CodeA Code = "A"
	CodeB Code = "B"

	CodeDefault Code = "A" //#exclude
)

const CodeInvalid Code = "" //#exclude
```

Excluded consts are also skipped by the duplicate value check,
so an alias like `CodeDefault` is not an error.
An alias initialized with another enum const, like `CodeFallback Code = CodeA`,
is skipped without a marker. Other values that use an enum const,
like `CodeAB Code = CodeA + "B"`, are evaluated and are enum values
with their own value.
`Valid()` compares values, so it still accepts an alias like `CodeDefault`
because its value equals `CodeA`. Only a sentinel with a value that
no other enum value has, like `CodeInvalid`, is rejected by `Valid()`.

The markers `//#null` and `//#exclude` must be written exactly like this
as comment at the end of the const line. Any other comment of an enum const
that starts with `#` followed by one of the reserved marker words `enum`,
`null`, `exclude` or `custom`, like `// #exclude`, `//#exclude legacy` or
`//#exclude` on the line above the const, is an error, so that a misused
marker can't silently make a sentinel valid. Other words are ignored,
including comments of other tools like `//#nosec G101` or `//#region`
and misspellings like `//#exlude`. A marker comment that is not at the end
of a type or const line or in the doc comment of a method, like
`//#exclude` separated from its const by an empty line or `) //#null`,
is also an error, and so is a marker at a var or at a const that is not of
an enum type, like `StatusNone = "" //#null` without type.

The generated methods use the packages `fmt`, `bytes`, `encoding/json`,
`database/sql/driver` and `github.com/invopop/jsonschema` by their package
names, so a declaration of the package or an import of the file of the enum
type with the name of a package used by the methods that are not marked with
`//#custom`, like an enum value named `fmt` or an import of
`github.com/goccy/go-json` without import name that is used as `json`,
is an error.

The `//#null` const must be declared in the file of the enum type,
because it changes which methods are generated in that file.

### Values in Other Files

Enum values are collected from the file declaring the enum type and from
the other files of the package that are compiled in every build of it,
ordered by file name and then by their position within the file.
The generated methods are always written to the file declaring the enum type.

Like with the go tool, files ending in `_test.go` or starting with `_` or `.`
are ignored. Files with build constraints, a `//go:build` line, a file name
suffix like `_linux.go` or `_arm64.go` or an `import "C"` for cgo, are also
ignored if the go tool does not compile them in every build of the file of
the enum type, because the generated methods would reference values that
don't exist in some builds. So for an enum type in `status_linux.go`, the
values in a file with `//go:build linux` or `//go:build unix` are used,
but not the values in `status_linux_arm64.go`. Build constraints are
compared like the go tool sets build tags: one GOOS, which can set more
tags like `android` sets `linux` and `unix`, one GOARCH, the `go1.N` tags up
to one Go version of at least 1.18, because the generated methods use `any`,
one of `gc` and `gccgo`, and never `ignore`, so files with
`//go:build ignore`, like a script for `go run`, are not part of the package,
and an enum type in such a file only has the values of that file.
Other GOOS values than the ones in the build constraints are always
possible, so cover them with a negated build constraint like
`//go:build !linux`.
For platform-specific values, declare the enum type with its values in each
platform-specific file. Each declaration gets its own generated methods,
which also use the values of the files without build constraints:

```go
// signal.go
package example

const SignalInterrupt Signal = 2
```

```go
// signal_linux.go
package example

type Signal int //#enum

const (
	SignalHangup Signal = 1
	SignalPoll   Signal = 29
)
```

```go
// signal_other.go
//go:build !linux

package example

type Signal int //#enum

const SignalHangup Signal = 1
```

On Linux `Signal(0).Enums()` returns `SignalInterrupt`, `SignalHangup` and
`SignalPoll`, on other platforms `SignalInterrupt` and `SignalHangup`.

An untyped const converted to the enum type, like
`const MethodGet = Method(http.MethodGet)`, is also an enum value.
An untyped const computed from such a value with an operator or with `min`
or `max`, like `const MethodGetX = MethodGet + "X"`, can't be evaluated and
is an error: declare it with the enum type, like
`const MethodGetX Method = MethodGet + "X"`, or mark it with
[`//#exclude`](#excluding-values-exclude) if it is no enum value.

The values are evaluated without loading imported packages, so a value
that uses an import, like `const MethodGet Method = http.MethodGet`,
is referenced by its name in the generated methods, for example as
`string(MethodGet)` in `EnumStrings()`.

Only the file declaring the enum type is rewritten, so a method in another
file, also in a file with build constraints, that the generator would produce,
like a hand-written `String()` per platform, must be
marked with [`//#custom`](#hand-written-methods-custom), else it is an error.
Such a method is then not generated, so it must be declared in every build
of the file of the enum type, like in `status_linux.go` and in a file with
`//go:build !linux`, else it is an error:

```go
// status.go
package example

type Status string //#enum

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)
```

```go
// status_linux.go
package example

// String returns the status for systemd units.
//
//#custom
func (s Status) String() string { return "unit-" + string(s) }
```

```go
// status_other.go
//go:build !linux

package example

// String returns the status.
//
//#custom
func (s Status) String() string { return string(s) }
```

go-enum generates all other methods of `Status` in `status.go`, but no `String()`.

`go test` compiles the internal `_test.go` files of the package together with
the generated methods, so a method there that would be generated, or a
package-level name like `driver` of a package used by the generated methods,
is an error. Their values are not used, because they are not compiled in
regular builds, and a test file that doesn't parse or has an invalid
`//go:build` line, like one being edited, is skipped.

### JSON Schema Support

Add `,jsonschema` to the `//#enum` comment. The marker must be written
exactly as `//#enum` at the end of the type line, so `// #enum` is an error.
Other words like `//#enums` and unknown flags are ignored:

```go
type Color string //#enum,jsonschema

const (
	ColorRed   Color = "red"
	ColorGreen Color = "green"
	ColorBlue  Color = "blue"
)
```

Generated method:

```go
import "github.com/invopop/jsonschema"

schema := Color("").JSONSchema()
// Returns JSONSchema with enum values for OpenAPI/Swagger documentation
```

For nullable enums with JSON Schema:

```go
type Size int //#enum,jsonschema

const (
	SizeNull  Size = 0 //#null
	SizeSmall Size = 1
	SizeLarge Size = 2
)
```

The generated schema uses `oneOf` to allow either the enum values or null:

```json
{
  "oneOf": [
    {
      "type": "number",
      "enum": [1, 2]
    },
    {
      "type": "null"
    }
  ],
  "default": 0
}
```

### Hand-written Methods (`//#custom`)

Sometimes you need a hand-written version of a method that the generator
would normally produce — for example, an `UnmarshalJSON` that accepts a
legacy JSON shape in addition to the current one. Mark the method's doc
comment with `//#custom` and the generator will preserve it verbatim:

```go
type Status string //#enum

const (
	StatusNone   Status = "" //#null
	StatusActive Status = "ACTIVE"
)

// UnmarshalJSON accepts legacy bool `true`/`false` in addition to the
// string enum form, so old JSON fixtures keep parsing.
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
```

Rules:

- The marker applies only to the immediately-following method declaration.
- It is meaningful only for methods the generator would otherwise produce
  (`Valid`, `Validate`, `ValidOr`, `ValidNotNull`, `ValidNotNullOr`, `Enums`,
  `EnumStrings`, `String`, `IsNull`, `IsNotNull`, `SetNull`, `MarshalJSON`,
  `UnmarshalJSON`, `Scan`, `Value`, `JSONSchema`). On other methods it is
  a no-op.
- A custom-marked method is not added to the generated block, so no
  duplicate is produced. Other methods on the same type continue to be
  regenerated normally.
- `-validate` mode treats custom-marked methods as up-to-date, so they do
  not cause CI failures.
- `gofmt` may normalize `//#custom` to `// #custom` (with a space after
  the slashes) on re-save; both forms are recognized.
- Any other doc comment line of these methods that starts with `#`
  followed by a reserved marker word, like `//#custom legacy JSON`
  or `//#exclude`, is an error. Other words like a misspelled `// #custum`,
  doc comment headings like `// # Examples` and comments of other tools
  like `//#nosec G115` are ignored. A method with a misspelled marker
  counts as unmarked, see the next rule.
- A method the generator would produce without `//#custom` is replaced
  only if it was generated before, recognized by the whole doc comment of
  the generated method, with or without a period at the end of its lines
  like added by the godot linter, and also by its code for `UnmarshalJSON`,
  whose doc comment is a common phrase. Else it is an error, so that a
  hand-written method without the marker and without exactly that doc
  comment is not silently replaced. A hand-written method with the doc
  comment of the generated method is replaced, mark it with `//#custom`
  to keep it.
  Comment lines of other tools in the doc comment of a replaced method,
  like `//nolint:gosec`, are removed with it, mark the method with
  `//#custom` to keep them.
- A method that is never generated for the underlying type of the enum,
  like `String` of an integer enum, is hand-written, also with the doc
  comment of a generated method. If it also has the code of a generated
  method, like after changing the underlying type from string to int, it is
  an error, delete it or mark it with `//#custom` to keep it.
- A generated method that is no longer generated, for example after removing
  `//#null`, is removed if it is unchanged. If it was changed since,
  it is an error, delete it or mark it with `//#custom` to keep it.
- In other files of the package, where the generator can't replace methods,
  every method it would produce must be marked with `//#custom`.

## Command-Line Options

```bash
go-enum [options] [path]
```

The path is one of:
- A package directory, by default the current directory `.`: the methods of all enums of the package are generated.
- A directory followed by `...`, like `./...`: the methods of the packages in that directory and in all directories below it are generated. Like with the go tool, directories starting with `.` or `_`, `testdata`, `vendor` and `node_modules` directories and directories of nested modules with their own `go.mod` are skipped, and directories without Go files are ignored, but an error if no directory has Go files. A single directory without Go files is an error. An error in one package doesn't stop the others: all errors and, with `-validate`, all outdated enums are reported.
- A Go file: only the methods of the enums declared in that file are generated. The other files of its package are still read for enum values and existing methods. A file that is not a Go file of the package, like a `_test.go` file, is an error.

Only one path argument is supported, and options must come before it, so `go-enum ./... -validate` is an error with exit code `2`.

Options:
- `-verbose`: Print the parsed package directories and the changed or unchanged files to stdout
- `-debug`: Show the replaced source before every generated method like a merge conflict. Only with `-print`, because the output is no Go code
- `-print`: Print the complete content of every file that would change to stdout instead of writing it, up-to-date files are not printed
- `-validate`: Check for missing or outdated enum methods without modifying files. Reports issues to stderr and exits with code 1 if any are found. Intended for CI.
- `-help`: Print the options

Exit codes:
- `0` — Success (no issues found in `-validate` mode, or generation completed)
- `1` — Error occurred (validation failed, file not found, invalid syntax, etc.)
- `2` — `-help` was used, an option is invalid, `-debug` was used without `-print`, or more than one path was given (`-h` prints the usage and exits with `0`)

Examples:

```bash
# Generate with verbose output
go-enum -verbose

# Preview generated code without writing
go-enum -print

# Generate for specific package
go-enum ./internal/models

# CI check: fail the build if any enum methods are missing or outdated
go-enum -validate ./...
```

## Generated Methods Reference

### For All Enums

| Method | Description |
|--------|-------------|
| `Valid() bool` | Returns true if the value is a valid enum constant |
| `Validate() error` | Returns an error if the value is invalid |
| `ValidOr(defaultVal T) T` | Returns the value if it is valid, else `defaultVal` |
| `Enums() []T` | Returns slice of all valid enum values |
| `EnumStrings() []string` | Returns slice of all enum values as strings |

### For String Enums

| Method | Description |
|--------|-------------|
| `String() string` | Implements `fmt.Stringer`, returns the string value |

### For Nullable Enums

| Method | Description |
|--------|-------------|
| `ValidNotNull() bool` | Returns true if the value is valid and not null |
| `ValidNotNullOr(defaultVal T) T` | Returns the value if it is valid and not null, else `defaultVal` |
| `IsNull() bool` | Returns true if the value equals the null constant |
| `IsNotNull() bool` | Returns true if the value is not null |
| `SetNull()` | Sets the value to the null constant |
| `MarshalJSON() ([]byte, error)` | JSON marshaling with null support |
| `UnmarshalJSON([]byte) error` | JSON unmarshaling with null support |
| `Scan(any) error` | `database/sql.Scanner` implementation |
| `Value() (driver.Value, error)` | `database/sql/driver.Valuer` implementation |

`Scan` and `Value` are only generated for string and integer enums,
see [Supported Types](#supported-types).

### For JSON Schema Enums

| Method | Description |
|--------|-------------|
| `JSONSchema() *jsonschema.Schema` | Returns JSON Schema definition |

## Examples

### HTTP API with Validation

```go
type OrderStatus string //#enum,jsonschema

const (
	OrderPending   OrderStatus = "pending"
	OrderProcessing OrderStatus = "processing"
	OrderComplete  OrderStatus = "complete"
)

func UpdateOrderHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrderID string      `json:"order_id"`
		Status  OrderStatus `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate enum value
	if err := req.Status.Validate(); err != nil {
		http.Error(w, "Invalid status: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Process order...
}
```

### Database Model with Nullable Enum

```go
type TaskPriority int //#enum

const (
	TaskPriorityUnset TaskPriority = 0 //#null
	TaskPriorityLow   TaskPriority = 1
	TaskPriorityHigh  TaskPriority = 2
)

type Task struct {
	ID       int
	Name     string
	Priority TaskPriority
}

func GetTask(db *sql.DB, id int) (*Task, error) {
	task := &Task{}
	err := db.QueryRow(
		"SELECT id, name, priority FROM tasks WHERE id = ?", id,
	).Scan(&task.ID, &task.Name, &task.Priority)

	// Database NULL automatically becomes TaskPriorityUnset
	if err != nil {
		return nil, err
	}
	return task, nil
}
```

### Enum Iteration

```go
type Weekday string //#enum

const (
	Monday    Weekday = "monday"
	Tuesday   Weekday = "tuesday"
	Wednesday Weekday = "wednesday"
	Thursday  Weekday = "thursday"
	Friday    Weekday = "friday"
	Saturday  Weekday = "saturday"
	Sunday    Weekday = "sunday"
)

// Print all weekdays
for _, day := range Weekday("").Enums() {
	fmt.Println(day)
}

// Create a dropdown menu
weekdayOptions := Weekday("").EnumStrings()
// Returns: []string{"monday", "tuesday", ...}
```

### Values with `iota` and Conversions

Values are evaluated like by the Go compiler, so `iota`, implicitly repeated
consts and conversions work:

```go
type Priority int //#enum

const (
	PriorityLow Priority = iota + 1
	PriorityMedium
	PriorityHigh
)
```

`Priority(0).EnumStrings()` returns the evaluated values `"1"`, `"2"` and `"3"`.

A value converted from a const of an imported package is an enum value too.
Imported packages are not loaded, so such a value is referenced by its name
in the generated methods:

```go
type Method string //#enum

const (
	MethodGet  = Method(http.MethodGet)
	MethodPost = Method(http.MethodPost)
)
```

Here `EnumStrings()` returns `string(MethodGet)` and `string(MethodPost)`,
which are `"GET"` and `"POST"`.

## How It Works

1. **AST Parsing**: go-enum parses the Go source files of a package using Go's AST
2. **Enum Detection**: Finds types marked with `//#enum` comments
3. **Value Discovery**: Type checks the files of the package that are compiled in every build of the file of the enum type with `go/types` and collects all consts of the enum type with their values
4. **Template Generation**: Generates methods using Go templates
5. **Smart Updates**: Either inserts new methods or replaces the methods it generated before
6. **Import Management**: Adds the imports the generated methods use and removes the ones they no longer use

The tool is smart about updates:
- If no methods exist, it inserts them after the last enum const in the file of the enum type, or after the type declaration if that file has no enum consts
- If methods were generated before, it replaces them in-place
- Methods that are no longer generated, for example after removing `//#null`, are removed if they are unchanged, else it is an error
- Preserves your file structure and other code
- The files of a package are only written after the methods of all its enums were generated without error
- When generated methods are already up to date, the file is left byte-identical — no import reordering, no whitespace churn. Safe to run in `go generate` on every build.
- A file is written to a temporary file in the same directory that is synced to disk and then renamed to the file, so an interrupted run or a crash can't leave a truncated source file. The file mode is kept, and so are the owner and group as far as the process is permitted to change them. A file whose group can't be given to the new file is written in place, so that its group permissions don't apply to another group. A hard linked file is replaced by a new file, and its ACLs and extended attributes are not kept. A read-only file is not rewritten, which is an error. In a directory without write permission, a writable file is written in place like with `os.WriteFile`, and so is a writable file that can't be replaced, like a file of another user in a directory with the sticky bit like `/tmp`, or a bind mounted file. Other errors, like a full disk, leave the file unchanged.
- A file with CRLF line endings, like in a Windows checkout, keeps them. A file with only some CRLF line endings gets LF line endings, like with gofmt.

The [Design](#design) section explains why it works this way.

## Best Practices

1. **Always run after changing enums**: Add `go-enum` to your `go generate` workflow
2. **Use meaningful names**: Prefix enum constants with the type name (e.g., `StatusPending`)
3. **Document your enums**: Add godoc comments to enum types and values
4. **Validate user input**: Always call `Validate()` on enum values from external sources
5. **Use nullable sparingly**: Only add `//#null` when you truly need to distinguish "not set" from valid values
6. **Commit generated code**: Include generated methods in version control

## Integration with `go generate`

Add a generate directive to your file:

```go
//go:generate go-enum

package mypackage

type MyEnum string //#enum
```

Then run:

```bash
go generate ./...
```

## Supported Types

Enum types can be declared in any package, also in `main` packages.

String-based enums:
- `string`
- Any string-based type of the package

Integer-based enums:
- `int`, `int8`, `int16`, `int32`, `int64`
- `uint`, `uint8`, `uint16`, `uint32`, `uint64`
- `byte` (alias for uint8)
- Any integer-based type of the package

Float and bool-based enums get all methods except `String` and the nullable `Scan` and `Value`.

## Requirements

- Go 1.26 or later
- Write access to source files (for code generation)

## Dependencies

- [golang.org/x/tools](https://pkg.go.dev/golang.org/x/tools) - Import handling of the rewritten files
- [github.com/invopop/jsonschema](https://github.com/invopop/jsonschema) - JSON Schema generation (optional, only if using `,jsonschema`)

## Limitations

- Enum values must be const declarations of the enum type
- The underlying type must be a basic type or a type of the package, not an imported type like `time.Duration`
- Enum type must have at least one value
- Only one null value per enum type
- Enum values in other files that are not compiled in every build of the file of the enum type are not used, and methods the generator would produce in other files must be marked `//#custom` (see [Values in Other Files](#values-in-other-files)). Every combination of build tags other than GOOS, GOARCH, `go1.N`, `gc` and `gccgo`, like `cgo` or custom tags, is evaluated, so with more than 12 of them in the build constraints of two files, the values of the other file are not used, its methods are compared like the methods of files compiled together with the enum type, and its `//#custom` methods are reported as missing in some builds
- The receiver name is taken from existing methods, else it is the first letter of the type name in lowercase, or `e`, `recv`, `e1`, `e2` and so on if that collides with an identifier used by the generated methods

## Design

Why go-enum works the way it does, and what that trades off.

### Values and Methods Across the Package

go-enum only rewrites the file declaring the enum type, but the generated
methods are compiled together with every file of the package. So they may only
reference consts that exist in every build of that file, and a method declared
in another file must not be generated again. Reading only the file of the enum
type, like before v1.2.0, missed the values of other files, and workarounds
like `const Invalid = Code("")` in that file made sentinels valid values.

The build constraints of every file are evaluated for all builds the go tool can
produce (see [Values in Other Files](#values-in-other-files)), which decides for
each file whether its values are used and its methods are compared:

```
file                with the enum type in status_linux.go     values     methods
values.go           compiled in every build                  used       compared
values_unix.go      //go:build unix, in every build          used       compared
values_arm64.go     compiled in some builds (linux/arm64)    not used   compared
string_windows.go   never compiled together                  ignored    ignored
```

Trade-offs:
- Values that exist only in some builds are not used, without an error.
  Reporting each of them would be too strict for per-platform values.
- GOOS values that no build constraint names are always possible, so
  `//#custom` methods for a list of platforms need a negated fallback like
  `//go:build !linux`.
- Every combination of the independent build tags is evaluated, so with
  more than 12 of them the safe answer is used: values are not used and
  methods are compared.

Comparing the text of build constraints was rejected, because it silently
dropped values when the same constraint was written differently, like
`status_linux.go` with an additional `//go:build linux` line.

### Values Evaluated With go/types

The package is type checked with `go/types` without loading imported
packages. Constant values are evaluated exactly like by the compiler, so `iota`,
implicit repetition and conversions work, and duplicates written differently,
like `1` and `0x1`, are detected. Copying the value expressions as
text, like before v1.2.0, produced wrong or uncompilable `EnumStrings()` for
them. Not loading imports needs no `go list`, module cache or compilable
package, at the cost that values computed from imports can't be evaluated:
they are referenced by name, and their duplicates can't be detected.

### Recognizing Generated Methods

Before v1.2.0 every method with the name of a generated method was replaced,
which silently overwrote hand-written methods. Now a method is only replaced
if it was generated before, recognized by the whole generated doc comment, and
for `UnmarshalJSON`, whose doc comment is a common phrase, also by its code.
Every other method that would be generated is an error, and `//#custom` keeps
it. A method that is never generated for the underlying type, like `String`
of an integer enum, is hand-written. If it has the code of a generated method
it is an error instead of being removed, because `string(x)` is correct for
rune and byte enums.

The trade-off: a hand-written method that copies the generated doc comment
is replaced, and comment lines of other tools in the doc comment of a
replaced method, like `//nolint`, are removed with it.

### Fail Loud, Write Safely

Problems that would silently produce wrong or uncompilable code are errors:
reserved marker words in the wrong form or at the wrong place, markers at
consts that are not of an enum type, and declarations or imports named like
a package used by the generated methods. All errors of a package are
reported at once, and its files are only written after the methods of all
its enums were generated, each through a temporary file that is synced to
disk and replaces it, or in place where it can't be replaced with its
group. Files with up-to-date methods are not written at all,
so their bytes, including the import layout, stay unchanged.

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for the changes of each release.

## License

See [LICENSE](LICENSE) file
