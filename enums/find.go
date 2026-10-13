package enums

import (
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/build/constraint"
	"go/constant"
	"go/token"
	"go/types"
	"io"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Find returns the enum types declared in the files of a package directory
// with their values and existing methods, sorted by file name and position.
// The files are keyed by their file path.
//
// A type is an enum when marked with a //#enum line comment. Its values are
// the consts of exactly that type, skipping consts marked with //#exclude and
// aliases of other enum consts like `StatusDefault Status = StatusActive`.
// The values are evaluated with go/types without loading imported packages,
// so values that use imports, like http.MethodGet, are referenced by name.
//
// Values are read from the file declaring the enum type, where the
// generated methods are written, and from the other files of the package
// that the go tool compiles in every build of that file: files ending in
// _test.go or starting with _ or . are ignored, and so are files with
// build constraints, like a //go:build line, a file name suffix like
// _linux.go or _arm64.go, or an import "C", that are not true in every
// build of the file of the enum type. Existing methods are read from the
// files that can be compiled together with the file of the enum type,
// except files with another declaration of the enum type.
//
// All enum types of the package are checked and all errors are returned.
func Find(fset *token.FileSet, files map[string]*ast.File) ([]*Enum, error) {
	fileNames := slices.Sorted(maps.Keys(files))
	var (
		enums       []*Enum
		errs        []error
		constraints = make(map[*ast.File]constraint.Expr) // build constraints, nil for every build
		infos       = make(map[string]*types.Info)        // type checked files by their names
		// Files of packages with enums, checked for marker comments at wrong places
		markerFiles = make(map[string]bool)
		enumTypes   = make(map[string]map[string]bool) // names of enum types by package name
	)
	for _, name := range fileNames {
		if isIgnoredFile(name) {
			continue
		}
		c, err := buildConstraint(fset, name, files[name])
		if err != nil {
			errs = append(errs, err)
			continue
		}
		constraints[files[name]] = c
	}
	for _, fileName := range fileNames {
		file := files[fileName]
		if _, ok := constraints[file]; !ok {
			continue // ignored or invalid build constraint
		}
		typeSpecs, err := findEnumTypeSpecs(fset, file)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if len(typeSpecs) == 0 {
			continue
		}
		for _, typeSpec := range typeSpecs {
			if enumTypes[file.Name.Name] == nil {
				enumTypes[file.Name.Name] = make(map[string]bool)
			}
			enumTypes[file.Name.Name][typeSpec.spec.Name.Name] = true
		}

		var (
			pkgFiles     []*ast.File // compiled in every build of file
			pkgFileNames []string
			methodFiles  []*ast.File // compiled in some build of file
		)
		for _, name := range fileNames {
			f := files[name]
			if _, ok := constraints[f]; !ok || f.Name.Name != file.Name.Name {
				continue
			}
			markerFiles[name] = true
			// A file that is never compiled with the package, like a script for
			// go run with //go:build ignore, is compatible with no other file
			if name != fileName && !compatible(constraints[file], constraints[f]) {
				continue
			}
			methodFiles = append(methodFiles, f)
			if name == fileName || implies(constraints[file], constraints[f]) {
				pkgFiles = append(pkgFiles, f)
				pkgFileNames = append(pkgFileNames, name)
			}
		}
		key := strings.Join(pkgFileNames, "\n")
		info, ok := infos[key]
		if !ok {
			info = typeCheck(fset, pkgFiles)
			infos[key] = info
		}

		for _, typeSpec := range typeSpecs {
			enum, err := newEnum(fset, info, pkgFiles, methodFiles, constraints, file, typeSpec)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			enums = append(enums, enum)
		}
	}
	for _, name := range fileNames {
		if markerFiles[name] {
			if err := checkStrayMarkers(fset, files[name], enumTypes[files[name].Name.Name]); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return enums, nil
}

// checkStrayMarkers returns an error for a marker comment outside of the
// function bodies of file that is not at the end of a type or const line
// or in the doc comment of a function, or that is at a var or at a const
// that is not of one of the enumTypes, because it would be ignored silently.
// Markers at those places are checked where the enums are found.
func checkStrayMarkers(fset *token.FileSet, file *ast.File, enumTypes map[string]bool) error {
	allowed := make(map[*ast.CommentGroup]bool)
	var bodies []ast.Node
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			allowed[decl.Doc] = true
			if decl.Body != nil {
				bodies = append(bodies, decl.Body)
			}
		case *ast.GenDecl:
			allowed[decl.Doc] = decl.Tok == token.TYPE
			var (
				typ    ast.Expr // explicit or implicitly repeated type of a const
				values []ast.Expr
			)
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					allowed[spec.Doc], allowed[spec.Comment] = true, true
				case *ast.ValueSpec:
					if spec.Type != nil || len(spec.Values) > 0 {
						typ, values = spec.Type, spec.Values
					}
					if decl.Tok == token.CONST && enumTypes[constTypeName(typ, values)] {
						allowed[decl.Doc], allowed[spec.Doc], allowed[spec.Comment] = true, true, true
						continue
					}
					// Like `StatusNone = "" //#null` without type
					for _, group := range []*ast.CommentGroup{spec.Doc, spec.Comment} {
						if c := findMarker(group); c != nil {
							pos := fset.Position(c.Pos())
							return fmt.Errorf("%s in %s:%d is no const of an enum type, so its marker comment %q would be ignored",
								spec.Names[0].Name, pos.Filename, pos.Line, c.Text)
						}
					}
				}
			}
		}
	}
	for _, group := range file.Comments {
		if allowed[group] || slices.ContainsFunc(bodies, func(body ast.Node) bool {
			return body.Pos() <= group.Pos() && group.End() <= body.End()
		}) {
			continue
		}
		if c := findMarker(group); c != nil {
			pos := fset.Position(c.Pos())
			return fmt.Errorf("invalid marker comment %q in %s:%d, markers must be at the end of a type or const line or in the doc comment of a method",
				c.Text, pos.Filename, pos.Line)
		}
	}
	return nil
}

