// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// protoveneer generates idiomatic Go types that correspond to protocol
// buffer messages and enums -- a veneer on top of the proto layer.
//
// # Relationship to GAPICs
//
// GAPICs and this tool complement each other.
//
// GAPICs have client types and idiomatic methods on them that correspond to
// RPCs. They focus on the RPC part of a service. A GAPIC relies on the
// underlying protocol buffer types for the request and response types, and all
// the types that these refer to.
//
// protoveener generates Go types that correspond to proto messages and enums,
// including requests and responses if desired. It doesn't touch the RPC parts
// of the proto definition.
//
// # Configuration
//
// protoveneer requires significant configuration to produce good results.
// See the config type in config.go and the config.yaml files in the testdata
// subdirectories to understand how to write configuration.
//
// # Unsupported features
//
// There is no support for oneofs. Omit the oneof type and write custom code.
// However, the types of the individual oneof cases can be generated.
package main

// TODO:
// - have omitFields on a TypeConfig, like omitTypes
// - Instead of parseCustomConverter, accept a list. Users can use the inline form
//   to be compact.
// - Check that a configured field is actually in the type.

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

var (
	outputDir   = flag.String("outdir", "", "directory to write to, or '-' for stdout")
	noFormat    = flag.Bool("nofmt", false, "do not format output")
	licenseFile = flag.String("license", "", "filename with license text")
)

func main() {
	log.SetPrefix("protoveneer: ")
	log.SetFlags(0)
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "usage: protoveneer CONFIG.yaml DIR_WITH_pb.go_FILES\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if err := run(context.Background(), flag.Arg(0), flag.Arg(1), *outputDir); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, configFile, pbDir, outDir string) error {
	config, err := readConfigFile(configFile)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	pkg, err := parseDir(fset, pbDir)
	if err != nil {
		return err
	}

	src, err := generate(config, pkg, fset)
	if err != nil {
		return err
	}
	if !*noFormat {
		src, err = format.Source(src)
		if err != nil {
			return fmt.Errorf("formatting: %v", err)
		}
	}

	if outDir == "-" {
		fmt.Printf("%s\n", src)
	} else {
		outfile := fmt.Sprintf("%s_veneer.gen.go", pkg.Name)
		if outDir != "" {
			outfile = filepath.Join(outDir, outfile)
		}
		if err := os.WriteFile(outfile, src, 0660); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("wrote %s\n", outfile)
	}
	return nil
}

func parseDir(fset *token.FileSet, dir string) (*ast.Package, error) {
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if len(pkgs) > 2 {
		return nil, errors.New("too many packages")
	}
	var pkg *ast.Package
	for name, apkg := range pkgs {
		if !strings.HasSuffix(name, "_test") {
			pkg = apkg
			break
		}
	}
	if pkg == nil {
		return nil, errors.New("no non-test package")
	}
	for filename := range pkg.Files {
		if !strings.HasSuffix(filename, ".pb.go") {
			return nil, fmt.Errorf("%s is not a .pb.go file", filename)
		}
	}
	return pkg, nil
}

