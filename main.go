/*
go-enum is a code generator for type-safe enums in Go.

It scans Go source files for enum type definitions marked with the //#enum comment
and automatically generates validation, conversion, and utility methods
using the Go AST and go/types.

# Usage

	go-enum [options] [path]

The path is a package directory, by default the current directory,
a directory followed by "..." like ./... for the packages in all
directories below it, or a Go file to generate only the methods
of the enums declared in that file.

# Options

	-verbose    Print information about what's happening
	-debug      Show the replaced source before every generated method
	            like a merge conflict, only with -print because it is no Go code
	-print      Print every file that would change to stdout instead of writing it
	-validate   Check for missing or outdated enum methods without modifying files.
	            Reports issues to stderr and exits with code 1 if any are found.
	            Useful for CI validation to ensure all enums have up-to-date methods.
	-help       Show help message

# Exit Codes

	0   Success (no issues found in validate mode, or generation completed successfully)
	1   Error occurred (validation failed, file not found, invalid syntax, etc.)
	2   -help was used, an option is invalid, -debug was used without -print,
	    or more than one path was given

# Enum Definition

Mark a type as an enum with the //#enum comment:

	type Status string //#enum

	const (
		StatusPending Status = "pending"
		StatusActive  Status = "active"
	)

The values are all consts of exactly that type, also with iota or conversions,
in the files of the package that are compiled in every build of the file of the
enum type. The generated methods are written to the file of the enum type.

# Markers

	//#enum             at the end of a type line marks an enum type
	//#enum,jsonschema  also generates a JSONSchema method
	//#null             at the end of a const line marks the null value
	//#exclude          at the end of a const line excludes the const from the values
	//#custom           in the doc comment of a method keeps a hand-written
	                    version of a method that would be generated

# Generated Methods

For all enums:
  - Valid() bool - Checks if value is valid
  - Validate() error - Returns error if invalid
  - ValidOr(defaultVal T) T - Returns defaultVal if invalid
  - Enums() []T - Returns all enum values
  - EnumStrings() []string - Returns all values as strings

For string enums:
  - String() string - Implements fmt.Stringer

For nullable enums (marked with //#null):
  - ValidNotNull() bool - Checks if value is valid and not null
  - ValidNotNullOr(defaultVal T) T - Returns defaultVal if invalid or null
  - IsNull() bool
  - IsNotNull() bool
  - SetNull()
  - MarshalJSON/UnmarshalJSON
  - Scan/Value for database/sql (string and integer enums)

For enums with ,jsonschema flag:
  - JSONSchema() *jsonschema.Schema

# Example

	//go:generate go-enum

	type Priority int //#enum

	const (
		PriorityNull Priority = 0 //#null
		PriorityLow  Priority = 1
		PriorityHigh Priority = 2
	)
*/
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ungerik/go-enum/enums"
)

var (
	verbose   bool
	debug     bool
	printOnly bool
	validate  bool
	printHelp bool
)

func main() {
	flag.BoolVar(&verbose, "verbose", false, "prints information to stdout of what's happening")
	flag.BoolVar(&debug, "debug", false, "inserts debug information")
	flag.BoolVar(&printOnly, "print", false, "prints to stdout instead of writing files")
	flag.BoolVar(&validate, "validate", false, "check for missing or outdated enum methods without modifying files")
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
	if debug && !printOnly && !validate {
		// The debug output is no Go code and would break the source files
		fmt.Fprintln(os.Stderr, "go-enum error: -debug needs -print, because its output is no Go code")
		os.Exit(2)
	}
	if args := flag.Args(); len(args) > 1 {
		// Options after the path are no options for the flag package
		fmt.Fprintf(os.Stderr, "go-enum error: expected at most one path argument, got %d, options must come before the path\n", len(args))
		os.Exit(2)
	} else if len(args) == 1 {
		path = args[0]
	}
	if verbose {
		verboseOut = os.Stdout
	}
	if printOnly {
		resultOut = os.Stdout
	}

	var err error
	if validate {
		err = enums.ValidateRewrite(path, verboseOut, debug)
	} else {
		err = enums.Rewrite(path, verboseOut, resultOut, debug)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-enum error:", err)
		os.Exit(1)
	}
}