// findMarker returns the first marker comment of group, or nil.
func findMarker(group *ast.CommentGroup) *ast.Comment {
	if group == nil {
		return nil
	}
	for _, c := range group.List {
		if commentMarker(c) != "" {
			return c
		}
	}
	return nil
}

// baseTypeName returns the name of a type expression without type
// arguments and parentheses, like j of j[int], or an empty string.
func baseTypeName(x ast.Expr) string {
	switch x := x.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.ParenExpr:
		return baseTypeName(x.X)
	case *ast.IndexExpr:
		return baseTypeName(x.X)
	case *ast.IndexListExpr:
		return baseTypeName(x.X)
	}
	return ""
}

// constTypeName returns the name of the explicit type of a const, or of the
// type its untyped value is converted to, like Method of
// `MethodGet = Method(http.MethodGet)`, or an empty string.
func constTypeName(typ ast.Expr, values []ast.Expr) string {
	if ident, ok := typ.(*ast.Ident); ok {
		return ident.Name
	}
	if typ == nil && len(values) > 0 {
		if call, ok := values[0].(*ast.CallExpr); ok {
			if ident, ok := call.Fun.(*ast.Ident); ok {
				return ident.Name
			}
		}
	}
	return ""
}

// typeCheck type checks the files of a package.
// Imported packages are not loaded, so expressions that use imports
// have unknown values. Type errors are ignored because a package
// usually doesn't compile before its enum methods are generated.
func typeCheck(fset *token.FileSet, files []*ast.File) *types.Info {
	info := &types.Info{
		Defs: make(map[*ast.Ident]types.Object),
		Uses: make(map[*ast.Ident]types.Object),
	}
	conf := types.Config{
		// Without importer, go/types reports an error for each import
		// and uses a fake package without declarations
		FakeImportC:      true,
		IgnoreFuncBodies: true, // only package level declarations are used
		Error:            func(error) {},
	}
	// The returned error is the first error passed to conf.Error
	_, _ = conf.Check(files[0].Name.Name, fset, files, info)
	return info
}

// enumTypeSpec is a type declaration marked with //#enum
type enumTypeSpec struct {
	decl       *ast.GenDecl
	spec       *ast.TypeSpec
	jsonSchema bool
}

// findEnumTypeSpecs returns the type declarations of the file
// marked with a //#enum line comment, or an error for comments
// of type declarations that use a reserved marker word but not
// exactly as //#enum marker at the end of the type line.
func findEnumTypeSpecs(fset *token.FileSet, file *ast.File) ([]enumTypeSpec, error) {
	var specs []enumTypeSpec
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			groups := []*ast.CommentGroup{typeSpec.Doc, typeSpec.Comment, genDecl.Doc}
			isEnum := false
			for _, group := range groups {
				if group == nil {
					continue
				}
				for _, c := range group.List {
					if commentMarker(c) == "" {
						continue
					}
					jsonSchema, ok := parseEnumMarker(c.Text)
					if !ok || group != typeSpec.Comment || isEnum {
						pos := fset.Position(c.Pos())
						return nil, fmt.Errorf("invalid marker comment %q for type %s in %s:%d, only //#enum or //#enum,jsonschema at the end of the type line is supported",
							c.Text, typeSpec.Name.Name, pos.Filename, pos.Line)
					}
					specs = append(specs, enumTypeSpec{decl: genDecl, spec: typeSpec, jsonSchema: jsonSchema})
					isEnum = true
				}
			}
		}
	}
	return specs, nil
}

// parseEnumMarker parses a //#enum comment with optional comma
// separated flags. The only flag is jsonschema, others are ignored.
func parseEnumMarker(text string) (jsonSchema, ok bool) {
	flags, ok := strings.CutPrefix(text, "//#enum")
	if !ok {
		return false, false
	}
	flags = strings.TrimSpace(flags)
	if flags == "" {
		return false, true
	}
	flags, ok = strings.CutPrefix(flags, ",")
	if !ok {
		return false, false
	}
	for flag := range strings.SplitSeq(flags, ",") {
		if strings.TrimSpace(flag) == "jsonschema" {
			jsonSchema = true
		}
	}
	return jsonSchema, true
}