func generate(conf *config, pkg *ast.Package, fset *token.FileSet) (src []byte, err error) {
	// Get information about all the types in the proto package.
	typeInfos, err := collectDecls(pkg)
	if err != nil {
		return nil, err
	}

	// Check that every configured type is present.
	for protoName := range conf.Types {
		if typeInfos[protoName] == nil {
			return nil, fmt.Errorf("configured type %s does not exist in package", protoName)
		}
	}

	// Consult the config to determine which types to omit.
	// If a type isn't matched by a glob in the OmitTypes list, it is output.
	// If a type is matched, but it also has a config, it is still output.
	var toWrite []*typeInfo
	for name, ti := range typeInfos {
		if !ast.IsExported(name) {
			continue
		}
		omit, err := sliceAnyError(conf.OmitTypes, func(glob string) (bool, error) {
			return path.Match(glob, name)
		})
		if err != nil {
			return nil, err
		}
		if !omit || conf.Types[name] != nil {
			toWrite = append(toWrite, ti)
		}
	}

	// Fill in the configured type names, which we need to do the rest of the work.
	for _, ti := range toWrite {
		if tc := conf.Types[ti.protoName]; tc != nil && tc.Name != "" {
			ti.veneerName = tc.Name
		}
	}

	// Sort for determinism.
	sort.Slice(toWrite, func(i, j int) bool {
		return toWrite[i].veneerName < toWrite[j].veneerName
	})

	// Process and configure all the types we care about.
	// Even if there is no config for a type, there is still work to do.
	for _, ti := range toWrite {
		if err := processType(ti, conf.Types[ti.protoName], typeInfos); err != nil {
			return nil, err
		}
	}

	converters, err := buildConverterMap(toWrite, conf)
	if err != nil {
		return nil, err
	}

	// Use the converters map to give every field a converter.
	for _, ti := range toWrite {
		for _, f := range ti.fields {
			if f.converter != nil || f.noConvert {
				continue
			}
			f.converter, err = makeConverter(f.af.Type, f.protoType, converters)
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", ti.protoName, f.protoName, err)
			}
		}
	}

	// Write the generated code.
	return write(toWrite, conf, fset)
}

// buildConverterMap builds a map from veneer name to a converter, which writes code that converts between the proto and veneer.
// This is used for fields when generating conversion functions.
func buildConverterMap(typeInfos []*typeInfo, conf *config) (map[string]converter, error) {
	converters := map[string]converter{}
	// Build a converter for each proto type.
	for _, ti := range typeInfos {
		var conv converter
		// Custom converters on the type take precedence.
		if tc := conf.Types[ti.protoName]; tc != nil && tc.ConvertToFrom != "" {
			c, err := parseCustomConverter(ti.veneerName, tc.ConvertToFrom)
			if err != nil {
				return nil, err
			}
			conv = c
		} else {
			switch ti.spec.Type.(type) {
			case *ast.StructType:
				conv = protoConverter{veneerName: ti.veneerName}
			case *ast.Ident:
				conv = enumConverter{protoName: ti.protoName, veneerName: ti.veneerName}
			default:
				conv = identityConverter{}
			}
		}
		converters[ti.veneerName] = conv
	}

	// Add converters for used external types to the map.
	for _, et := range externalTypes {
		if et.used && et.convertTo != "" {
			converters[et.qualifiedName] = customConverter{et.convertTo, et.convertFrom}
			needSupport(et.convertTo)
			needSupport(et.convertFrom)
		}
	}

	// Add custom converters to the map.
	// These differ from custom converters on the proto types (a few lines above here)
	// because they are keyed by veneer type, not proto type.
	// That can matter when the proto type is omitted but there is a corresponding
	// veneer type.
	for key, value := range conf.Converters {
		c, err := parseCustomConverter(key, value)
		if err != nil {
			return nil, err
		}
		converters[key] = c
	}
	return converters, nil
}

func parseCustomConverter(name, value string) (converter, error) {
	toFunc, fromFunc := parseCommaPair(value)
	if toFunc == "" || fromFunc == "" {
		return nil, fmt.Errorf(`%s: ConvertToFrom = %q, want "toFunc, fromFunc"`, name, value)
	}
	return customConverter{toFunc, fromFunc}, nil
}

// parseCommaPair parses a string like "foo, bar" into "foo" and "bar".
func parseCommaPair(s string) (string, string) {
	a, b, _ := strings.Cut(s, ",")
	return strings.TrimSpace(a), strings.TrimSpace(b)
}

