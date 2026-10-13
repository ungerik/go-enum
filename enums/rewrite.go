package enums

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/imports"
)

// Rewrite scans Go packages at the given path for enum type definitions
// and generates or updates type-safe methods for each enum.
//
// It finds all types marked with //#enum, generates validation methods
// (Valid, Validate), utility methods (Enums, EnumStrings), and optional
// methods based on flags (String, IsNull, MarshalJSON, Scan, Value, JSONSchema).
//
// The generated methods are written to the file declaring the enum type.
// They replace methods generated before, or are inserted after the last
// enum constant declaration in that file, or after the type declaration
// if that file has no enum constants. The files of a package are only
// written after the methods of all its enums were generated.
//
// Parameters:
//   - path: Directory or file path to process (defaults to current directory),
//     append "..." to process all package directories below a directory
//   - verboseOut: Writer for verbose progress output (nil to disable)
//   - resultOut: Writer for generated code output (nil to write to files)
//   - debug: If true, inserts debug comments in generated code
func Rewrite(path string, verboseOut io.Writer, resultOut io.Writer, debug bool) error {
	return rewrite(path, verboseOut, resultOut, debug, false)
}

// ValidateRewrite checks if enum methods are missing or outdated without modifying files.
//
// It scans Go packages at the given path for enum type definitions and checks
// if the generated methods would differ from the current file contents.
// Any missing or outdated methods are reported to stderr.
//
// Parameters:
//   - path: Directory or file path to process (defaults to current directory),
//     append "..." to process all package directories below a directory
//   - verboseOut: Writer for verbose progress output (nil to disable)
//   - debug: If true, inserts debug comments in generated code
//
// Returns an error if any enum methods are missing or outdated.
func ValidateRewrite(path string, verboseOut io.Writer, debug bool) error {
	return rewrite(path, verboseOut, nil, debug, true)
}

func rewrite(path string, verboseOut io.Writer, resultOut io.Writer, debug bool, validate bool) error {
	dirs, onlyFile, recursive, err := packageDirs(path)
	if err != nil {
		return err
	}
	var (
		outdated []string
		errs     []error // of all packages, so that one run reports all of them
		packages int     // directories with Go files
	)
	for _, dir := range dirs {
		fset := token.NewFileSet()
		files, sources, crlfFiles, err := parsePackageDir(fset, dir)
		if err != nil {
			errs = append(errs, err)
			packages++ // with a Go file that doesn't parse
			continue
		}
		if len(files) == 0 {
			if !recursive {
				errs = append(errs, fmt.Errorf("no Go files in %s", dir))
			}
			continue
		}
		packages++
		if !slices.ContainsFunc(slices.Collect(maps.Keys(files)), func(filePath string) bool { return !isTestFile(filePath) }) {
			// Like a test-only module, which has no enums
			if !recursive {
				errs = append(errs, fmt.Errorf("no non-test Go files in %s", dir))
			}
			continue
		}
		if onlyFile != "" {
			onlyFile, err = packageFile(onlyFile, files)
			if err != nil {
				errs = append(errs, err)
				continue
			}
		}
		fprintf(verboseOut, "parsing package: %s\n", dir)
		enums, err := Find(fset, files)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// Generate the methods of all enums before writing any file
		var (
			changedFiles []string
			results      = make(map[string][]byte)
			failed       bool
		)
		enumsByFile := make(map[string][]*Enum)
		for _, enum := range enums {
			enumsByFile[enum.File] = append(enumsByFile[enum.File], enum)
		}
		for _, filePath := range slices.Sorted(maps.Keys(enumsByFile)) {
			fileEnums := enumsByFile[filePath]
			if onlyFile != "" && filePath != onlyFile {
				continue
			}
			result, outdatedEnums, err := rewriteFile(fset, files[filePath], sources[filePath], fileEnums, debug)
			if err != nil {
				errs = append(errs, fmt.Errorf("generating methods in %s: %w", filePath, err))
				failed = true
				break
			}
			for _, enum := range outdatedEnums {
				outdated = append(outdated, fmt.Sprintf("%s:%d: missing or outdated methods of enum type %s", enum.File, enum.Line, enum.Type))
			}
			if result == nil {
				fprintf(verboseOut, "no changes in file: %s\n", filePath)
				continue
			}
			if crlfFiles[filePath] {
				result = bytes.ReplaceAll(result, []byte("\n"), []byte("\r\n"))
			}
			changedFiles = append(changedFiles, filePath)
			results[filePath] = result
		}
		if validate || failed {
			continue
		}
		for _, filePath := range changedFiles {
			fprintf(verboseOut, "changed file: %s\n", filePath)
			if resultOut != nil {
				if _, err := resultOut.Write(results[filePath]); err != nil {
					// With the errors of the packages before
					return errors.Join(append(errs, err)...)
				}
				continue
			}
			if err := writeFile(filePath, results[filePath]); err != nil {
				errs = append(errs, err)
				break
			}
		}
	}
	if recursive && packages == 0 {
		// Like a mistyped path, which must not pass -validate silently
		errs = append(errs, fmt.Errorf("no Go files in %s or below", dirs[0]))
	}

	// In validation mode, report the outdated methods and fail if any were found
	if validate && len(outdated) > 0 {
		for _, msg := range outdated {
			fmt.Fprintln(os.Stderr, msg)
		}
		errs = append(errs, fmt.Errorf("found %d enum type(s) with missing or outdated methods", len(outdated)))
	}
	return errors.Join(errs...)
}