// newEnum returns the enum declared by typeSpec in file with its values
// in pkgFiles and its existing methods in methodFiles.
func newEnum(fset *token.FileSet, info *types.Info, pkgFiles, methodFiles []*ast.File, constraints map[*ast.File]constraint.Expr, file *ast.File, typeSpec enumTypeSpec) (*Enum, error) {
	// Not adjusted by //line directives because the methods
	// are written to the file that declares the type
	typePos := fset.PositionFor(typeSpec.spec.Pos(), false)
	typeName := typeSpec.spec.Name.Name
	if n := len(slices.DeleteFunc(slices.Clone(pkgFiles), func(f *ast.File) bool { return !declaresType(f, typeName) })); n > 1 {
		return nil, fmt.Errorf("enum type %s in %s:%d is declared more than once in the package", typeName, typePos.Filename, typePos.Line)
	}
	obj, _ := info.Defs[typeSpec.spec.Name].(*types.TypeName)
	if obj == nil || typeSpec.spec.Assign.IsValid() {
		return nil, fmt.Errorf("enum type %s in %s:%d must be a type definition, not an alias", typeName, typePos.Filename, typePos.Line)
	}
	basic, _ := obj.Type().Underlying().(*types.Basic)
	if basic == nil || basic.Kind() == types.Invalid || basic.Info()&types.IsComplex != 0 {
		return nil, fmt.Errorf("enum type %s in %s:%d needs a string, integer, float or bool type of the package as underlying type, not %s",
			typeName, typePos.Filename, typePos.Line, types.ExprString(typeSpec.spec.Type))
	}
	enum := &Enum{
		File:           typePos.Filename,
		Line:           typePos.Line,
		Package:        file.Name.Name,
		Type:           typeName,
		Underlying:     types.ExprString(typeSpec.spec.Type),
		underlyingName: baseTypeName(typeSpec.spec.Type),
		JSONSchema:     typeSpec.jsonSchema,
		CustomMethods:  make(map[string]bool),
		basic:          basic,
		insertAfter:    typeSpec.decl,
	}
	if err := enum.findValues(fset, info, pkgFiles, file, obj.Type()); err != nil {
		return nil, err
	}
	if err := enum.findMethods(fset, methodFiles, constraints, file); err != nil {
		return nil, err
	}
	if err := enum.checkPackageNames(fset, methodFiles, file); err != nil {
		return nil, err
	}
	return enum, nil
}

// checkPackageNames returns an error if a declaration of the package or an
// import of the file of the enum type has the name of a package used by the
// generated methods, because the generated methods would refer to it instead.
// The declarations of all methodFiles are checked, because the generated
// methods are compiled together with them in some build.
func (e *Enum) checkPackageNames(fset *token.FileSet, methodFiles []*ast.File, file *ast.File) error {
	// The names of the packages used by the methods that are not //#custom
	_, imports, err := e.generateMethods()
	if err != nil {
		return err
	}
	var names []string
	for name, importPath := range generatorImports {
		if slices.Contains(imports, importPath) {
			names = append(names, name)
		}
	}
	for _, f := range methodFiles {
		for _, decl := range f.Decls {
			var idents []*ast.Ident
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Recv == nil {
					idents = append(idents, decl.Name)
				}
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *ast.ValueSpec:
						idents = append(idents, spec.Names...)
					case *ast.TypeSpec:
						idents = append(idents, spec.Name)
					}
				}
			}
			for _, ident := range idents {
				if slices.Contains(names, ident.Name) {
					pos := fset.Position(ident.Pos())
					return fmt.Errorf("%s in %s:%d has the name of the package %s used by the generated methods of enum type %s",
						ident.Name, pos.Filename, pos.Line, generatorImports[ident.Name], e.Type)
				}
			}
		}
	}
	for _, imp := range file.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return err
		}
		// Without import name, the package name is assumed like by goimports,
		// which can be wrong, so only if the file uses that name
		name := assumedPackageName(importPath)
		if imp.Name != nil {
			name = imp.Name.Name
		} else if !usedPackageNames(file)[name] {
			continue
		}
		if slices.Contains(names, name) && importPath != generatorImports[name] {
			spec := imp.Path.Value
			if imp.Name != nil {
				spec = imp.Name.Name + " " + spec
			}
			pos := fset.Position(imp.Pos())
			return fmt.Errorf("import %s in %s:%d has the name of the package %s used by the generated methods of enum type %s, import it with another name",
				spec, pos.Filename, pos.Line, generatorImports[name], e.Type)
		}
	}
	return nil
}

// assumedPackageName returns the package name that goimports assumes for an
// import path, like json for github.com/goccy/go-json or yaml for
// gopkg.in/yaml.v3, because packages are not loaded.
func assumedPackageName(importPath string) string {
	base := path.Base(importPath)
	if v, ok := strings.CutPrefix(base, "v"); ok {
		if _, err := strconv.Atoi(v); err == nil && path.Dir(importPath) != "." {
			base = path.Base(path.Dir(importPath))
		}
	}
	base = strings.TrimPrefix(base, "go-")
	if i := strings.IndexFunc(base, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' }); i >= 0 {
		base = base[:i]
	}
	return base
}