// makeConverter constructs a converter for the given type. Not every type is in the map: this
// function puts together converters for types like pointers, slices and maps, as well as
// named types.
func makeConverter(veneerType, protoType ast.Expr, converters map[string]converter) (converter, error) {
	if c, ok := converters[typeString(veneerType)]; ok {
		return c, nil
	}
	// If there is no converter for this type, look for a converter for a part of the type.
	switch t := veneerType.(type) {
	case *ast.Ident:
		// Handle the case where the veneer type is the dereference of the proto type.
		if se, ok := protoType.(*ast.StarExpr); ok {
			if identName(se.X) != t.Name {
				return nil, fmt.Errorf("veneer type %s does not match dereferenced proto type %s", t.Name, identName(se.X))
			}
			return derefConverter{}, nil
		}
		return identityConverter{}, nil
	case *ast.StarExpr:
		return makeConverter(t.X, protoType.(*ast.StarExpr).X, converters)
	case *ast.ArrayType:
		eltc, err := makeConverter(t.Elt, protoType.(*ast.ArrayType).Elt, converters)
		if err != nil {
			return nil, err
		}
		return sliceConverter{eltc}, nil
	case *ast.MapType:
		// Assume the key types are the same.
		vc, err := makeConverter(t.Value, protoType.(*ast.MapType).Value, converters)
		if err != nil {
			return nil, err
		}
		return mapConverter{vc}, nil
	default:
		return identityConverter{}, nil
	}
}

// A typeInfo holds information about a named type.
type typeInfo struct {
	// These fields are collected from the proto package.
	protoName string        // name of type in the proto package
	spec      *ast.TypeSpec // the spec for the type, which will be modified
	decl      *ast.GenDecl  // the decl holding the spec; not sure we need this
	values    *ast.GenDecl  // the list of values for an enum

	// These fields are added later.
	veneerName   string       // may be provided by config; else same as protoName
	fields       []*fieldInfo // for structs
	valueNames   []string     // to generate String functions
	populateFrom string       // name of function doing additional work converting from proto
	populateTo   string       // name of function doing additional work converting to proto
}

// A fieldInfo holds information about a struct field.
type fieldInfo struct {
	protoType             ast.Expr
	af                    *ast.Field
	protoName, veneerName string
	converter             converter
	noConvert             bool
}

// collectDecls collects declaration information from a package.
// It returns information about every named type in the package in a map
// keyed by the type's name.
func collectDecls(pkg *ast.Package) (map[string]*typeInfo, error) {
	typeInfos := map[string]*typeInfo{} // key is proto name

	getInfo := func(name string) *typeInfo {
		if info, ok := typeInfos[name]; ok {
			return info
		}
		info := &typeInfo{protoName: name, veneerName: name}
		typeInfos[name] = info
		return info
	}

	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok {
				switch gd.Tok {
				case token.TYPE:
					if len(gd.Specs) != 1 {
						return nil, errors.New("multiple TypeSpecs in a GenDecl not supported")
					}
					ts := gd.Specs[0].(*ast.TypeSpec)
					info := getInfo(ts.Name.Name)
					info.spec = ts
					info.decl = gd

				case token.CONST:
					// Assume consts for an enum type are grouped together, and every one has a type.
					// That's what the proto compiler generates.
					vs0 := gd.Specs[0].(*ast.ValueSpec)
					if len(vs0.Names) != 1 || len(vs0.Values) != 1 {
						return nil, errors.New("multiple names/values not supported")
					}

					protoName := identName(vs0.Type)
					if protoName == "" {
						continue
					}
					for _, s := range gd.Specs {
						vs := s.(*ast.ValueSpec)
						if identName(vs.Type) != protoName {
							return nil, fmt.Errorf("%s: not all same type", protoName)
						}
					}
					info := getInfo(protoName)
					info.values = gd
				}
			}
		}
	}
	return typeInfos, nil
}