// packageDirs returns the package directories to rewrite for path,
// which is a file, a directory, or a directory followed by "..."
// for all package directories below it, and the file if path is one.
func packageDirs(path string) (dirs []string, file string, recursive bool, err error) {
	path, recursive = strings.CutSuffix(path, "...")
	path, err = filepath.Abs(cmp.Or(path, "."))
	if err != nil {
		return nil, "", false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", false, err
	}
	if !info.IsDir() {
		return []string{filepath.Dir(path)}, path, false, nil
	}
	if !recursive {
		return []string{path}, "", false, nil
	}
	// WalkDir doesn't follow a symlinked root, like a symlinked checkout
	if path, err = filepath.EvalSymlinks(path); err != nil {
		return nil, "", false, err
	}
	err = filepath.WalkDir(path, func(dir string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		// Like ./... of the go tool, skip directories starting with . or _,
		// testdata, vendor and nested modules, and node_modules like go-astvisit did
		name := entry.Name()
		if dir != path && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" || name == "node_modules") {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); dir != path && err == nil {
			return filepath.SkipDir
		}
		dirs = append(dirs, dir)
		return nil
	})
	return dirs, "", true, err
}

// writeFile writes data to a temporary file in the directory of filePath
// and renames it to filePath, so that an interrupted write can't leave
// a truncated source file. The file mode of filePath is kept. Without
// permission to create a file in the directory, or if the temporary file
// can't get the group of filePath or has no permission to replace it, like
// a bind mounted file, filePath is written in place without that guarantee.
func writeFile(filePath string, data []byte) error {
	if resolved, err := filepath.EvalSymlinks(filePath); err == nil {
		filePath = resolved
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	// Like os.WriteFile, fail for a read-only file instead of replacing it
	f, err := os.OpenFile(filePath, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(filePath), "."+filepath.Base(filePath)+".*.tmp")
	if errors.Is(err, fs.ErrPermission) {
		// Without permission to create a file in the directory
		return writeInPlace(filePath, data)
	}
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // Fails after the successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	// Written to disk before the rename, so that a crash can't
	// replace the file with an empty one
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	// Like writing the file itself, keep its owner and group if possible,
	// and its mode, set through the open file to not follow a replaced path
	if !chownLike(tmp, info) {
		// The group permissions of the mode would apply to another group
		tmp.Close()
		return writeInPlace(filePath, data)
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), filePath); errors.Is(err, fs.ErrPermission) || isNotReplaceable(err) {
		// Like the file of another user in a sticky directory,
		// or a bind mounted file, which can't be replaced
		return writeInPlace(filePath, data)
	} else if err != nil {
		// Like a full disk, where writing in place could truncate the file
		return err
	}
	return nil
}

