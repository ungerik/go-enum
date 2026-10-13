# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v1.2.0] - 2026-10-13

### Added

- `//#exclude` line comment marker for consts of an enum type that are not
  enum values, like sentinels and aliases. Excluded consts are not used in the
  generated methods and are skipped by the duplicate checks.
- Enum values and methods are read from all files of the package that are
  compiled in every build of the file declaring the enum type instead of only
  from that file. Like with the go tool, files ending in `_test.go` or
  starting with `_` or `.` are ignored, and so are files with a `//go:build`
  line, a file name suffix like `_linux.go` or an `import "C"` that are not
  compiled in every build of the file of the enum type, like the go tool sets
  build tags for one GOOS, GOARCH, Go version and compiler, see the README
  section "Values in Other Files". Files with `//go:build ignore` are not part
  of the package. An invalid build constraint is an error.
- Methods marked with `//#custom` in other files of the package, also in
  files with build constraints like one per platform, are not generated
  again, and must be declared in every build of the file of the enum type.
  Unmarked methods in other files that would be generated are an error,
  because only the file declaring the enum type is rewritten.
- Untyped consts converted to the enum type from an imported value, like
  `MethodGet = Method(http.MethodGet)`, are enum values.
- Enum values are evaluated with `go/types`: consts declared with `iota` and
  implicit repetition, like `KindB` after `KindA Kind = iota`, conversions
  like `FlagD = Flag(0x10)` and other constant expressions are enum values,
  and `EnumStrings()` returns their values, like `"16"` instead of `"0x10"`.
- Enums of `main` packages are generated.
- GitHub Actions workflow that builds, vets and tests on pushes and pull
  requests to `main`.

### Changed

- **Breaking** for code that imports the package `enums`: `Find` takes the
  parsed files of a package and returns its enums as a slice sorted by file
  name and position instead of a map by type name, the `Enum` fields `Enums`,
  `Literals` and `JSONSchemaEnum` were replaced by `Values`, the fields
  `LastEnumDecl` and `KnownMethods` were removed, and `Enum.File` is the path
  of the file declaring the enum type instead of the package name.
  The command line is compatible.
- The methods of all enums of a package are generated from one model of the
  package that is parsed and type checked once per run, instead of scanning
  the package again for every file. Runs with up-to-date methods don't format
  any file. The files of a package are only written after the methods of all
  its enums were generated, and all errors of a package are reported.
- Typed consts of an enum type declared in other files of the package now
  become enum values when the methods are regenerated: `Valid()` accepts them
  and `Enums()`, `EnumStrings()` and `JSONSchema()` list them. Consts with the
  same value as another enum value now fail generation. Mark consts that
  should not be enum values with `//#exclude`.
- Aliases initialized with another enum const, like
  `StatusDefault Status = StatusActive`, are not enum values. In the file of
  the enum type they generated duplicate switch cases before.
- Duplicate enum values are detected by their evaluated value,
  so `1`, `0x1` and `1 << 0` are duplicates.
- Values that use an import, like `http.MethodGet`, are referenced by their
  const name in the generated methods, like `string(MethodGet)` in
  `EnumStrings()`, instead of copying the expression, so they can be declared
  in any file of the package. This changes the generated `EnumStrings()` and
  `JSONSchema()` of such enums once.
- Existing methods are only replaced if they were generated before, recognized
  by the whole doc comment of the generated method, with or without a period
  at the end of its lines, and for `UnmarshalJSON`, whose doc comment is a
  common phrase, also by its code. A method the generator would produce in
  the file of the enum type without that doc comment and without `//#custom`
  is an error instead of being silently replaced. A method that is never
  generated for the underlying type of the enum, like `String` of an integer
  enum, is kept like before. If it also has the code of a generated method,
  like after changing the underlying type from string to int, it is an error.
- Methods that are no longer generated, for example after removing `//#null`
  or `,jsonschema`, are removed if they are unchanged, together with imports
  they used. A changed method that is no longer generated is an error.
- An aliased import like `f "fmt"` is not taken as import of the package
  name used by the generated methods, which added no import before and
  generated methods that did not compile.
- Files are written to a temporary file that is synced to disk and replaces
  the file, so an interrupted run or a crash can't leave a truncated source
  file. Read-only files are not rewritten, like before. The owner and group
  of a rewritten file are kept as far as the process is permitted to change
  them. A rewritten file that was hard linked is no longer linked to its
  other paths.