// findValues sets the values of the enum from the consts of enumType.
func (e *Enum) findValues(fset *token.FileSet, info *types.Info, pkgFiles []*ast.File, file *ast.File, enumType types.Type) error {
	var (
		nullPos    token.Position
		seen       = make(map[string]*Value)     // exact constant value -> first enum value
		enumConsts = make(map[types.Object]bool) // all consts of enumType, also with invalid type
	)
	for _, f := range pkgFiles {
		for _, decl := range f.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.CONST {
				specValues := constSpecValues(genDecl)
				for i, spec := range genDecl.Specs {
					for j, name := range spec.(*ast.ValueSpec).Names {
						if isEnumConst(info, name, valueAt(specValues[i], j), enumType) {
							enumConsts[info.Defs[name]] = true
						}
					}
				}
			}
		}
	}
	for _, f := range pkgFiles {
		for _, decl := range f.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				continue
			}
			specValues := constSpecValues(genDecl)
			isEnumSpec := func(i int) bool {
				for j, name := range genDecl.Specs[i].(*ast.ValueSpec).Names {
					if isEnumConst(info, name, valueAt(specValues[i], j), enumType) {
						return true
					}
				}
				return false
			}
			for specIndex, spec := range genDecl.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				values := specValues[specIndex]
				if !isEnumSpec(specIndex) {
					// A const of the enum type in an expression with an import,
					// like Method(http.MethodGet) + "X", has an invalid type
					for j, name := range valueSpec.Names {
						c, ok := info.Defs[name].(*types.Const)
						if ok && c.Type() == types.Typ[types.Invalid] && usesType(info, valueAt(values, j), enumType) {
							pos := fset.Position(name.Pos())
							return fmt.Errorf("enum const %s in %s:%d can't be evaluated because it uses an import, declare it with the type like %s %s = ...",
								name.Name, pos.Filename, pos.Line, name.Name, e.Type)
						}
					}
					continue
				}
				// Generated methods are inserted after
				// the last enum const of the enum type file
				if f == file {
					e.insertAfter = genDecl
				}
				null, exclude, err := constMarkers(fset, genDecl, valueSpec)
				if err != nil {
					return err
				}
				specPos := fset.Position(valueSpec.Pos())
				// A null value in another file would make the enum nullable
				// by an edit outside of the file where the methods are generated
				if null && f != file {
					return fmt.Errorf("the //#null enum const %s in %s:%d must be declared in %s, the file of the enum type",
						valueSpec.Names[0].Name, specPos.Filename, specPos.Line, e.File)
				}
				if null && len(valueSpec.Names) > 1 {
					return fmt.Errorf("cant use //#null for multiple enums in %s:%d", specPos.Filename, specPos.Line)
				}
				if exclude {
					continue
				}
				for i, name := range valueSpec.Names {
					if !isEnumConst(info, name, valueAt(values, i), enumType) {
						continue
					}
					c := info.Defs[name].(*types.Const)
					if i >= len(values) {
						return fmt.Errorf("enum const %s of type %s.%s in %s:%d needs a value",
							name.Name, e.Package, e.Type, specPos.Filename, specPos.Line)
					}
					if name.Name == "_" {
						if null {
							return fmt.Errorf("the //#null enum const in %s:%d can't be blank", specPos.Filename, specPos.Line)
						}
						// A blank const reserves a value but can't be used
						continue
					}
					// An alias has the value of another enum const,
					// but the null value must be an enum value
					if !null && isAlias(info, values[i], enumConsts) {
						continue
					}
					value := &Value{Name: name.Name, Pos: fset.Position(name.Pos()), Null: null}
					value.StringExpr, value.JSONExpr = e.valueExprs(name.Name, c.Val())
					if null {
						if e.Null != "" {
							return fmt.Errorf("second //#null enum encountered %s in %s:%d (first //#null: %s in %s:%d)",
								name.Name, value.Pos.Filename, value.Pos.Line, e.Null, nullPos.Filename, nullPos.Line)
						}
						e.Null, nullPos = name.Name, value.Pos
					}
					if c.Val().Kind() != constant.Unknown {
						key := c.Val().ExactString()
						if first, ok := seen[key]; ok {
							return fmt.Errorf("duplicate enum value %s for type %s.%s in %s:%d (used by both %s in %s:%d and %s)",
								c.Val(), e.Package, e.Type, value.Pos.Filename, value.Pos.Line,
								first.Name, first.Pos.Filename, first.Pos.Line, value.Name)
						}
						seen[key] = value
					}
					e.Values = append(e.Values, value)
				}
			}
		}
	}
	if len(e.Values) == 0 {
		return fmt.Errorf("enum type %s.%s in %s:%d has no typed const enum values", e.Package, e.Type, e.File, e.Line)
	}
	return nil
}

// valueExprs returns Go expressions of an enum value as string
// for EnumStrings() and for the JSON Schema enum.
func (e *Enum) valueExprs(name string, value constant.Value) (stringExpr, jsonExpr string) {
	var literal string
	switch value.Kind() {
	case constant.String:
		literal = strconv.Quote(constant.StringVal(value))
		return literal, literal
	case constant.Int:
		literal = value.ExactString()
	case constant.Float:
		f, _ := constant.Float64Val(value)
		bitSize := 64
		if e.basic.Kind() == types.Float32 {
			bitSize = 32
		}
		literal = strconv.FormatFloat(f, 'g', -1, bitSize)
	case constant.Bool:
		literal = value.String()
	default:
		// Values that use imports can't be evaluated
		jsonExpr = e.Underlying + "(" + name + ")"
		if e.IsStringType() {
			return "string(" + name + ")", jsonExpr
		}
		return "fmt.Sprint(" + e.basic.Name() + "(" + name + "))", jsonExpr
	}
	// Untyped constants default to string and int,
	// other underlying types need a conversion
	jsonExpr = literal
	if e.Underlying != "int" {
		jsonExpr = e.Underlying + "(" + literal + ")"
	}
	return strconv.Quote(literal), jsonExpr
}