// neverBuilt reports whether a file is compiled in no build, like a
// template for go run with //go:build ignore, by its build constraints.
func neverBuilt(fset *token.FileSet, filePath string, source []byte) bool {
	header, err := parser.ParseFile(fset, filePath, source, parser.PackageClauseOnly|parser.ParseComments)
	if err != nil {
		return false
	}
	c, err := buildConstraint(fset, filePath, header)
	return err == nil && !compatible(c, c)
}

// writeInPlace writes data to the existing file filePath like os.WriteFile,
// but without O_CREATE, which Linux refuses with fs.protected_regular
// for the file of another user in a sticky directory.
func writeInPlace(filePath string, data []byte) error {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// packageFile returns the path of the parsed file that is the same file
// as path, which can differ in case on case-insensitive file systems,
// or an error if path is no Go file of the package.
func packageFile(path string, files map[string]*ast.File) (string, error) {
	pathInfo, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	for filePath := range files {
		if isTestFile(filePath) {
			continue
		}
		if info, err := os.Stat(filePath); err == nil && os.SameFile(info, pathInfo) {
			return filePath, nil
		}
	}
	return "", fmt.Errorf("%s is not a Go file of the package in %s", path, filepath.Dir(path))
}

// parsePackageDir parses the Go files of a directory, except the files
// starting with _ or . that the go tool ignores. Test files are parsed if
// another file has an enum marker, or if there are only test files, and
// skipped if they don't parse, because they are only read to check the
// enums for collisions in test builds.
//
// The sources are parsed with LF line endings, so that they can be compared
// with generated methods. Files with CRLF line endings, like in a Windows
// checkout, are returned in crlfFiles to write them with CRLF line endings.
func parsePackageDir(fset *token.FileSet, dir string) (files map[string]*ast.File, sources map[string][]byte, crlfFiles map[string]bool, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, nil, err
	}
	files = make(map[string]*ast.File)
	sources = make(map[string][]byte)
	crlfFiles = make(map[string]bool)
	var testFiles []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || isIgnoredFile(entry.Name()) {
			continue
		}
		filePath := filepath.Join(dir, entry.Name())
		if isTestFile(filePath) {
			testFiles = append(testFiles, filePath)
			continue
		}
		source, err := os.ReadFile(filePath) //#nosec G304 -- file of the package to rewrite
		if err != nil {
			return nil, nil, nil, err
		}
		if crlf := bytes.Count(source, []byte("\r\n")); crlf > 0 {
			// Like gofmt, a file with some CRLF line endings gets LF line endings
			crlfFiles[filePath] = crlf == bytes.Count(source, []byte("\n"))
			source = bytes.ReplaceAll(source, []byte("\r\n"), []byte("\n"))
		}
		// With object resolution for usedPackageNames
		file, err := parser.ParseFile(fset, filePath, source, parser.ParseComments)
		if err != nil {
			if !bytes.Contains(source, []byte("#enum")) && neverBuilt(fset, filePath, source) {
				// Like go build, which doesn't parse a file it never compiles,
				// unless it has enums to generate methods for
				continue
			}
			return nil, nil, nil, err
		}
		files[filePath] = file
		sources[filePath] = source
	}
	if len(sources) > 0 && !slices.ContainsFunc(slices.Collect(maps.Values(sources)), func(source []byte) bool {
		return bytes.Contains(source, []byte("#enum"))
	}) {
		return files, sources, crlfFiles, nil
	}
	// Without other files, the test files have no enums to check,
	// so they don't need to parse beyond their package clause
	mode := parser.ParseComments
	if len(sources) == 0 {
		mode = parser.PackageClauseOnly
	}
	for _, filePath := range testFiles {
		// Only rewritten files need their source
		source, err := os.ReadFile(filePath) //#nosec G304 -- test file of the package to rewrite
		if err != nil {
			return nil, nil, nil, err
		}
		file, err := parser.ParseFile(fset, filePath, source, mode)
		if err != nil {
			// Like a test file being edited, which doesn't stop generating the methods
			continue
		}
		files[filePath] = file
	}
	return files, sources, crlfFiles, nil
}