// processType processes a single type, modifying the AST.
// If it's an enum, just change its name.
// If it's a struct, modify its name and fields.
func processType(ti *typeInfo, tconf *typeConfig, typeInfos map[string]*typeInfo) error {
	ti.spec.Name.Name = ti.veneerName
	switch t := ti.spec.Type.(type) {
	case *ast.StructType:
		// Check that all configured fields are present, and added fields are not.
		exportedFields := map[string]bool{}
		for _, f := range t.Fields.List {
			if len(f.Names) > 1 {
				return fmt.Errorf("%s: multiple names in one field spec not supported: %v", ti.protoName, f.Names)
			}
			if f.Names[0].IsExported() {
				exportedFields[f.Names[0].Name] = true
			}
		}
		if tconf != nil {
			for name, fconfig := range tconf.Fields {
				if !exportedFields[name] && !fconfig.Add {
					return fmt.Errorf("%s: configured field %s is not present", ti.protoName, name)
				}
				if exportedFields[name] && fconfig.Add {
					return fmt.Errorf("%s: added field %s is already present", ti.protoName, name)
				}
			}
		}
		// Process the existing fields.
		fs := t.Fields.List
		t.Fields.List = t.Fields.List[:0]
		for _, f := range fs {
			fi, err := processField(f, tconf, typeInfos)
			if err != nil {
				return err
			}
			if fi != nil {
				t.Fields.List = append(t.Fields.List, f)
				ti.fields = append(ti.fields, fi)
			}
		}
		// Add additional fields.
		if tconf != nil {
			for name, fconfig := range tconf.Fields {
				if fconfig.Add {
					if err := addField(name, fconfig, t, ti); err != nil {
						return err
					}
				}
			}
		}
		// Other processing.
		if tconf != nil && tconf.PopulateToFrom != "" {
			toFunc, fromFunc := parseCommaPair(tconf.PopulateToFrom)
			if toFunc == "" || fromFunc == "" {
				return fmt.Errorf(`%s: PopulateToFrom = %q, want "toFunc, fromFunc"`, ti.protoName, tconf.PopulateToFrom)
			}
			ti.populateTo = toFunc
			ti.populateFrom = fromFunc
		}
	case *ast.Ident:
		// Enum type. Nothing else to do with the type itself; but see processEnumValues.
	default:
		return fmt.Errorf("unknown type: %+v: protoName=%s", ti.spec, ti.protoName)
	}
	processDoc(ti.decl, ti.protoName, tconf)
	if ti.values != nil {
		ti.valueNames = processEnumValues(ti.values, tconf)
	}
	return nil
}

// processField processes a struct field.
func processField(af *ast.Field, tc *typeConfig, typeInfos map[string]*typeInfo) (*fieldInfo, error) {
	id := af.Names[0]
	if !id.IsExported() {
		return nil, nil
	}
	fi := &fieldInfo{
		protoType:  af.Type,
		af:         af,
		protoName:  id.Name,
		veneerName: id.Name,
	}
	if tc != nil {
		if fc, ok := tc.Fields[id.Name]; ok {
			if fc.Omit {
				return nil, nil
			}
			if fc.Name != "" {
				id.Name = fc.Name
				fi.veneerName = fc.Name
			}
			if fc.Type != "" {
				expr, err := parser.ParseExpr(fc.Type)
				if err != nil {
					return nil, err
				}
				af.Type = expr
			}
			if fc.Doc != "" {
				cg := &ast.CommentGroup{}
				for i, line := range strings.Split(strings.TrimSpace(fc.Doc), "\n") {
					c := &ast.Comment{Text: "// " + line}
					if i == 0 {
						c.Slash = af.Pos() - 1
					}
					cg.List = append(cg.List, c)
				}
				af.Doc = cg
			}
			if fc.ConvertToFrom != "" {
				c, err := parseCustomConverter(id.Name, fc.ConvertToFrom)
				if err != nil {
					return nil, err
				}
				fi.converter = c
			}
			fi.noConvert = fc.NoConvert
		}
	}
	af.Type = veneerType(af.Type, typeInfos)
	af.Tag = nil
	return fi, nil
}

// veneerType returns a type expression for the veneer type corresponding to the given proto type.
func veneerType(protoType ast.Expr, typeInfos map[string]*typeInfo) ast.Expr {
	var wtype func(ast.Expr) ast.Expr
	wtype = func(protoType ast.Expr) ast.Expr {
		if et := protoTypeToExternalType[typeString(protoType)]; et != nil {
			et.used = true
			return et.typeExpr
		}
		switch t := protoType.(type) {
		case *ast.Ident:
			if ti := typeInfos[t.Name]; ti != nil {
				wt := *t
				wt.Name = ti.veneerName
				return &wt
			}
		case *ast.ParenExpr:
			wt := *t
			wt.X = wtype(wt.X)
			return &wt

		case *ast.StarExpr:
			wt := *t
			wt.X = wtype(wt.X)
			return &wt

		case *ast.ArrayType:
			wt := *t
			wt.Elt = wtype(wt.Elt)
			return &wt
		}
		return protoType
	}

	return wtype(protoType)
}

