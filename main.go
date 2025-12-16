/*
go-enum is a code generator for type-safe enums in Go.

It scans Go source files for enum type definitions marked with the //#enum comment
and automatically generates validation, conversion, and utility methods using AST
manipulation provided by github.com/ungerik/go-astvisit.

# Usage

	go-enum [options] [path]

# Options

	-verbose    Print information about what's happening
	-debug      Insert debug comments in generated code
	-print      Print generated code to stdout instead of writing files
	-validate   Check for missing or outdated enum methods without modifying files.
	            Reports issues to stderr and exits with code 1 if any are found.
	            Useful for CI validation to ensure all enums have up-to-date methods.
	-help       Show help message

# Exit Codes

	0   Success (no issues found in validate mode, or generation completed successfully)
	1   Error occurred (validation failed, file not found, invalid syntax, etc.)

# Enum Definition

Mark a type as an enum with the //#enum comment:

	type Status string //#enum

	const (
		StatusPending Status = "pending"
		StatusActive  Status = "active"
	)

# Generated Methods

For all enums:
  - Valid() bool - Checks if value is valid
  - Validate() error - Returns error if invalid
  - Enums() []T - Returns all enum values
  - EnumStrings() []string - Returns all values as strings

For string enums:
  - String() string - Implements fmt.Stringer

For nullable enums (marked with //#null):
  - IsNull() bool
  - IsNotNull() bool
  - SetNull()
  - MarshalJSON/UnmarshalJSON
  - Scan/Value for database/sql

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
	if args := flag.Args(); len(args) > 0 {
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