// rewriteFile returns the source of a file with the generated methods
// of the enums declared in it, or nil if all methods are up to date,
// and the enums with missing or outdated methods.
func rewriteFile(fset *token.FileSet, file *ast.File, source []byte, enums []*Enum, debug bool) (result []byte, outdated []*Enum, err error) {
	var (
		edits   []edit
		imports []string
	)
	for _, enum := range enums {
		methods, methodImports, err := enum.generateMethods()
		if err != nil {
			return nil, nil, err
		}
		imports = append(imports, methodImports...)
		enumEdits := enum.edits(fset, file, source, methods)
		if len(enumEdits) == 0 && hasImports(file, methodImports) {
			continue
		}
		outdated = append(outdated, enum)
		edits = append(edits, enumEdits...)
	}
	if len(outdated) == 0 {
		return nil, nil, nil
	}
	result = applyEdits(fset.Position(file.Pos()).Filename, source, edits, debug)
	if debug {
		return result, outdated, nil
	}
	result, err = formatWithImports(result, imports)
	if err != nil {
		return nil, nil, err
	}
	if bytes.Equal(result, source) {
		return nil, outdated, nil
	}
	return result, outdated, nil
}

// generateMethods returns the gofmt formatted methods generated for the enum,
// except the ones marked with //#custom, and the import paths they use.
func (e *Enum) generateMethods() (methods string, imports []string, err error) {
	var source bytes.Buffer
	source.WriteString("package p\n")
	for _, m := range generatedMethods {
		if !m.generate(e) || e.CustomMethods[m.name] {
			continue
		}
		if err := m.tmpl.Execute(&source, e); err != nil {
			return "", nil, err
		}
	}
	fset := token.NewFileSet()
	// With object resolution for usedPackageNames
	file, err := parser.ParseFile(fset, "", source.Bytes(), parser.ParseComments)
	if err != nil {
		return "", nil, fmt.Errorf("can't parse generated methods of enum type %s.%s in %s:%d: %w", e.Package, e.Type, e.File, e.Line, err)
	}
	for name := range usedPackageNames(file) {
		if importPath, ok := generatorImports[name]; ok {
			imports = append(imports, importPath)
		}
	}
	slices.Sort(imports)
	var formatted bytes.Buffer
	if err := format.Node(&formatted, fset, file); err != nil {
		return "", nil, err
	}
	_, methods, _ = strings.Cut(formatted.String(), "\n") // without the package clause
	return strings.TrimSpace(methods), imports, nil
}

// edits returns the edits of source to replace the methods generated before
// with the gofmt formatted methods, or to insert them if there were none.
// No edits are returned if the generated methods are up to date.
func (e *Enum) edits(fset *token.FileSet, file *ast.File, source []byte, methods string) []edit {
	offset := func(pos token.Pos) int { return fset.Position(pos).Offset }
	id := "Replacement for " + e.Type
	if len(e.generated) == 0 {
		if methods == "" {
			return nil
		}
		// Insert after the line of the declaration to keep its
		// line comment on that line, or after a block comment
		// that starts on that line and ends on a following line
		end := e.insertAfter.End()
		for _, group := range file.Comments {
			if group.Pos() >= end && fset.PositionFor(group.Pos(), false).Line == fset.PositionFor(end, false).Line {
				end = group.End()
			}
		}
		pos := offset(end)
		text := "\n" + methods + "\n"
		if i := bytes.IndexByte(source[pos:], '\n'); i >= 0 {
			pos += i + 1
		} else {
			pos = len(source)
			text = "\n" + text
		}
		return []edit{{start: pos, end: pos, text: text, id: id}}
	}
	first, last := e.generated[0], e.generated[len(e.generated)-1]
	if string(source[offset(first.Doc.Pos()):offset(last.End())]) == methods {
		return nil
	}
	// Replace the first generated method with all methods
	// and remove the others with their line
	edits := make([]edit, len(e.generated))
	for i, fn := range e.generated {
		edits[i] = edit{start: offset(fn.Doc.Pos()), end: offset(fn.End()), id: id}
		if i == 0 && methods != "" {
			edits[i].text = methods
		} else if edits[i].end < len(source) && source[edits[i].end] == '\n' {
			edits[i].end++
		}
	}
	return edits
}

