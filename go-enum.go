package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"os"
	"text/template"

	"github.com/ungerik/go-astvisit"
	"github.com/ungerik/go-enum/enums"
)

var (
	verbose   bool
	debug     bool
	printOnly bool
	printHelp bool
)

func main() {
	flag.BoolVar(&verbose, "verbose", false, "prints information to stdout of what's happening")
	flag.BoolVar(&debug, "debug", false, "inserts debug information")
	flag.BoolVar(&printOnly, "print", false, "prints to stdout instead of writing files")
	flag.BoolVar(&printHelp, "help", false, "prints this help output")
	flag.Parse()
	if printHelp {
		flag.PrintDefaults()
		os.Exit(2)
	}

	var (
		path       = "."
		verboseOut io.Writer
		resultOut  io.Writer
	)
	if args := flag.Args(); len(args) > 0 {
		path = args[0]
	}
	if verbose {
		verboseOut = os.Stdout
	}
	if printOnly {
		resultOut = os.Stdout
	}
	err := astvisit.RewriteWithReplacements(
		path,
		verboseOut,
		resultOut,
		debug,
		fileReplacements,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-enum error:", err)
		os.Exit(1)
	}
}

func fileReplacements(fset *token.FileSet, pkg *ast.Package, astFile *ast.File, filePath string, verboseOut io.Writer) (astvisit.NodeReplacements, astvisit.Imports, error) {
	// ast.Print(fset, astFile)
	// return nil, nil

	enums, err := enums.Find(fset, pkg, astFile)
	if err != nil {
		return nil, nil, err
	}
	if len(enums) == 0 {
		return nil, nil, nil
	}

	var (
		replacements astvisit.NodeReplacements
		imports      = make(astvisit.Imports)
	)
	for _, enum := range enums {
		var methods bytes.Buffer
		imports[`"fmt"`] = struct{}{}
		err := methodsValidValidateTempl.Execute(&methods, enum)
		if err != nil {
			return nil, nil, err
		}
		if enum.IsNullable() {
			err = methodsIsNullIsNotNullTempl.Execute(&methods, enum)
			if err != nil {
				return nil, nil, err
			}
			imports[`"encoding/json"`] = struct{}{}
			err = methodsMarshalJSONTempl.Execute(&methods, enum)
			if err != nil {
				return nil, nil, err
			}
			switch {
			case enum.IsStringType():
				imports[`"database/sql/driver"`] = struct{}{}
				err = methodsScanValueStringTempl.Execute(&methods, enum)
				if err != nil {
					return nil, nil, err
				}
			case enum.IsIntType():
				imports[`"database/sql/driver"`] = struct{}{}
				err = methodsScanValueIntTempl.Execute(&methods, enum)
				if err != nil {
					return nil, nil, err
				}
			}
		}

		debugID := "Replacement for " + enum.Type
		if len(enum.KnownMethods) == 0 {
			// No existing methods to replace,
			// insert new methods after last enum declaration
			replacements.AddInsertAfter(enum.LastEnumDecl, methods.Bytes(), debugID)
			continue
		}
		for i, method := range enum.KnownMethods {
			methodWithDoc := astvisit.NodeRange{method}
			if method.Doc != nil {
				methodWithDoc = append(methodWithDoc, method.Doc)
			}
			if i == 0 {
				// Replace the first existing method with all new ones
				replacements.AddReplacement(methodWithDoc, methods.Bytes(), debugID)
			} else {
				// Remove all further existing methods
				replacements.AddRemoval(methodWithDoc, debugID)
			}
		}
	}

	return replacements, imports, nil
}

var methodsValidValidateTempl = template.Must(template.New("").Parse(`
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

var methodsIsNullIsNotNullTempl = template.Must(template.New("").Parse(`
// IsNull returns true if {{.Recv}} is the null value {{.Null}}
func ({{.Recv}} {{.Type}}) IsNull() bool {
	return {{.Recv}} == {{.Null}}
}

// IsNotNull returns true if {{.Recv}} is not the null value {{.Null}}
func ({{.Recv}} {{.Type}}) IsNotNull() bool {
	return {{.Recv}} != {{.Null}}
}
`))

var methodsScanValueStringTempl = template.Must(template.New("").Parse(`
// Scan implements the database/sql.Scanner interface for {{.Type}}
func ({{.Recv}} *{{.Type}}) Scan(value interface{}) error {
	switch x := value.(type) {
	case string:
		*{{.Recv}} = {{.Type}}(x)
	case []byte:
		*{{.Recv}} = {{.Type}}(x)
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

var methodsScanValueIntTempl = template.Must(template.New("").Parse(`
// Scan implements the database/sql.Scanner interface for {{.Type}}
func ({{.Recv}} *{{.Type}}) Scan(value interface{}) error {
	switch x := value.(type) {
	case int64:
		*{{.Recv}} = {{.Type}}(x)
	case float64:
		*{{.Recv}} = {{.Type}}(x)
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

var methodsMarshalJSONTempl = template.Must(template.New("").Parse(`
// MarshalJSON implements encoding/json.Marshaler for {{.Type}}
// by returning the JSON null value for an empty (null) string.
func ({{.Recv}} {{.Type}}) MarshalJSON() ([]byte, error) {
	if {{.Recv}} == {{.Null}} {
		return []byte("null"), nil
	}
	return json.Marshal({{.Underlying}}({{.Recv}}))
}
`))
