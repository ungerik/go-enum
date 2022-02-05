package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"

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
		args            = flag.Args()
		cwd, _          = os.Getwd()
		filePath        string
		verboseWriter   io.Writer
		printOnlyWriter io.Writer
	)
	if len(args) == 0 {
		filePath = cwd
	} else {
		recursive := strings.HasSuffix(args[0], "...")
		if args[0] == "." || args[0] == "./..." {
			filePath = cwd
		} else {
			filePath = filepath.Clean(strings.TrimSuffix(args[0], "..."))
		}
		if recursive {
			filePath = filepath.Join(filePath, "...")
		}
	}
	if verbose {
		verboseWriter = os.Stdout
	}
	if printOnly {
		printOnlyWriter = os.Stdout
	}

	err = astvisit.Rewrite(filePath, verboseWriter, printOnlyWriter, rewriteAstFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-enum error:", err)
		os.Exit(2)
	}
}

func rewriteAstFile(fset *token.FileSet, filePkg *ast.Package, astFile *ast.File, filePath string, verboseWriter, printOnly io.Writer) error {
	panic("todo")
}
