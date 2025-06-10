package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"os"

	"github.com/ungerik/go-astvisit"
	"github.com/ungerik/go-enum/find"
	"github.com/ungerik/go-enum/templates"
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
		func(fset *token.FileSet, pkg *ast.Package, astFile *ast.File, filePath string, verboseOut io.Writer) (astvisit.NodeReplacements, astvisit.Imports, error) {
			// ast.Print(fset, astFile)
			// return nil, nil

			enums, err := find.Enums(fset, pkg, astFile)
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
				err := templates.ValidateMethods.Execute(&methods, enum)
				if err != nil {
					return nil, nil, err
				}
				err = templates.EnumsMethods.Execute(&methods, enum)
				if err != nil {
					return nil, nil, err
				}
				if enum.IsStringType() {
					err = templates.StringMethods.Execute(&methods, enum)
					if err != nil {
						return nil, nil, err
					}
				}
				if enum.IsNullable() {
					imports[`"bytes"`] = struct{}{}
					imports[`"encoding/json"`] = struct{}{}
					err = templates.NullableMethods.Execute(&methods, enum)
					if err != nil {
						return nil, nil, err
					}
					switch {
					case enum.IsStringType():
						imports[`"database/sql/driver"`] = struct{}{}
						err = templates.NullableStringMethods.Execute(&methods, enum)
						if err != nil {
							return nil, nil, err
						}
					case enum.IsIntType():
						imports[`"database/sql/driver"`] = struct{}{}
						err = templates.NullableIntMethods.Execute(&methods, enum)
						if err != nil {
							return nil, nil, err
						}
					}
				}
				if enum.JSONSchema {
					imports[`"github.com/invopop/jsonschema"`] = struct{}{}
					err = templates.JSONSchemaMethod.Execute(&methods, enum)
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

			return replacements, imports, nil
		},
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-enum error:", err)
		os.Exit(1)
	}
}