func addField(name string, fconfig fieldConfig, t *ast.StructType, ti *typeInfo) error {
	expr, err := parser.ParseExpr(fconfig.Type)
	if err != nil {
		return err
	}
	af := &ast.Field{
		Names: []*ast.Ident{{Name: name}},
		Type:  expr,
	}
	t.Fields.List = append(t.Fields.List, af)
	ti.fields = append(ti.fields, &fieldInfo{
		af:         af,
		protoName:  "",
		veneerName: name,
		noConvert:  true,
	})
	return nil
}

// processEnumValues processes enum values.
// The proto compiler puts all the values for an enum in one GenDecl,
// and there are no other values in that GenDecl.
func processEnumValues(d *ast.GenDecl, tc *typeConfig) []string {
	var valueNames []string
	for _, s := range d.Specs {
		vs := s.(*ast.ValueSpec)
		id := vs.Names[0]
		protoName := id.Name
		veneerName := veneerValueName(id.Name, tc)
		valueNames = append(valueNames, veneerName)
		id.Name = veneerName

		if tc != nil {
			vs.Type.(*ast.Ident).Name = tc.Name
		}
		modifyCommentGroup(vs.Doc, protoName, veneerName, "means", "", false)
	}
	return valueNames
}

// veneerValueName returns an idiomatic Go name for a proto enum value.
func veneerValueName(protoValueName string, tc *typeConfig) string {
	if tc == nil {
		return protoValueName
	}
	if nn, ok := tc.ValueNames[protoValueName]; ok {
		return nn
	}
	name := strings.TrimPrefix(protoValueName, tc.ProtoPrefix)
	// Some values have the type name in upper snake case after the prefix.
	// Example:
	//    proto type: FinishReason
	//    prefix: Candidate_
	//    value:  Candidate_FINISH_REASON_UNSPECIFIED
	prefix := camelToUpperSnakeCase(tc.Name) + "_"
	name = strings.TrimPrefix(name, prefix)
	return tc.VeneerPrefix + snakeToCamelCase(name)
}

func processDoc(gd *ast.GenDecl, protoName string, tc *typeConfig) {
	doc := ""
	verb := ""
	remOther := false
	if tc != nil {
		doc = tc.Doc
		verb = tc.DocVerb
		remOther = tc.RemoveOtherDoc
	}

	spec := gd.Specs[0]
	var name string
	switch spec := spec.(type) {
	case *ast.TypeSpec:
		name = spec.Name.Name
	case *ast.ValueSpec:
		name = spec.Names[0].Name
	default:
		panic("bad spec")
	}
	if tc != nil && name != tc.Name {
		panic(fmt.Errorf("GenDecl name is %q, config name is %q", name, tc.Name))
	}
	modifyCommentGroup(gd.Doc, protoName, name, verb, doc, remOther)
}

func modifyCommentGroup(cg *ast.CommentGroup, protoName, veneerName, verb, doc string, removeOther bool) {
	if cg == nil {
		return
	}
	if len(cg.List) == 0 {
		return
	}
	c := cg.List[0]
	c.Text = "// " + adjustDoc(strings.TrimPrefix(c.Text, "// "), protoName, veneerName, verb, doc)
	if removeOther {
		cg.List = cg.List[:1]
	}
}

