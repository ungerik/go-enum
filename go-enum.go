package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"os"
	"text/template"

	"github.com/ungerik/go-astvisit"
)

var (
	verbose   bool
	printOnly bool
	printHelp bool
)

func main() {
	flag.BoolVar(&verbose, "verbose", false, "prints information to stdout of what's happening")
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
	err := astvisit.Rewrite(path, verboseOut, resultOut, rewriteFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-enum error:", err)
		os.Exit(2)
	}
}

type enumSpec struct {
	Type string
	Recv string

	EnumNames  []string
	EnumValues []ast.Expr
	NullName   string

	funcDeclValid       *ast.FuncDecl
	funcDeclValidate    *ast.FuncDecl
	funcDeclIsNull      *ast.FuncDecl
	funcDeclIsNotNull   *ast.FuncDecl
	funcDeclScan        *ast.FuncDecl
	funcDeclValue       *ast.FuncDecl
	funcDeclMarshalJSON *ast.FuncDecl
}

func rewriteFile(fset *token.FileSet, pkg *ast.Package, astFile *ast.File, filePath string, verboseOut io.Writer) ([]byte, error) {
	// ast.Print(fset, astFile)
	// return nil, nil

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
					enums[typeSpec.Name.Name] = &enumSpec{Type: typeSpec.Name.Name}
					break
				}
			}
		}
	}
	if len(enums) == 0 {
		return nil, nil
	}

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
			for i := range valueSpec.Names {
				enum.EnumNames = append(enum.EnumNames, valueSpec.Names[i].Name)
				enum.EnumValues = append(enum.EnumValues, valueSpec.Values[i])
			}
			if valueSpec.Comment != nil {
				for _, c := range valueSpec.Comment.List {
					if c.Text != "//#null" {
						continue
					}
					if enum.NullName != "" {
						return nil, fmt.Errorf("second //#null enum encountered %s", valueSpec.Names[0].Name)
					}
					if len(valueSpec.Names) > 1 {
						return nil, fmt.Errorf("cant use //#null for multiple enums: %#v", valueSpec.Names)
					}
					enum.NullName = valueSpec.Names[0].Name
					break
				}
			}
		}

		for _, decl := range astFile.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv == nil {
				continue
			}
			recv := funcDecl.Recv.List[0]
			recvType := astvisit.ExprString(recv.Type)
			enum, ok := enums[recvType]
			if !ok {
				continue
			}
			switch funcDecl.Name.Name {
			case "Valid":
				enum.funcDeclValid = funcDecl
			case "Validate":
				enum.funcDeclValidate = funcDecl
			case "IsNull":
				enum.funcDeclIsNull = funcDecl
			case "IsNotNull":
				enum.funcDeclIsNotNull = funcDecl
			case "Scan":
				enum.funcDeclScan = funcDecl
			case "Value":
				enum.funcDeclValue = funcDecl
			case "MarshalJSON":
				enum.funcDeclMarshalJSON = funcDecl
			}
			// recvName := recv.Names[0].Name
			// methodName := funcDecl.Name
			// methodSignature := astvisit.FuncTypeString(funcDecl.Type)
		}
		return nil, nil
	}

	return nil, nil
}

var templateValid = template.Must(template.New("").Parse(`func ({{.Recv}} {{.Type}}) Valid() bool {
	switch s {
	case
		{{$last := len (slice .Enums 1)}}{{range $i, $e := .Enums}}{{$e}}{{if $i lt $last}},{{else}}:{{end}}{{end}}
		return true
	}
	return false
}
`))

var templateValidate = template.Must(template.New("").Parse(`func ({{.Recv}} {{.Type}}) Validate() error {
	if !{{.Recv}}.Valid() {
		return fmt.Errorf("invalid value %#v for type {{.Type}}", {{.Recv}})
	}
	return nil
}
`))

var templateIsNull = template.Must(template.New("").Parse(`func ({{.Recv}} {{.Type}}) IsNull() bool {
	return {{.Recv}} == {{.NullName}}
}
`))

var templateIsNotNull = template.Must(template.New("").Parse(`func ({{.Recv}} {{.Type}}) IsNotNull() bool {
	return {{.Recv}} != {{.NullName}}
}
`))

var templateScan = template.Must(template.New("").Parse(`// Scan implements the database/sql.Scanner interface.
func ({{.Recv}} *{{.Type}}) Scan(value interface{}) error {
	switch x := value.(type) {
	case string:
		*s = {{.Type}}(x)
	case []byte:
		*s = {{.Type}}(x)
	case nil:
		*s = {{.NullName}}
	default:
		return fmt.Errorf("can't scan SQL value of type %T as {{.Type}}", value)
	}
	return nil
}
`))

var templateValue = template.Must(template.New("").Parse(`// Value implements the driver database/sql/driver.Valuer interface.
func ({{.Recv}} {{.Type}}) Value() (driver.Value, error) {
	if {{.Recv}} == {{.NullName}} {
		return nil, nil
	}
	return string({{.Recv}}), nil
}
`))

var templateMarshalJSON = template.Must(template.New("").Parse(`// MarshalJSON implements encoding/json.Marshaler
// by returning the JSON null for an empty/null string.
func ({{.Recv}} {{.Type}}) MarshalJSON() ([]byte, error) {
	if {{.Recv}} == {{.NullName}} {
		return []byte("null"), nil
	}
	return json.Marshal(string({{.Recv}}))
}
`))