// findMethods finds the existing methods of the enum type in the package files,
// including files with build constraints, except files with another declaration
// of the enum type. Methods in the file of the enum type that were generated
// by go-enum are replaced, other methods that would be generated must be marked
// with //#custom to not be generated again.
func (e *Enum) findMethods(fset *token.FileSet, methodFiles []*ast.File, constraints map[*ast.File]constraint.Expr, file *ast.File) error {
	// Receiver names of generated methods, other methods
	// in the file of the enum type, and methods in other files
	var recvs [3]string
	// Files of the //#custom methods in other files than the file of the enum type
	customFiles := make(map[string][]*ast.File)
	for _, f := range methodFiles {
		if f != file && declaresType(f, e.Type) {
			continue
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || recvTypeName(fn) != e.Type {
				continue
			}
			generatedDoc := f == file && hasGeneratedDoc(fset, fn)
			// A method that is never generated for the underlying type,
			// like String of an integer enum, is hand-written
			generated := generatedDoc && e.canGenerate(fn.Name.Name)
			if names := fn.Recv.List[0].Names; len(names) > 0 && names[0].Name != "_" {
				i := 2
				if f == file {
					i = 1
				}
				if generated {
					i = 0
				}
				recvs[i] = cmp.Or(recvs[i], names[0].Name)
			}
			if _, ok := generatedPatterns[fn.Name.Name]; !ok {
				continue
			}
			custom, err := isCustom(fset, fn)
			if err != nil {
				return err
			}
			pos := fset.Position(fn.Pos())
			switch {
			case custom:
				if e.generates(fn.Name.Name) {
					e.CustomMethods[fn.Name.Name] = true
					customFiles[fn.Name.Name] = append(customFiles[fn.Name.Name], f)
				}
			case generatedDoc && !generated && hasGeneratedBody(fset, fn):
				// Generated before the underlying type was changed, like String
				// of a string enum that is wrong for an integer enum, or written
				// like a generated method, like String of a rune enum
				return fmt.Errorf("method %s.%s in %s:%d has the code of a method generated by go-enum for another underlying type than %s, delete it or mark it with //#custom to keep it",
					e.Type, fn.Name.Name, pos.Filename, pos.Line, e.Underlying)
			case generated && (e.generates(fn.Name.Name) || hasGeneratedBody(fset, fn) || e.wasGenerated(fset, fn)):
				// Replaced or removed if it is no longer generated
				e.generated = append(e.generated, fn)
			case generated:
				// Generated before, but no longer generated and changed since,
				// for example with values that no longer exist
				return fmt.Errorf("method %s.%s in %s:%d was generated by go-enum but is no longer generated and was changed since, delete it or mark it with //#custom to keep it",
					e.Type, fn.Name.Name, pos.Filename, pos.Line)
			case !e.generates(fn.Name.Name):
				// A hand-written method like MarshalJSON of an enum that is not nullable
			case f == file:
				return fmt.Errorf("method %s.%s in %s:%d was not generated by go-enum, mark it with //#custom to keep it or delete it to generate it",
					e.Type, fn.Name.Name, pos.Filename, pos.Line)
			default:
				return fmt.Errorf("method %s.%s in %s:%d would be generated again in %s, mark it with //#custom to keep it",
					e.Type, fn.Name.Name, pos.Filename, pos.Line, e.File)
			}
		}
	}
	// The method is not generated in every build of the file of the enum type,
	// so the //#custom methods must be declared in every one of them
	for _, name := range slices.Sorted(maps.Keys(customFiles)) {
		var (
			fileConstraints []constraint.Expr
			fileNames       []string
		)
		for _, f := range customFiles[name] {
			fileConstraints = append(fileConstraints, constraints[f])
			fileNames = append(fileNames, fset.PositionFor(f.Package, false).Filename)
		}
		if !implies(constraints[file], fileConstraints...) {
			return fmt.Errorf("method %s.%s is marked with //#custom only in %s, which the go tool does not compile in every build of %s, add the method with //#custom for the other builds",
				e.Type, name, strings.Join(fileNames, ", "), e.File)
		}
	}
	firstLetter := ""
	if i := strings.IndexFunc(e.Type, unicode.IsLetter); i >= 0 {
		r, _ := utf8.DecodeRuneInString(e.Type[i:])
		firstLetter = string(unicode.ToLower(r))
	}
	for _, recv := range []string{recvs[0], recvs[1], recvs[2], firstLetter, "e", "recv"} {
		if e.usableRecv(recv) {
			e.Recv = recv
			return nil
		}
	}
	for i := 1; ; i++ {
		if recv := "e" + strconv.Itoa(i); e.usableRecv(recv) {
			e.Recv = recv
			return nil
		}
	}
}

// usableRecv reports whether name can be the receiver name of the
// generated methods without shadowing an identifier that they use,
// like an enum value in Valid() or the fmt package in Validate().
func (e *Enum) usableRecv(name string) bool {
	if name == "" || name == "_" || name == e.Type || name == e.underlyingName || types.Universe.Lookup(name) != nil {
		return false
	}
	if _, ok := generatorImports[name]; ok {
		return false
	}
	return !slices.ContainsFunc(e.Values, func(v *Value) bool { return v.Name == name })
}

// declaresType reports whether file declares a type with the name.
func declaresType(file *ast.File, name string) bool {
	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
			for _, spec := range genDecl.Specs {
				if spec.(*ast.TypeSpec).Name.Name == name {
					return true
				}
			}
		}
	}
	return false
}

// recvTypeName returns the receiver type name of a method
// or an empty string if fn is not a method.
func recvTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	recvType := ast.Unparen(fn.Recv.List[0].Type)
	if star, ok := recvType.(*ast.StarExpr); ok {
		recvType = ast.Unparen(star.X)
	}
	if ident, ok := recvType.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// isEnumConst reports whether name declares a const of enumType with value.
