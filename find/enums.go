package find

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"strings"

	"github.com/ungerik/go-astvisit"
)

type Enum struct {
	File           string
	Line           int
	Package        string
	Type           string
	Underlying     string
	Recv           string
	Enums          []string
	Literals       []string
	JSONSchemaEnum []string
	Null           string
	JSONSchema     bool

	LastEnumDecl ast.Decl
	KnownMethods []*ast.FuncDecl
}

func (e *Enum) IsStringType() bool {
	return e.Underlying == "string"
}

func (e *Enum) IsIntType() bool {
	return e.Underlying == "byte" ||
		strings.HasPrefix(e.Underlying, "int") ||
		strings.HasPrefix(e.Underlying, "uint")
}

func (e *Enum) IsNullable() bool {
	return e.Null != ""
}

func (e *Enum) LastIndex() int {
	return len(e.Enums) - 1
}

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

func Enums(fset *token.FileSet, pkg *ast.Package, astFile *ast.File) (map[string]*Enum, error) {
	// Find enum types
	enums := make(map[string]*Enum)
	for _, decl := range astFile.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Comment == nil {
				continue
			}
			for _, c := range typeSpec.Comment.List {
				parts := strings.Split(c.Text, ",")
				if len(parts) > 0 && parts[0] == "//#enum" {
					enums[typeSpec.Name.Name] = &Enum{
						File:       astFile.Name.Name,
						Line:       fset.Position(typeSpec.Pos()).Line,
						Package:    pkg.Name,
						Type:       typeSpec.Name.Name,
						Underlying: astvisit.ExprString(typeSpec.Type),
						JSONSchema: slices.Contains(parts, "jsonschema"),
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
			enum.LastEnumDecl = decl
			isNullValue := false
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
					isNullValue = true
					break
				}
			}
			for i, name := range valueSpec.Names {
				enum.Enums = append(enum.Enums, name.Name)
				enum.Literals = append(enum.Literals, astvisit.ExprString(valueSpec.Values[i]))
				// Only add non-null values to JSONSchemaEnum because null is another oneOf type variant
				if !isNullValue {
					if enum.Underlying == "string" || enum.Underlying == "int" {
						enum.JSONSchemaEnum = append(enum.JSONSchemaEnum, astvisit.ExprString(valueSpec.Values[i]))
					} else {
						// Value literal type does not default to underlying type
						enum.JSONSchemaEnum = append(enum.JSONSchemaEnum, fmt.Sprintf("%s(%s)", enum.Underlying, astvisit.ExprString(valueSpec.Values[i])))
					}
				}
			}
		}
	}

	for _, enum := range enums {
		if len(enum.Enums) == 0 {
			return nil, fmt.Errorf("enum type %s.%s in %s:%d has no typed const enum values", enum.Package, enum.Type, enum.File, enum.Line)
		}
	}

	// Find known enum methods
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
		if len(recv.Names) > 0 {
			enum.Recv = recv.Names[0].Name
		}
		switch funcDecl.Name.Name {
		case "Valid", "Validate", "Enums", "EnumStrings":
			enum.KnownMethods = append(enum.KnownMethods, funcDecl)
		case "String":
			if enum.IsStringType() {
				enum.KnownMethods = append(enum.KnownMethods, funcDecl)
			}
		case "IsNull", "IsNotNull", "SetNull", "MarshalJSON", "UnmarshalJSON":
			if enum.IsNullable() {
				enum.KnownMethods = append(enum.KnownMethods, funcDecl)
			}
		case "Scan", "Value":
			if enum.IsNullable() {
				enum.KnownMethods = append(enum.KnownMethods, funcDecl)
			}
		case "JSONSchema":
			if enum.JSONSchema {
				enum.KnownMethods = append(enum.KnownMethods, funcDecl)
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

	return enums, nil
}
