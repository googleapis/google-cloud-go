package main

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"slices"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

type alias struct {
	TypeImport string
	TypeName   string
}

func AddAliases(file *ast.File, aliases map[string]*alias) {
	for _, a := range aliases {
		file.Decls = append(file.Decls, &ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{
				&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent(a.TypeName)},
					Values: []ast.Expr{
						&ast.SelectorExpr{
							X:   ast.NewIdent(a.TypeImport),
							Sel: ast.NewIdent(a.TypeName),
						},
					},
				},
			},
		})
	}
}

func AddStorageV2ContextToControl(f *ast.File) {
	astutil.Apply(f, nil, func(c *astutil.Cursor) bool {
		n := c.Node()
		switch x := n.(type) {
		case *ast.TypeSpec:
			if x.Name.Name == "StorageControlClient" {
				st := x.Type.(*ast.StructType)
				st.Fields.List = append(st.Fields.List, &ast.Field{
					Names: []*ast.Ident{ast.NewIdent("internalStorageClient")},
					Type: &ast.StarExpr{
						X: &ast.SelectorExpr{
							X:   ast.NewIdent("sv2pb"),
							Sel: ast.NewIdent("Client"),
						},
					},
				})
			}
		case *ast.FuncDecl:
			if x.Name.Name == "NewStorageControlClient" {
				bs := x.Body
				lreturn := bs.List[len(bs.List)-1]
				astutil.Apply(bs, nil, func(d *astutil.Cursor) bool {
					i := d.Node()
					if i != lreturn {
						return true
					}
					switch i.(type) {
					case *ast.ReturnStmt:
						d.InsertBefore(&ast.AssignStmt{
							Lhs: []ast.Expr{ast.NewIdent("v2"), ast.NewIdent("err")},
							Tok: token.DEFINE,
							Rhs: []ast.Expr{&ast.CallExpr{
								Fun: &ast.SelectorExpr{
									X:   ast.NewIdent("sv2pb"),
									Sel: ast.NewIdent("NewClient"),
								},
								Args: []ast.Expr{ast.NewIdent("ctx"), ast.NewIdent("opts...")},
							}},
						})
						d.InsertBefore(&ast.IfStmt{
							Cond: &ast.BinaryExpr{X: ast.NewIdent("err"), Op: token.NEQ, Y: ast.NewIdent("nil")},
							Body: &ast.BlockStmt{List: []ast.Stmt{
								&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("nil"), ast.NewIdent("err")}},
							}},
						})
						d.InsertBefore(&ast.AssignStmt{
							Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent("client"), Sel: ast.NewIdent("internalStorageClient")}},
							Tok: token.ASSIGN,
							Rhs: []ast.Expr{ast.NewIdent("v2")},
						})
					}
					return true
				})
			}
		}
		return true
	})
}

func getStructFields(file *ast.File, structName string) []*ast.Field {
	var fields []*ast.Field
	ast.Inspect(file, func(n ast.Node) bool {
		if ts, ok := n.(*ast.TypeSpec); ok {
			if ts.Name.Name == structName {
				if st, ok := ts.Type.(*ast.StructType); ok {
					fields = st.Fields.List
					return false
				}
			}
		}
		return true
	})
	return fields
}

func GenerateWrapper(storagev2 *ast.File, storageControl *ast.File, structType string) *ast.FuncDecl {
	// Create the assignment statements for each field.
	var assignStmts []ast.Stmt
	for _, field := range getStructFields(storageControl, structType) { // Replace "StructA" with your actual struct name
		assignStmt := &ast.AssignStmt{
			Lhs: []ast.Expr{
				&ast.SelectorExpr{
					X:   ast.NewIdent("r"),
					Sel: ast.NewIdent(field.Names[0].Name)},
			},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{
				&ast.SelectorExpr{
					X:   ast.NewIdent("a"),
					Sel: ast.NewIdent(field.Names[0].Name),
				},
			},
		}
		assignStmts = append(assignStmts, assignStmt)
	}

}