// An untyped const converting a value that can't be evaluated to enumType,
// like `MethodGet = Method(http.MethodGet)` with an imported package,
// has an invalid type, but is also a const of enumType.
func isEnumConst(info *types.Info, name *ast.Ident, value ast.Expr, enumType types.Type) bool {
	c, ok := info.Defs[name].(*types.Const)
	if !ok {
		return false
	}
	if types.Identical(c.Type(), enumType) {
		return true
	}
	call, ok := ast.Unparen(value).(*ast.CallExpr)
	if c.Type() != types.Typ[types.Invalid] || !ok || len(call.Args) != 1 {
		return false
	}
	fun, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	typeName, ok := info.Uses[fun].(*types.TypeName)
	return ok && types.Identical(typeName.Type(), enumType)
}

// valueAt returns values[i] or nil if values has no value at i.
func valueAt(values []ast.Expr, i int) ast.Expr {
	if i < len(values) {
		return values[i]
	}
	return nil
}

// isAlias reports whether a const value is another const
// of the enum type, like StatusActive in
// `StatusDefault Status = StatusActive`.
func isAlias(info *types.Info, value ast.Expr, enumConsts map[types.Object]bool) bool {
	ident, ok := ast.Unparen(value).(*ast.Ident)
	return ok && enumConsts[info.Uses[ident]]
}

// usesType reports whether expr references the type name of typ.
func usesType(info *types.Info, expr ast.Expr, typ types.Type) bool {
	if expr == nil {
		return false
	}
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok {
			if typeName, ok := info.Uses[ident].(*types.TypeName); ok && types.Identical(typeName.Type(), typ) {
				found = true
			}
		}
		return !found
	})
	return found
}

// constSpecValues returns the values of the specs of a const declaration,
// repeated by specs without type and values like with iota.
func constSpecValues(genDecl *ast.GenDecl) [][]ast.Expr {
	specValues := make([][]ast.Expr, len(genDecl.Specs))
	for i, spec := range genDecl.Specs {
		valueSpec := spec.(*ast.ValueSpec)
		if i > 0 && valueSpec.Type == nil && len(valueSpec.Values) == 0 {
			specValues[i] = specValues[i-1]
		} else {
			specValues[i] = valueSpec.Values
		}
	}
	return specValues
}

// isIgnoredFile reports whether the go tool ignores the file
// because it is a test file or its name starts with _ or .
func isIgnoredFile(fileName string) bool {
	base := filepath.Base(fileName)
	return strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "_") || strings.HasPrefix(base, ".")
}

// maxBuildTags limits the independent build tags of compared build
// constraints, like cgo or custom tags, and maxBuilds the evaluated
// builds, because every combination of their values is evaluated.
const (
	maxBuildTags = 12
	maxBuilds    = 1 << 20
)

// buildConstraint returns the build constraint of a file, the and of its
// //go:build line, or its // +build lines without //go:build line, the
// GOOS and GOARCH of its file name like linux and arm64 of foo_linux_arm64.go,
// and cgo if it imports "C". It returns nil if the go tool compiles the file
// in every build.
func buildConstraint(fset *token.FileSet, fileName string, file *ast.File) (constraint.Expr, error) {
	var goBuild, plusBuild []constraint.Expr
	// Like go/build, // +build lines are only read before the package doc
	// comment and before the first /* */ comment
	plusBuildHeader := true
	for _, group := range file.Comments {
		if group.Pos() >= file.Package {
			break
		}
		if group == file.Doc || slices.ContainsFunc(group.List, func(c *ast.Comment) bool { return strings.HasPrefix(c.Text, "/*") }) {
			plusBuildHeader = false
		}
		for _, c := range group.List {
			if !constraint.IsGoBuild(c.Text) && !(plusBuildHeader && constraint.IsPlusBuild(c.Text)) {
				continue
			}
			x, err := constraint.Parse(c.Text)
			if err != nil {
				pos := fset.PositionFor(c.Pos(), false)
				return nil, fmt.Errorf("invalid build constraint in %s:%d: %w", pos.Filename, pos.Line, err)
			}
			if constraint.IsGoBuild(c.Text) {
				goBuild = append(goBuild, x)
			} else {
				plusBuild = append(plusBuild, x)
			}
		}
	}
	exprs := goBuild
	if len(goBuild) == 0 {
		exprs = plusBuild
	}
	for _, tag := range fileNameTags(fileName) {
		exprs = append(exprs, &constraint.TagExpr{Tag: tag})
	}
	if slices.ContainsFunc(file.Imports, func(imp *ast.ImportSpec) bool { return imp.Path.Value == `"C"` }) {
		exprs = append(exprs, &constraint.TagExpr{Tag: "cgo"})
	}
	var and constraint.Expr
	for _, x := range exprs {
		if and == nil {
			and = x
		} else {
			and = &constraint.AndExpr{X: and, Y: x}
		}
	}
	return and, nil
}

// fileNameTags returns the GOOS and GOARCH of a file name, like linux
// and arm64 of foo_linux_arm64.go, following the rules of go/build:
// only the part before the first dot counts and a _test suffix
// is not part of the constraint.
func fileNameTags(fileName string) []string {
	name, _, _ := strings.Cut(filepath.Base(fileName), ".")
	i := strings.Index(name, "_")
	if i < 0 {
		return nil
	}
	parts := strings.Split(name[i:], "_")
	if n := len(parts); n > 0 && parts[n-1] == "test" {
		parts = parts[:n-1]
	}
	n := len(parts)
	switch {
	case n >= 2 && isGOOS(parts[n-2]) && isGOARCH(parts[n-1]):
		return parts[n-2:]
	case n >= 1 && (isGOOS(parts[n-1]) || isGOARCH(parts[n-1])):
		return parts[n-1:]
	default:
		return nil
	}
}

