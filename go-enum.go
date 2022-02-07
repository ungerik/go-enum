package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/ungerik/go-astvisit"
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
	err := astvisit.Rewrite(
		path,
		verboseOut,
		resultOut,
		func(fset *token.FileSet, pkg *ast.Package, astFile *ast.File, filePath string, verboseOut io.Writer) ([]byte, error) {
			return rewriteFile(fset, pkg, astFile, filePath, verboseOut, debug)
		},
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-enum error:", err)
		os.Exit(2)
	}
}

type enumSpec struct {
	Package    string
	Type       string
	Underlying string
	Recv       string
	Enums      []string
	Null       string
	// EnumValues []ast.Expr

	lastEnumDecl     *ast.GenDecl
	methodsToRewrite []*ast.FuncDecl
}

func (e *enumSpec) LastIndex() int {
	return len(e.Enums) - 1
}

func (e *enumSpec) IsStringType() bool {
	return e.Underlying == "string"
}

func (e *enumSpec) IsNullable() bool {
	return e.Null != ""
}

func rewriteFile(fset *token.FileSet, pkg *ast.Package, astFile *ast.File, filePath string, verboseOut io.Writer, debug bool) ([]byte, error) {
	// ast.Print(fset, astFile)
	// return nil, nil

	// Find enum types
	enums := make(map[string]*enumSpec)
	for _, decl := range astFile.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			for _, c := range typeSpec.Comment.List {
				if c.Text == "//#enum" {
					enums[typeSpec.Name.Name] = &enumSpec{
						Package:    pkg.Name,
						Type:       typeSpec.Name.Name,
						Underlying: astvisit.ExprString(typeSpec.Type),
					}
					break
				}
			}
		}
	}
	if len(enums) == 0 {
		return nil, nil
	}

	// Find enum values
	for _, decl := range astFile.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		// ast.Print(fset, genDecl)

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			enum, ok := enums[astvisit.ExprString(valueSpec.Type)]
			if !ok {
				continue
			}
			enum.lastEnumDecl = genDecl
			for _, name := range valueSpec.Names {
				enum.Enums = append(enum.Enums, name.Name)
				// enum.EnumValues = append(enum.EnumValues, valueSpec.Values[i])
			}
			if valueSpec.Comment != nil {
				for _, c := range valueSpec.Comment.List {
					if c.Text != "//#null" {
						continue
					}
					if enum.Null != "" {
						return nil, fmt.Errorf("second //#null enum encountered %s", valueSpec.Names[0].Name)
					}
					if len(valueSpec.Names) > 1 {
						return nil, fmt.Errorf("cant use //#null for multiple enums: %#v", valueSpec.Names)
					}
					enum.Null = valueSpec.Names[0].Name
					break
				}
			}
		}
	}

	// Find enum methods
	for _, decl := range astFile.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil {
			continue
		}
		recv := funcDecl.Recv.List[0]
		recvType := strings.TrimPrefix(astvisit.ExprString(recv.Type), "*")
		enum, ok := enums[recvType]
		if !ok {
			continue
		}
		enum.Recv = recv.Names[0].Name
		switch funcDecl.Name.Name {
		case "Valid", "Validate":
			enum.methodsToRewrite = append(enum.methodsToRewrite, funcDecl)
		case "IsNull", "IsNotNull", "MarshalJSON":
			if enum.IsNullable() {
				enum.methodsToRewrite = append(enum.methodsToRewrite, funcDecl)
			}
		case "Scan", "Value":
			if enum.IsNullable() && enum.IsStringType() {
				enum.methodsToRewrite = append(enum.methodsToRewrite, funcDecl)
			}
		}
	}

	// Set common method receiver name
	// if no existing method was encountered
	for _, enum := range enums {
		if enum.Recv == "" {
			enum.Recv = strings.ToLower(enum.Type[:1])
		}
	}

	// Write method replacements
	var (
		replacements astvisit.NodeReplacements
		importLines  = make(map[string]struct{})
	)
	for _, enum := range enums {
		var methods bytes.Buffer
		importLines[`"fmt"`] = struct{}{}
		err := templateValidValidate.Execute(&methods, enum)
		if err != nil {
			return nil, err
		}
		if enum.IsNullable() {
			err = templateIsNullIsNotNull.Execute(&methods, enum)
			if err != nil {
				return nil, err
			}
			importLines[`"encoding/json"`] = struct{}{}
			err = templateMarshalJSON.Execute(&methods, enum)
			if err != nil {
				return nil, err
			}
			if enum.IsStringType() {
				importLines[`"database/sql/driver"`] = struct{}{}
				err = templateScanValue.Execute(&methods, enum)
				if err != nil {
					return nil, err
				}
			}
		}

		debugID := "Replacement for " + enum.Type
		if len(enum.methodsToRewrite) == 0 {
			// No existing methods to replace,
			// insert new methods after last enum declaration
			replacements.AddInsertAfter(enum.lastEnumDecl, methods.Bytes(), debugID)
			continue
		}
		for i, method := range enum.methodsToRewrite {
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
	if len(replacements) == 0 {
		return nil, nil
	}

	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var rewritten []byte
	if debug {
		rewritten, err = replacements.DebugApply(fset, source)
		if err != nil {
			return nil, err
		}
	} else {
		rewritten, err = replacements.Apply(fset, source)
		if err != nil {
			return nil, err
		}
		rewritten, err = astvisit.FormatFileWithImports(fset, rewritten, importLines)
		if err != nil {
			return nil, err
		}
	}

	if bytes.Equal(source, rewritten) {
		return nil, nil
	}
	return rewritten, nil
}

var templateValidValidate = template.Must(template.New("").Parse(`
// Valid indicates if {{.Recv}} is any of the valid values for {{.Type}}
func ({{.Recv}} {{.Type}}) Valid() bool {
	switch s {
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

var templateIsNullIsNotNull = template.Must(template.New("").Parse(`
// IsNull returns true if {{.Recv}} is the null value {{.Null}}
func ({{.Recv}} {{.Type}}) IsNull() bool {
	return {{.Recv}} == {{.Null}}
}

// IsNotNull returns true if {{.Recv}} is not the null value {{.Null}}
func ({{.Recv}} {{.Type}}) IsNotNull() bool {
	return {{.Recv}} != {{.Null}}
}
`))

var templateScanValue = template.Must(template.New("").Parse(`
// Scan implements the database/sql.Scanner interface for {{.Type}}
func ({{.Recv}} *{{.Type}}) Scan(value interface{}) error {
	switch x := value.(type) {
	case string:
		*s = {{.Type}}(x)
	case []byte:
		*s = {{.Type}}(x)
	case nil:
		*s = {{.Null}}
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

var templateMarshalJSON = template.Must(template.New("").Parse(`
// MarshalJSON implements encoding/json.Marshaler for {{.Type}}
// by returning the JSON null value for an empty (null) string.
func ({{.Recv}} {{.Type}}) MarshalJSON() ([]byte, error) {
	if {{.Recv}} == {{.Null}} {
		return []byte("null"), nil
	}
	return json.Marshal({{.Underlying}}({{.Recv}}))
}
`))
