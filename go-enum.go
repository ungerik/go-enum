package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"os"

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
	typeSpec *ast.TypeSpec
	nullName string
	names    []string
	values   []ast.Expr
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
					enums[typeSpec.Name.Name] = &enumSpec{typeSpec: typeSpec}
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
		ast.Print(fset, genDecl)

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
				enum.names = append(enum.names, valueSpec.Names[i].Name)
				enum.values = append(enum.values, valueSpec.Values[i])
			}
			if valueSpec.Comment != nil {
				for _, c := range valueSpec.Comment.List {
					if c.Text != "//#null" {
						continue
					}
					if enum.nullName != "" {
						return nil, fmt.Errorf("second //#null enum encountered %s", valueSpec.Names[0].Name)
					}
					if len(valueSpec.Names) > 1 {
						return nil, fmt.Errorf("cant use //#null for multiple enums: %#v", valueSpec.Names)
					}
					enum.nullName = valueSpec.Names[0].Name
					break
				}
			}
		}

		return nil, nil
	}

	return nil, nil
}