// implies reports whether one of bs is true in every build where a is
// true. A nil constraint is true in every build.
func implies(a constraint.Expr, bs ...constraint.Expr) bool {
	key := []string{"implies", exprString(a)}
	for _, b := range bs {
		if b == nil || exprString(b) == exprString(a) {
			return true
		}
		key = append(key, exprString(b))
	}
	return cachedComparison(key, func() bool {
		return forAllBuilds(func(eval func(constraint.Expr) bool) bool {
			return !eval(a) || slices.ContainsFunc(bs, eval)
		}, append(bs, a)...)
	})
}

// compatible reports whether a and b are both true in some build.
func compatible(a, b constraint.Expr) bool {
	return cachedComparison([]string{"compatible", exprString(a), exprString(b)}, func() bool {
		return !forAllBuilds(func(eval func(constraint.Expr) bool) bool {
			return !eval(a) || !eval(b)
		}, a, b)
	})
}

// buildComparisons caches the results of implies and compatible
// by the constraints, because packages have few distinct ones
var buildComparisons sync.Map

// cachedComparison returns the cached result of compare for key.
func cachedComparison(key []string, compare func() bool) bool {
	k := strings.Join(key, "\n")
	if result, ok := buildComparisons.Load(k); ok {
		return result.(bool)
	}
	result := compare()
	buildComparisons.Store(k, result)
	return result
}

// exprString returns the text of a build constraint, or "" for nil.
func exprString(x constraint.Expr) string {
	if x == nil {
		return ""
	}
	return x.String()
}

// forAllBuilds reports whether f returns true for every build, where eval
// returns the value of an expression of exprs in the build. The builds are
// the combinations of the values of the build tags of exprs that the go tool
// sets together: the GOOS tags and unix like one GOOS, for example android
// sets android, linux and unix, one GOARCH tag, the go1.N release tags up to
// one Go version of at least 1.18, because the generated methods use any,
// one of gc and gccgo, and never ignore, used for files that are not part of
// the package. Other build tags, like cgo, are independent. It returns false
// for more than maxBuildTags independent build tags or maxBuilds builds.
func forAllBuilds(f func(eval func(constraint.Expr) bool) bool, exprs ...constraint.Expr) bool {
	tagSet := make(map[string]bool)
	for _, x := range exprs {
		if x != nil {
			x.Eval(func(tag string) bool {
				tagSet[tag] = true
				return false
			})
		}
	}
	var goosTags, archTags, releaseTags, others []string
	for _, tag := range slices.Sorted(maps.Keys(tagSet)) {
		switch {
		case tag == "unix" || tag == "gc" || tag == "gccgo" || tag == "ignore":
		case isGOOS(tag):
			goosTags = append(goosTags, tag)
		case isGOARCH(tag):
			archTags = append(archTags, tag)
		case releaseVersion(tag) > 0:
			releaseTags = append(releaseTags, tag)
		default:
			others = append(others, tag)
		}
	}
	// The options of the settings below times the combinations of others
	if len(others) > maxBuildTags || (len(goosTags)+2)*(len(archTags)+1)*(len(releaseTags)+1)*2<<len(others) > maxBuilds {
		return false
	}
	// The settings of a build with an option for each value,
	// every option sets all tags of its setting
	var goos, goarch []map[string]bool
	for _, g := range goosTags {
		option := map[string]bool{"unix": matchTag(g, "unix")}
		for _, tag := range goosTags {
			option[tag] = matchTag(g, tag)
		}
		goos = append(goos, option)
	}
	for _, unix := range []bool{false, true} { // another GOOS
		option := map[string]bool{"unix": unix}
		for _, tag := range goosTags {
			option[tag] = false
		}
		goos = append(goos, option)
	}
	for _, a := range append(slices.Clone(archTags), "") { // or another GOARCH
		option := make(map[string]bool)
		for _, tag := range archTags {
			option[tag] = tag == a
		}
		goarch = append(goarch, option)
	}
	// The Go versions of the release tags, set like by the version
	// of the option, without a map per option for many release tags
	versions := []int{18}
	for _, tag := range releaseTags {
		versions = append(versions, max(releaseVersion(tag), 18))
	}
	compiler := []map[string]bool{{"gc": true, "gccgo": false}, {"gc": false, "gccgo": true}}
	settings := [][]map[string]bool{goos, goarch, compiler}

	set := make(map[string]bool)
	eval := func(x constraint.Expr) bool {
		return x == nil || x.Eval(func(tag string) bool { return set[tag] })
	}
	var forAll func(settings [][]map[string]bool) bool
	forAll = func(settings [][]map[string]bool) bool {
		if len(settings) > 0 {
			for _, option := range settings[0] {
				maps.Copy(set, option)
				if !forAll(settings[1:]) {
					return false
				}
			}
			return true
		}
		for _, version := range versions {
			for _, tag := range releaseTags {
				set[tag] = releaseVersion(tag) <= version
			}
			for bits := range 1 << len(others) {
				for i, tag := range others {
					set[tag] = bits>>i&1 == 1
				}
				if !f(eval) {
					return false
				}
			}
		}
		return true
	}
	return forAll(settings)
}

