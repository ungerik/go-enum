# go-enum

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

## Installation

```bash
go install github.com/ungerik/go-enum@latest
```

## Quick Start

### 1. Define Your Enum

Add a type with the `//#enum` comment and const values:

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

Or specify a path:

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

### JSON Schema Support

Add `,jsonschema` to the `//#enum` comment:

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

## Command-Line Options

```bash
go-enum [options] [path]
```

Options:
- `-verbose`: Print information about what's happening
- `-debug`: Insert debug comments in generated code
- `-print`: Print generated code to stdout instead of writing files
- `-validate`: Check for missing or outdated enum methods without modifying files. Reports issues to stderr and exits with code 1 if any are found. Intended for CI.
- `-help`: Show help message

Exit codes:
- `0` — Success (no issues found in `-validate` mode, or generation completed)
- `1` — Error occurred (validation failed, file not found, invalid syntax, etc.)

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
| `Enums() []T` | Returns slice of all valid enum values |
| `EnumStrings() []string` | Returns slice of all enum values as strings |

### For String Enums

| Method | Description |
|--------|-------------|
| `String() string` | Implements `fmt.Stringer`, returns the string value |

### For Nullable Enums

| Method | Description |
|--------|-------------|
| `IsNull() bool` | Returns true if the value equals the null constant |
| `IsNotNull() bool` | Returns true if the value is not null |
| `SetNull()` | Sets the value to the null constant |
| `MarshalJSON() ([]byte, error)` | JSON marshaling with null support |
| `UnmarshalJSON([]byte) error` | JSON unmarshaling with null support |
| `Scan(any) error` | `database/sql.Scanner` implementation |
| `Value() (driver.Value, error)` | `database/sql/driver.Valuer` implementation |

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

## How It Works

1. **AST Parsing**: go-enum parses your Go source files using Go's AST
2. **Enum Detection**: Finds types marked with `//#enum` comments
3. **Value Discovery**: Collects all const values of the enum type
4. **Template Generation**: Generates methods using Go templates
5. **Smart Updates**: Either inserts new methods or replaces existing ones
6. **Import Management**: Automatically adds required imports

The tool is smart about updates:
- If no methods exist, it inserts them after the last enum const
- If methods already exist, it replaces them in-place
- Preserves your file structure and other code
- When generated methods are already up to date, the file is left byte-identical — no import reordering, no whitespace churn. Safe to run in `go generate` on every build.

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

String-based enums:
- `string`
- Any string-based type alias

Integer-based enums:
- `int`, `int8`, `int16`, `int32`, `int64`
- `uint`, `uint8`, `uint16`, `uint32`, `uint64`
- `byte` (alias for uint8)

## Requirements

- Go 1.23 or later
- Write access to source files (for code generation)

## Dependencies

- [github.com/ungerik/go-astvisit](https://github.com/ungerik/go-astvisit) - AST manipulation utilities
- [github.com/invopop/jsonschema](https://github.com/invopop/jsonschema) - JSON Schema generation (optional, only if using `,jsonschema`)

## Limitations

- Enum values must be const declarations of the enum type
- Enum type must have at least one value
- Only one null value per enum type
- Receiver name is auto-generated from type name (first letter, lowercase)

## License

See [LICENSE](LICENSE) file