// edit replaces source[start:end] with text
type edit struct {
	start, end int
	text       string
	id         string // shown in debug mode
}

// applyEdits returns source with the edits applied.
// In debug mode the replaced source of every edit is shown
// before its replacement text like a merge conflict.
func applyEdits(filename string, source []byte, edits []edit, debug bool) []byte {
	slices.SortStableFunc(edits, func(a, b edit) int {
		return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(a.end, b.end))
	})
	var result bytes.Buffer
	pos := 0
	for _, e := range edits {
		result.Write(source[pos:e.start])
		if debug {
			fmt.Fprintf(&result, "<<<<<<< %s\n%s\n=======", filename, source[e.start:e.end])
		}
		result.WriteString(e.text)
		if debug {
			fmt.Fprintf(&result, "\n>>>>>>> %s\n", e.id)
		}
		pos = e.end
	}
	result.Write(source[pos:])
	return result.Bytes()
}

// generatorImports are the import paths of the
// packages used by generated methods by package name
var generatorImports = map[string]string{
	"bytes":      "bytes",
	"driver":     "database/sql/driver",
	"fmt":        "fmt",
	"json":       "encoding/json",
	"jsonschema": "github.com/invopop/jsonschema",
}

// formatWithImports formats source with gofmt, removes the imports of
// packages used by generated methods that are not used anymore, for example
// because the methods of a nullable enum were removed or replaced by //#custom
// methods, and adds the missing imports of importPaths. Only if imports were
// added, the imports are grouped like goimports does, else the import groups
// of the file are kept.
func formatWithImports(source []byte, importPaths []string) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", source, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	used := usedPackageNames(file)
	for _, imp := range slices.Clone(file.Imports) {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		name, localName := path.Base(importPath), ""
		if imp.Name != nil {
			name, localName = imp.Name.Name, imp.Name.Name
		}
		if generatorImports[name] == importPath && !used[name] {
			astutil.DeleteNamedImport(fset, file, localName, importPath)
		}
	}
	added := false
	for _, importPath := range importPaths {
		if !hasImports(file, []string{importPath}) {
			added = astutil.AddImport(fset, file, importPath) || added
		}
	}
	var result bytes.Buffer
	if err := format.Node(&result, fset, file); err != nil {
		return nil, err
	}
	if !added {
		return result.Bytes(), nil
	}
	return imports.Process("", result.Bytes(), &imports.Options{Comments: true, FormatOnly: true})
}

// usedPackageNames returns the identifiers used as package name
// of a qualified identifier like fmt in fmt.Errorf. The file must be
// parsed with object resolution, without parser.SkipObjectResolution.
func usedPackageNames(file *ast.File) map[string]bool {
	names := make(map[string]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		if sel, ok := node.(*ast.SelectorExpr); ok {
			// A resolved identifier is a declaration of the file
			// like a local variable, not an imported package
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Obj == nil {
				names[ident.Name] = true
			}
		}
		return true
	})
	return names
}

// hasImports reports whether file imports all import paths.
func hasImports(file *ast.File, importPaths []string) bool {
	for _, importPath := range importPaths {
		if !slices.ContainsFunc(file.Imports, func(imp *ast.ImportSpec) bool {
			// An aliased import like `f "fmt"` doesn't declare
			// the package name fmt used by the generated methods
			return imp.Path.Value == strconv.Quote(importPath) && (imp.Name == nil || generatorImports[imp.Name.Name] == importPath)
		}) {
			return false
		}
	}
	return true
}

func fprintf(w io.Writer, format string, args ...any) {
	if w != nil {
		fmt.Fprintf(w, format, args...)
	}
}