- Marker comments at other places than the end of a type or const line or
  the doc comment of a method, like `) //#null`, are errors instead of being
  ignored.
- Declarations of the package, also in files with build constraints, or
  imports of the file of the enum type with the name of a package used by
  the generated methods that are not `//#custom`, like an enum value named
  `fmt` or an import of `github.com/goccy/go-json` without import name that
  is used as `json`, are errors instead of generating code that does not
  compile.
- Imports are only added for the generated methods that use them, so a
  `//#custom` method no longer leaves an unused import like `bytes`.
- The kind of the underlying type is determined with `go/types`, so string and
  integer types of the package, like `type Status MyString`, get the methods of
  string and integer enums. Imported underlying types like `time.Duration`
  and complex numbers, which can't be marshaled as JSON, are an error.
- Blank consts of an enum type like `_ Status = "reserved"` are ignored
  instead of generating code that does not compile.
- Comments that use a reserved marker word (`enum`, `null`, `exclude`,
  `custom`) after `#` but not exactly as marker are errors instead of being
  ignored: on types anything other than `//#enum` with optional flags at the
  end of the type line, on enum consts anything other than exactly `//#null`
  or `//#exclude` at the end of the const line, and in the doc comment of a
  method that would be generated anything other than `//#custom` (or
  `// #custom` as written by gofmt). Other words, like `//#nosec G101` of
  other tools or a misspelled `//#exlude`, and unknown `//#enum` flags
  are ignored. A marker at a var or at a const that is not of an enum type,
  like `StatusNone = "" //#null` without type, is an error.
- The `//#null` const must be declared in the file of the enum type,
  because it changes which methods are generated in that file.
- Error messages show the file name instead of the package name, and duplicate
  errors point to the duplicate const and to its first declaration.
- The methods of enums inserted after the same declaration are generated
  together in declaration order.
- Recursive mode skips directories starting with `_`, `testdata` and
  `vendor` directories and nested modules like `./...` of the go tool,
  so vendored packages are never rewritten.
- A file path argument that is not a Go file of its package, like a
  `_test.go` file, is an error instead of silently doing nothing.
- No longer depends on github.com/ungerik/go-astvisit. Imports of a rewritten
  file are only grouped like goimports does when generated methods add an
  import, else the import groups of the file are kept as formatted by gofmt.
- Requires Go 1.26. Dependencies updated: testify v1.12.1,
  golang.org/x/tools v0.52.0, golang.org/x/mod v0.42.0,
  golang.org/x/sync v0.24.0.

### Fixed

- Methods inserted after a declaration with a trailing comment, like
  `const StatusNone Status = "" //#null` or `) // end of values`, no longer
  move the comment behind the methods, which made the next run lose the
  `//#null` marker. A block comment that starts on the last line of the
  declaration is also kept in front of the methods.
- Panic when the methods of several enums were inserted after the same
  declaration, for example for two enums with values in one `const` block.
- Panic for an enum const without a value, which is now an error.
- Generated methods that did not compile or were wrong because of the
  receiver or parameter names: `j` for a type like `JobState` collided with
  the parameter of `UnmarshalJSON`, a parameter like `value` of `Scan`
  collided with an enum type, its underlying type or an enum value of that
  name, a blank receiver `_` of an existing method was used, and a receiver
  of an existing method named like an enum const or a package used by the
  generated methods, like `x` or `fmt`, was used, which made `Valid()`
  accept every value or not compile.
- Hand-written `Scan` and `Value` methods of nullable enums that are neither
  string nor integer based were removed.
- Generating methods in a file with CRLF line endings converted the whole
  file to LF line endings. The CRLF line endings of the file are kept,
  a file with only some CRLF line endings gets LF line endings like with gofmt.

## [v1.1.0] - 2026-04-19

### Added

- `//#custom` marker in the doc comment of a method that the generator would
  produce, to keep a hand-written version of that method.

## [v1.0.0] - 2026-04-16

Initial tagged release.

[v1.2.0]: https://github.com/ungerik/go-enum/compare/v1.1.0...v1.2.0
[v1.1.0]: https://github.com/ungerik/go-enum/compare/v1.0.0...v1.1.0
[v1.0.0]: https://github.com/ungerik/go-enum/releases/tag/v1.0.0