// adjustDoc takes a doc string with initial comment characters and whitespace removed, and returns
// a replacement that uses the given veneer name, verb and new doc string.
func adjustDoc(origDoc, protoName, veneerName, verb, newDoc string) string {
	// if newDoc is non-empty, completely replace the existing doc.
	if newDoc != "" {
		return veneerName + " " + newDoc
	}
	// If the doc string starts with the proto name, just replace it with the
	// veneer name. We can't do anything about the verb because we don't know
	// where it is in the original doc string. (I guess we could assume it's the
	// next word, but that might not always work.)
	if strings.HasPrefix(origDoc, protoName+" ") {
		return veneerName + origDoc[len(protoName):]
	}

	// Lowercase the first letter of the given doc if it's not part of an acronym.
	runes := []rune(origDoc)
	// It shouldn't be possible for the original doc string to be empty,
	// but check just in case to avoid panics.
	if len(runes) == 0 {
		return origDoc
	}
	// Heuristic: an acronym begins with two consecutive uppercase letters.
	if unicode.IsUpper(runes[0]) && (len(runes) == 1 || !unicode.IsUpper(runes[1])) {
		runes[0] = unicode.ToLower(runes[0])
		origDoc = string(runes)
	}

	if verb == "" {
		verb = "is"
	}
	return fmt.Sprintf("%s %s %s", veneerName, verb, origDoc)
}

////////////////////////////////////////////////////////////////

func write(typeInfos []*typeInfo, conf *config, fset *token.FileSet) ([]byte, error) {
	var buf bytes.Buffer
	pr := func(format string, args ...any) { fmt.Fprintf(&buf, format, args...) }
	prn := func(format string, args ...any) {
		pr(format, args...)
		pr("\n")
	}
	// Top of file.
	if *licenseFile != "" {
		licenseText, err := os.ReadFile(*licenseFile)
		if err != nil {
			return nil, err
		}
		pr("%s\n\n", licenseText)
	}
	pr("// This file was generated by protoveneer. DO NOT EDIT.\n\n")
	pr("package %s\n\n", conf.Package)

	// Imports.
	// format.Source  will sort paths within a group, but will not regroup.
	// So ensure stdlib imports are together.
	stdImportPaths := map[string]bool{}
	otherImportPaths := map[string]bool{}
	for _, et := range externalTypes {
		if et.used {
			for _, ip := range et.importPaths {
				if inStdLib(ip) {
					stdImportPaths[ip] = true
				} else {
					otherImportPaths[ip] = true
				}
			}
		}
	}
	prn("import (")
	prn(`    "fmt"`)
	for ip := range stdImportPaths {
		prn(`    "%s"`, ip)
	}
	pr("\n")
	prn(`    pb "%s"`, conf.ProtoImportPath)
	for ip := range otherImportPaths {
		// May be just a path, or "id path".
		id, path, found := strings.Cut(ip, " ")
		if !found {
			prn(`    "%s"`, ip)
		} else {
			prn(`  %s "%s"`, id, path)
		}
	}
	pr(")\n\n")

	// Types.
	for _, ti := range typeInfos {
		for _, decl := range []*ast.GenDecl{ti.decl, ti.values} {
			if decl != nil {
				data, err := formatDecl(fset, decl)
				if err != nil {
					return nil, err
				}
				buf.Write(data)
				prn("")
			}
		}
		if ti.valueNames != nil {
			if err := generateEnumStringMethod(&buf, ti.veneerName, ti.valueNames); err != nil {
				return nil, err
			}
		}
		if _, ok := ti.spec.Type.(*ast.StructType); ok {
			ti.generateConversionMethods(pr)
		}
	}
	if err := generateSupportFunctions(&buf, neededSupportFunctions); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatDecl(fset *token.FileSet, gd *ast.GenDecl) ([]byte, error) {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, gd); err != nil {
		return nil, err
	}
	// Remove blank lines that result from deleting unexported struct fields.
	return bytes.ReplaceAll(buf.Bytes(), []byte("\n\n"), []byte("\n")), nil
}

////////////////////////////////////////////////////////////////

var stringMethodTemplate = template.Must(template.New("").Parse(`
	var namesFor{{.Type}} = map[{{.Type}}]string {
		{{- range .Values}}
			{{.}}: "{{.}}",
		{{- end}}
	}

	func (v {{.Type}}) String() string {
		if n, ok := namesFor{{.Type}}[v]; ok {
			return n
		}
		return fmt.Sprintf("{{.Type}}(%d)", v)
	}
`))

func generateEnumStringMethod(w io.Writer, typeName string, valueNames []string) error {
	return stringMethodTemplate.Execute(w, struct {
		Type   string
		Values []string
	}{typeName, valueNames})
}

