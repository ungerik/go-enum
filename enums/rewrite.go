package enums

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"os"

	"github.com/ungerik/go-astvisit"
)

// Rewrite scans Go source files at the given path for enum type definitions
// and generates or updates type-safe methods for each enum.
//
// It finds all types marked with //#enum, generates validation methods
// (Valid, Validate), utility methods (Enums, EnumStrings), and optional
// methods based on flags (String, IsNull, MarshalJSON, Scan, Value, JSONSchema).
//
// The generated methods either replace existing enum methods or are inserted
// after the last enum constant declaration.
//
// Parameters:
//   - path: Directory or file path to process (defaults to current directory)
//   - verboseOut: Writer for verbose progress output (nil to disable)
//   - resultOut: Writer for generated code output (nil to write to files)
//   - debug: If true, inserts debug comments in generated code
func Rewrite(path string, verboseOut io.Writer, resultOut io.Writer, debug bool) error {
	return rewrite(path, verboseOut, resultOut, debug, false)
}

// ValidateRewrite checks if enum methods are missing or outdated without modifying files.
//
// It scans Go source files at the given path for enum type definitions and checks
// if the generated methods would differ from the current file contents.
// Any missing or outdated methods are reported to stderr.
//
// Parameters:
//   - path: Directory or file path to process (defaults to current directory)
//   - verboseOut: Writer for verbose progress output (nil to disable)
//   - debug: If true, inserts debug comments in generated code
//
// Returns an error if any enum methods are missing or outdated.
func ValidateRewrite(path string, verboseOut io.Writer, debug bool) error {
	return rewrite(path, verboseOut, nil, debug, true)
}

func rewrite(path string, verboseOut io.Writer, resultOut io.Writer, debug bool, validate bool) error {
	var validationErrors []string

	err := astvisit.RewriteWithReplacements(
		path,
		verboseOut,
		resultOut,
		debug,
		func(fset *token.FileSet, pkg *ast.Package, astFile *ast.File, filePath string, verboseOut io.Writer) (astvisit.NodeReplacements, astvisit.Imports, error) {
			// ast.Print(fset, astFile)
			// return nil, nil

			enums, err := Find(fset, pkg, astFile)
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
				err := validateMethodsTemplate.Execute(&methods, enum)
				if err != nil {
					return nil, nil, err
				}
				err = enumsMethodsTemplate.Execute(&methods, enum)
				if err != nil {
					return nil, nil, err
				}
				if enum.IsStringType() {
					err = stringMethodsTemplate.Execute(&methods, enum)
					if err != nil {
						return nil, nil, err
					}
				}
				if enum.IsNullable() {
					imports[`"bytes"`] = struct{}{}
					imports[`"encoding/json"`] = struct{}{}
					err = nullableMethodsTemplate.Execute(&methods, enum)
					if err != nil {
						return nil, nil, err
					}
					switch {
					case enum.IsStringType():
						imports[`"database/sql/driver"`] = struct{}{}
						err = nullableStringMethodsTemplate.Execute(&methods, enum)
						if err != nil {
							return nil, nil, err
						}
					case enum.IsIntType():
						imports[`"database/sql/driver"`] = struct{}{}
						err = nullableIntMethodsTemplate.Execute(&methods, enum)
						if err != nil {
							return nil, nil, err
						}
					}
				}
				if enum.JSONSchema {
					imports[`"github.com/invopop/jsonschema"`] = struct{}{}
					err = jsonSchemaMethodTemplate.Execute(&methods, enum)
					if err != nil {
						return nil, nil, err
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

			// In validation mode, check if replacements would actually change the file
			if validate && len(replacements) > 0 {
				// Read original source
				source, err := os.ReadFile(filePath)
				if err != nil {
					return nil, nil, err
				}

				// Apply replacements to see if file would change
				rewritten, err := replacements.Apply(fset, source)
				if err != nil {
					return nil, nil, err
				}

				// Apply formatting with imports
				rewritten, err = astvisit.FormatFileWithImports(fset, rewritten, imports)
				if err != nil {
					return nil, nil, err
				}

				// Check if the content actually changed
				if !bytes.Equal(source, rewritten) {
					// File would change - report as validation error
					for _, repl := range replacements {
						var pos token.Position
						if repl.Node != nil {
							pos = fset.Position(repl.Node.Pos())
						}
						desc := repl.DebugID
						if desc == "" {
							desc = "enum methods"
						}
						validationErrors = append(validationErrors, fmt.Sprintf("%s: missing or outdated %s", pos, desc))
					}
				}

				// Return nil to prevent file modification in validate mode
				return nil, nil, nil
			}

			return replacements, imports, nil
		},
	)
	if err != nil {
		return err
	}

	// In validation mode, report errors and fail if any were found
	if validate && len(validationErrors) > 0 {
		for _, errMsg := range validationErrors {
			fmt.Fprintln(os.Stderr, errMsg)
		}
		return fmt.Errorf("found %d missing or outdated enum method(s)", len(validationErrors))
	}

	return nil
}
