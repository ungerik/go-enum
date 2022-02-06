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

func rewriteFile(fset *token.FileSet, pkg *ast.Package, astFile *ast.File, filePath string, verboseOut io.Writer) ([]byte, error) {
	// ast.Print(fset, astFile)
	// return nil, nil

	var enumTypeSpecs []*ast.TypeSpec
	for _, decl := range astFile.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			for _, c := range typeSpec.Comment.List {
				if c.Text == "//#enum" {
					enumTypeSpecs = append(enumTypeSpecs, typeSpec)
					break
				}
			}
		}
	}

	if len(enumTypeSpecs) == 0 {
		return nil, nil
	}

	for _, decl := range astFile.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		ast.Print(fset, genDecl)
		return nil, nil
	}

	return nil, nil
}