func (ti *typeInfo) generateConversionMethods(pr func(string, ...any)) {
	ti.generateToProto(pr)
	pr("\n")
	ti.generateFromProto(pr)
}

func (ti *typeInfo) generateToProto(pr func(string, ...any)) {
	pr("func (v *%s) toProto() *pb.%s {\n", ti.veneerName, ti.protoName)
	pr("  if v == nil { return nil }\n")
	if ti.populateTo == "" {
		pr("  return &pb.%s{\n", ti.protoName)
	} else {
		pr("  p := &pb.%s{\n", ti.protoName)
	}
	for _, f := range ti.fields {
		if f.noConvert {
			continue
		}
		pr("        %s: %s,\n", f.protoName, f.converter.genTo("v."+f.veneerName))
	}
	pr("    }\n")
	if ti.populateTo != "" {
		pr("  %s(p, v)\n", ti.populateTo)
		pr("  return p\n")
	}
	pr("}\n")
}

func (ti *typeInfo) generateFromProto(pr func(string, ...any)) {
	pr("func (%s) fromProto(p *pb.%s) *%[1]s {\n", ti.veneerName, ti.protoName)
	pr("  if p == nil { return nil }\n")
	if ti.populateFrom == "" {
		pr("  return &%s{\n", ti.veneerName)
	} else {
		pr("  v := &%s{\n", ti.veneerName)
	}
	for _, f := range ti.fields {
		if f.noConvert {
			continue
		}
		pr("        %s: %s,\n", f.veneerName, f.converter.genFrom("p."+f.protoName))
	}
	pr("    }\n")
	if ti.populateFrom != "" {
		pr("  %s(v, p)\n", ti.populateFrom)
		pr("  return v\n")
	}
	pr("}\n")
}

////////////////////////////////////////////////////////////////

// externalType holds information about a type that is not part of the proto package.
type externalType struct {
	qualifiedName string
	replaces      string
	importPaths   []string
	convertTo     string
	convertFrom   string

	typeExpr ast.Expr
	used     bool
}

var externalTypes = []*externalType{
	{
		qualifiedName: "civil.Date",
		replaces:      "*date.Date",
		importPaths:   []string{"cloud.google.com/go/civil", "google.golang.org/genproto/googleapis/type/date"},
		convertTo:     "pvCivilDateToProto",
		convertFrom:   "pvCivilDateFromProto",
	},
	{
		qualifiedName: "map[string]any",
		replaces:      "*structpb.Struct",
		importPaths:   []string{"google.golang.org/protobuf/types/known/structpb"},
		convertTo:     "pvMapToStructPB",
		convertFrom:   "pvMapFromStructPB",
	},
	{
		qualifiedName: "time.Time",
		replaces:      "*timestamppb.Timestamp",
		importPaths:   []string{"time", "google.golang.org/protobuf/types/known/timestamppb"},
		convertTo:     "pvTimeToProto",
		convertFrom:   "pvTimeFromProto",
	},
	{
		qualifiedName: "time.Duration",
		replaces:      "*durationpb.Duration",
		importPaths:   []string{"time", "google.golang.org/protobuf/types/known/durationpb"},
		convertTo:     "durationpb.New",
		convertFrom:   "pvDurationFromProto",
	},
	{
		qualifiedName: "*apierror.APIError",
		replaces:      "*status.Status",
		importPaths: []string{
			"github.com/googleapis/gax-go/v2/apierror",
			"spb google.golang.org/genproto/googleapis/rpc/status",
			"gstatus google.golang.org/grpc/status",
		},
		convertTo:   "pvAPIErrorToProto",
		convertFrom: "pvAPIErrorFromProto",
	},
}

var protoTypeToExternalType = map[string]*externalType{}

func init() {
	var err error
	for _, et := range externalTypes {
		if et.replaces == "" {
			continue
		}
		et.typeExpr, err = parser.ParseExpr(et.qualifiedName)
		if err != nil {
			panic(err)
		}
		protoTypeToExternalType[et.replaces] = et
	}
}