// releaseVersion returns N of a release build tag go1.N, or 0.
func releaseVersion(tag string) int {
	minor, ok := strings.CutPrefix(tag, "go1.")
	if !ok {
		return 0
	}
	// Only the canonical form, like go1.21 but not go1.021 or go1.+21
	if minor == "" || minor[0] == '0' || strings.ContainsFunc(minor, func(r rune) bool { return r < '0' || r > '9' }) {
		return 0
	}
	n, err := strconv.Atoi(minor)
	if err != nil {
		return 0
	}
	return n
}

// isGOOS reports whether s is a GOOS value known to go/build.
func isGOOS(s string) bool {
	return platformTag(s) == "GOOS"
}

// isGOARCH reports whether s is a GOARCH value known to go/build.
func isGOARCH(s string) bool {
	return platformTag(s) == "GOARCH"
}

// platformTags caches the results of platformTag
var platformTags sync.Map

// platformTag returns "GOOS" or "GOARCH" if tag is one known to go/build,
// or an empty string.
func platformTag(tag string) string {
	if kind, ok := platformTags.Load(tag); ok {
		return kind.(string)
	}
	kind := ""
	switch {
	case tag == "" || strings.ContainsAny(tag, "_."):
		// Not a file name suffix, like a custom tag or amd64.v3
	case !matchFile(build.Context{GOARCH: "amd64"}, "x_"+tag+"_amd64.go", "package p"):
		// Only for a known GOOS the file name has a GOOS and a GOARCH suffix,
		// else only the GOARCH suffix is matched
		kind = "GOOS"
	case !matchFile(build.Context{}, "x_"+tag+".go", "package p"):
		// A file with the suffix of a known GOARCH
		// doesn't match a context without GOARCH
		kind = "GOARCH"
	}
	platformTags.Store(tag, kind)
	return kind
}

// osTags caches the results of matchTag by GOOS and tag
var osTags sync.Map

// matchTag reports whether a build tag is set for a GOOS.
func matchTag(goos, tag string) bool {
	key := goos + "\n" + tag
	if match, ok := osTags.Load(key); ok {
		return match.(bool)
	}
	match := matchFile(build.Context{GOOS: goos}, "x.go", "//go:build "+tag+"\n\npackage p")
	osTags.Store(key, match)
	return match
}

// matchFile reports whether a file with the name and source matches the context.
func matchFile(ctxt build.Context, name, src string) bool {
	ctxt.OpenFile = func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(src)), nil
	}
	match, err := ctxt.MatchFile("", name)
	return err == nil && match
}

// constMarkers returns whether an enum const is marked with //#null
// or //#exclude, or an error if a comment of the const uses a reserved
// marker word but not exactly as one of these markers at the end of
// the const line, so that a misused marker fails instead of being ignored.
func constMarkers(fset *token.FileSet, genDecl *ast.GenDecl, valueSpec *ast.ValueSpec) (null, exclude bool, err error) {
	for _, group := range []*ast.CommentGroup{genDecl.Doc, valueSpec.Doc, valueSpec.Comment} {
		if group == nil {
			continue
		}
		for _, c := range group.List {
			switch {
			case commentMarker(c) == "":
			case group == valueSpec.Comment && c.Text == "//#null":
				null = true
			case group == valueSpec.Comment && c.Text == "//#exclude":
				exclude = true
			default:
				pos := fset.Position(c.Pos())
				return false, false, fmt.Errorf("invalid marker comment %q for enum const %s in %s:%d, only //#null or //#exclude at the end of the const line are supported",
					c.Text, valueSpec.Names[0].Name, pos.Filename, pos.Line)
			}
		}
	}
	return null, exclude, nil
}

// isCustom reports whether the method's doc comment contains a `//#custom`
// marker line. Such methods are treated as hand-written overrides and are
// neither replaced nor regenerated.
//
// gofmt rewrites `//#custom` in doc comments to `// #custom`,
// so both forms are accepted. Other comments with a reserved marker
// word, like `//#custom legacy`, are an error. Other words like a
// misspelled `// #custum` are ignored, see commentMarker.
func isCustom(fset *token.FileSet, fn *ast.FuncDecl) (bool, error) {
	if fn.Doc == nil {
		return false, nil
	}
	custom := false
	for _, c := range fn.Doc.List {
		switch commentMarker(c) {
		case "":
		case "#custom":
			custom = true
		default:
			pos := fset.Position(c.Pos())
			return false, fmt.Errorf("invalid marker comment %q for method %s in %s:%d, only //#custom is supported",
				c.Text, fn.Name.Name, pos.Filename, pos.Line)
		}
	}
	return custom, nil
}

// markers are the reserved words of marker comments
var markers = []string{"enum", "null", "exclude", "custom"}

// commentMarker returns the text of a // comment without the slashes
// if it starts with # followed by a reserved marker word, like #null,
// #custom, #exclude legacy or #enum,jsonschema, else an empty string.
// Other words like `// #nosec G101`, `//#region` or a misspelled
// `//#exlude`, and doc comment headings like `// # Heading`
// are not markers and ignored.
func commentMarker(c *ast.Comment) string {
	text, ok := strings.CutPrefix(c.Text, "//")
	if !ok {
		return ""
	}
	text = strings.TrimSpace(text)
	name, ok := strings.CutPrefix(text, "#")
	if !ok {
		return ""
	}
	if i := strings.IndexFunc(name, func(r rune) bool { return !unicode.IsLetter(r) }); i >= 0 {
		name = name[:i]
	}
	if !slices.Contains(markers, name) {
		return ""
	}
	return text
}