func GetStorageV2Funcs(file *ast.File, structName string, includedFuncNames []string, newStructName, internalClientName string) ([]*ast.FuncDecl, map[string]*alias) {
	var fi []*ast.FuncDecl
	aliases := make(map[string]*alias)
	astutil.Apply(file, nil, func(c *astutil.Cursor) bool {
		n := c.Node()
		switch x := n.(type) {
		case *ast.FuncDecl:
			// Check only public API
			if x.Recv != nil && token.IsExported(x.Name.Name) && slices.Contains(includedFuncNames, x.Name.Name) {
				// Only want to copy methods from structName
				recvType := x.Recv.List[0].Type
				switch t := recvType.(type) {
				case *ast.StarExpr:
					if o := t.X.(*ast.Ident); o.Name == structName {
						o.Name = newStructName
						// Update parameter list to remove direct ref to storagev2 client proto
						for _, param := range x.Type.Params.List {
							for _, name := range param.Names {
								if name.Name == "req" {
									starE := param.Type.(*ast.StarExpr)
									selE := starE.X.(*ast.SelectorExpr)
									if selE.X.(*ast.Ident).Name == "storagepb" {
										aliases[selE.Sel.Name] = &alias{
											TypeImport: selE.X.(*ast.Ident).Name,
											TypeName:   selE.Sel.Name,
										}
										selE.X.(*ast.Ident).Name = "controlpb"
									}
								}
							}
						}
						// Update return values to use local aliases
						for _, result := range x.Type.Results.List {
							if starE, ok := result.Type.(*ast.StarExpr); ok {
								switch seX := starE.X.(type) {
								case *ast.SelectorExpr:
									name := seX.X.(*ast.Ident)
									// TODO: Get import based on name of object here?
									if name.Name == "storagepb" && name.Name != "iampb" {
										aliases[seX.Sel.Name] = &alias{
											TypeImport: seX.X.(*ast.Ident).Name,
											TypeName:   seX.Sel.Name,
										}
										name.Name = "controlpb"
									}
								case *ast.Ident:
									// Assume this is a type defined in sv2pb
									aliases[seX.Name] = &alias{
										TypeImport: "sv2pb",
										TypeName:   seX.Name,
									}
								}
							}
						}

						astutil.Apply(x, nil, func(z *astutil.Cursor) bool {
							q := z.Node()
							switch v := q.(type) {
							case *ast.CallExpr:
								// modify internalClient name within func call
								se := v.Fun.(*ast.SelectorExpr)
								internalClientExpr := se.X.(*ast.SelectorExpr)
								if internalClientExpr.Sel.Name == "internalClient" {
									internalClientExpr.Sel.Name = internalClientName
								}
							}
							return true
						})
						fi = append(fi, x)
					}
				}
			}
		}
		return true
	})
	return fi, aliases
}

func RemoveFuncDef(file *ast.File, funcName string) {
	astutil.Apply(file, nil, func(c *astutil.Cursor) bool {
		n := c.Node()
		switch x := n.(type) {
		case *ast.Comment:
			if strings.Contains(x.Text, funcName) {
				c.Delete()
			}
		case *ast.FuncDecl:
			if x.Name.Name == funcName {
				c.Delete()
			}
		case *ast.TypeSpec:
			// Remove from StorageControlCallOptions
			if x.Name.Name == "StorageControlCallOptions" || x.Name.Name == "internalStorageControlClient" {
				astutil.Apply(n, func(c *astutil.Cursor) bool {
					if field, ok := c.Node().(*ast.Field); ok {
						for _, ident := range field.Names {
							if ident.Name == funcName {
								c.Delete()
								return false // Stop iterating after removing the field
							}
						}
					}
					return true
				}, nil)
			}
		case *ast.KeyValueExpr:
			// Remove from defaultStorageControlCallOptions() default values
			if x.Key.(*ast.Ident).Name == funcName {
				c.Delete()
			}
		}
		return true
	})
}

func main() {
	// Read the source files for Storage Control and Storage V2
	fset := token.NewFileSet()
	storageControlFile, err := parser.ParseFile(fset, "../storage_control_client.go", nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	storageV2File, err := parser.ParseFile(fset, "../../../internal/apiv2/storage_client.go", nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	if ok := astutil.AddNamedImport(fset, storageControlFile, "sv2pb", "cloud.google.com/go/storage/internal/apiv2"); !ok {
		panic("Unable to add import sv2pb")
	}
	// Auto detect these imports
	if ok := astutil.AddNamedImport(fset, storageControlFile, "storagepb", "cloud.google.com/go/storage/internal/apiv2/storagepb"); !ok {
		panic("Unable to add import storagepb")
	}
	if ok := astutil.AddNamedImport(fset, storageControlFile, "iampb", "cloud.google.com/go/iam/apiv1/iampb"); !ok {
		panic("Unable to add import iampb")
	}
	// detect imports
	AddStorageV2ContextToControl(storageControlFile)
	allowed := []string{
		"GetBucket", "DeleteBucket",
	}
	// Copy Storage V2 into Storage Control
	funcs, _ := GetStorageV2Funcs(storageV2File, "Client", allowed, "StorageControlClient", "internalStorageClient")
	for _, fn := range allowed {
		RemoveFuncDef(storageControlFile, fn)
	}
	// AddAliases(storageControlFile, aliases)
	for _, fn := range funcs {
		storageControlFile.Decls = append(storageControlFile.Decls, fn)
	}

	// Print the modified AST
	printer.Fprint(os.Stdout, fset, storageControlFile)
}