////////////////////////////////////////////////////////////////

//go:embed internal/support/support.go
var supportCode string

var neededSupportFunctions = map[string]bool{}

// needSupport should be called whenever a support function is needed by the generated code.
// It is OK to call it for functions that are not in the support package.
func needSupport(name string) { neededSupportFunctions[name] = true }

var (
	// Regexps to match the start and end of top-level functions.
	// These assume the file is gofmt'd.
	// The "m" flag means that ^ and $ match line starts and ends, respectively.
	startFuncRegexp = regexp.MustCompile(`(?m:^func ([A-Za-z0-9_]+))`)
	endFuncRegexp   = regexp.MustCompile(`(?m:^}$)`)
)

// generateSupportFunctions writes the support functions needed by the
// generated code to w.
func generateSupportFunctions(w io.Writer, need map[string]bool) error {
	// Walk through the file of support functions,
	// writing the ones whose names are in need.
	code := supportCode
	for {
		starts := startFuncRegexp.FindStringSubmatchIndex(code)
		if starts == nil {
			break
		}
		end := endFuncRegexp.FindStringIndex(code)
		if end == nil {
			return errors.New("generateSupportFunctions: missing function end")
		}
		// starts[0] to starts[1]: entire start regexp
		// starts[2] to starts[3]: function name
		// end[1]: index of newline after '}'.
		name := code[starts[2]:starts[3]]
		if need[name] {
			comment, err := extractFunctionComment(code, starts[0])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "\n%s%s\n", comment, code[starts[0]:end[1]]); err != nil {
				return err
			}
		}
		// Move past match.
		code = code[end[1]:]
	}

	// Keep in sync with the type in internal/support/support.go
	_, err := fmt.Fprintf(w, `
// pvPanic wraps panics from support functions.
// User-provided functions in the same package can also use it.
// It allows callers to distinguish conversion function panics from other panics.
type pvPanic error

// pvCatchPanic recovers from panics of type pvPanic and
// returns an error instead.
func pvCatchPanic[T any](f func() T) (_ T, err error) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(pvPanic); ok {
				err = r.(error)
			} else {
				panic(r)
			}
		}
	}()
	return f(), nil
}
`)
	return err
}

// extractFunctionComment returns the top-level comment for the function
// beginning at code[funcStart].
func extractFunctionComment(code string, funcStart int) (string, error) {
	// Take advantage of the facts that every support function has a doc comment,
	// and all functions are separated by at least one empty line.

	// Search backwards for a blank line.
	for i := funcStart; i > 0; i-- {
		if code[i] == '\n' && code[i-1] == '\n' {
			return code[i+1 : funcStart], nil
		}
	}
	return "", fmt.Errorf("could not find comment before function at %d", funcStart)
}

////////////////////////////////////////////////////////////////

var emptyFileSet = token.NewFileSet()

// typeString produces a string for a type expression.
func typeString(t ast.Expr) string {
	var buf bytes.Buffer
	err := format.Node(&buf, emptyFileSet, t)
	if err != nil {
		panic(err)
	}
	return buf.String()
}

func identName(x any) string {
	id, ok := x.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

func snakeToCamelCase(s string) string {
	words := strings.Split(s, "_")
	for i, w := range words {
		if len(w) == 0 {
			words[i] = w
		} else {
			words[i] = fmt.Sprintf("%c%s", unicode.ToUpper(rune(w[0])), strings.ToLower(w[1:]))
		}
	}
	return strings.Join(words, "")
}

func camelToUpperSnakeCase(s string) string {
	var res []rune
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			res = append(res, '_')
		}
		res = append(res, unicode.ToUpper(r))
	}
	return string(res)
}

func sliceAnyError[T any](s []T, f func(T) (bool, error)) (bool, error) {
	for _, e := range s {
		b, err := f(e)
		if err != nil {
			return false, err
		}
		if b {
			return true, nil
		}
	}
	return false, nil
}

// inStdLib reports whether the given import path could be part of the Go standard library,
// by reporting whether the first component lacks a '.'.
func inStdLib(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}
