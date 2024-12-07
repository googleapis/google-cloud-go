package main

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"slices"

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
				&ast.TypeSpec{
					Name: ast.NewIdent(a.TypeName + "="),
					Type: &ast.SelectorExpr{
						X:   ast.NewIdent(a.TypeImport),
						Sel: ast.NewIdent(a.TypeName),
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

func GetStorageV2Func(file *ast.File, structName string, ignoredFuncNames []string, newStructName, internalClientName string) ([]*ast.FuncDecl, map[string]*alias) {
	var fi []*ast.FuncDecl
	aliases := make(map[string]*alias)
	astutil.Apply(file, nil, func(c *astutil.Cursor) bool {
		n := c.Node()
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Recv != nil && !slices.Contains(ignoredFuncNames, x.Name.Name) {
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
									aliases[selE.Sel.Name] = &alias{
										TypeImport: selE.X.(*ast.Ident).Name,
										TypeName:   selE.Sel.Name,
									}
									param.Type = &ast.StarExpr{
										X: ast.NewIdent(selE.Sel.Name),
									}

								}
							}
						}
						if len(x.Type.Results.List) > 1 {
							for idx, result := range x.Type.Results.List {
								if starE, ok := result.Type.(*ast.StarExpr); ok {
									seE := starE.X.(*ast.SelectorExpr)
									name := seE.X.(*ast.Ident)
									if name.Name == "storagepb" {
										aliases[seE.Sel.Name] = &alias{
											TypeImport: seE.X.(*ast.Ident).Name,
											TypeName:   seE.Sel.Name,
										}
										x.Type.Results.List[idx] = &ast.Field{
											Type: &ast.StarExpr{
												X: seE.Sel,
											},
										}
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
	if ok := astutil.AddNamedImport(fset, storageControlFile, "storagepb", "cloud.google.com/go/storage/internal/apiv2/storagepb"); !ok {
		panic("Unable to add import storagepb")
	}

	AddStorageV2ContextToControl(storageControlFile)
	disallowed := []string{
		"WriteObject", "ReadObject", "StartResumableWrite", "QueryWriteStatus", "CancelResumableWrite", "BidiWriteObject", "Close", "setGoogleClientInfo", "Connection",
		// Not ready yet
		"GetIamPolicy", "SetIamPolicy", "TestIamPermissions", "ListObjects", "ListBuckets",
	}
	// Copy Storage V2 into Storage Control
	funcs, aliases := GetStorageV2Func(storageV2File, "Client", disallowed, "StorageControlClient", "internalStorageClient")
	AddAliases(storageControlFile, aliases)
	for _, fn := range funcs {
		storageControlFile.Decls = append(storageControlFile.Decls, fn)
	}

	// Print the modified AST
	printer.Fprint(os.Stdout, fset, storageControlFile)
}
