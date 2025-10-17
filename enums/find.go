package enums

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"strings"

	"github.com/ungerik/go-astvisit"
)

// Find scans a Go AST file for enum type definitions and extracts their metadata.
//
// It looks for types marked with the //#enum comment, collects all const values
// of that type, identifies nullable values marked with //#null, and finds existing
// methods that should be replaced.
//
// Returns a map of enum type name to Enum metadata, or an error if the enum
// definitions are invalid.
func Find(fset *token.FileSet, pkg *ast.Package, astFile *ast.File) (map[string]*Enum, error) {
	// Validate package name
	if pkg == nil || pkg.Name == "" {
		return nil, fmt.Errorf("invalid or missing package name in %s", astFile.Name.Name)
	}

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
				for i, part := range parts {
					parts[i] = strings.TrimSpace(part)
				}
				if len(parts) > 0 && parts[0] == "//#enum" {
					typeName := typeSpec.Name.Name
					if typeName == "" {
						return nil, fmt.Errorf("enum type has empty name in %s:%d", astFile.Name.Name, fset.Position(typeSpec.Pos()).Line)
					}
					enums[typeName] = &Enum{
						File:       astFile.Name.Name,
						Line:       fset.Position(typeSpec.Pos()).Line,
						Package:    pkg.Name,
						Type:       typeName,
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

		// Check for duplicate enum names
		seenNames := make(map[string]string) // name -> first literal value
		for i, name := range enum.Enums {
			if firstLiteral, exists := seenNames[name]; exists {
				return nil, fmt.Errorf("duplicate enum name %s for type %s.%s in %s:%d (values: %s and %s)",
					name, enum.Package, enum.Type, enum.File, enum.Line, firstLiteral, enum.Literals[i])
			}
			seenNames[name] = enum.Literals[i]
		}

		// Check for duplicate literal values
		seenLiterals := make(map[string]string) // literal -> first name
		for i, literal := range enum.Literals {
			if firstName, exists := seenLiterals[literal]; exists {
				return nil, fmt.Errorf("duplicate enum value %s for type %s.%s in %s:%d (used by both %s and %s)",
					literal, enum.Package, enum.Type, enum.File, enum.Line, firstName, enum.Enums[i])
			}
			seenLiterals[literal] = enum.Enums[i]
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
			if len(enum.Type) == 0 {
				// Should never happen due to earlier validation, but be defensive
				return nil, fmt.Errorf("enum type %s.%s has empty name", enum.Package, enum.Type)
			}
			enum.Recv = strings.ToLower(enum.Type[:1])
		}
	}

	return enums, nil
}
