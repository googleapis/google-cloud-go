package main

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"

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

func AddStorageV2Func(file *ast.File, funcName, requestTypeName, responseTypeName string) {
	file.Decls = append(file.Decls,
		&ast.FuncDecl{
			Recv: &ast.FieldList{List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent("c")},
					Type:  &ast.StarExpr{X: ast.NewIdent("StorageControlClient")},
				},
			}},
			Name: ast.NewIdent(funcName),
			Type: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("ctx")},
						Type:  ast.NewIdent("context.Context"),
					},
					{
						Names: []*ast.Ident{ast.NewIdent("req")},
						Type:  &ast.StarExpr{X: ast.NewIdent(requestTypeName)},
					},
					{
						Names: []*ast.Ident{ast.NewIdent("opts")},
						Type:  &ast.Ellipsis{Elt: ast.NewIdent("gax.CallOption")},
					},
				}},
				Results: &ast.FieldList{List: []*ast.Field{
					{Type: &ast.StarExpr{X: ast.NewIdent(responseTypeName)}},
					{Type: ast.NewIdent("error")},
				}},
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.ReturnStmt{Results: []ast.Expr{
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   &ast.SelectorExpr{X: ast.NewIdent("c"), Sel: ast.NewIdent("internalStorageClient")},
							Sel: ast.NewIdent(funcName),
						},
						Args: []ast.Expr{
							ast.NewIdent("ctx"), ast.NewIdent("req"), ast.NewIdent("opts..."),
						},
					},
				}},
			}},
		})
}

func main() {
	// Read the source file
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "../storage_control_client.go", nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	// file2, err := parser.ParseFile(fset, "../storage_control_client_modified.go", nil, parser.ParseComments)
	// if err != nil {
	// 	panic(err)
	// }
	// ast.Print(nil, file2)

	if ok := astutil.AddNamedImport(fset, file, "sv2pb", "cloud.google.com/go/storage/internal/apiv2"); !ok {
		panic("Unable to add import sv2pb")
	}
	if ok := astutil.AddNamedImport(fset, file, "storagepb", "cloud.google.com/go/storage/internal/apiv2/storagepb"); !ok {
		panic("Unable to add import storagepb")
	}

	astutil.Apply(file, nil, func(c *astutil.Cursor) bool {
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
	// Add funcs
	AddAliases(file, map[string]*alias{
		"Bucket":              &alias{TypeImport: "storagepb", TypeName: "Bucket"},
		"GetBucketRequest":    &alias{TypeImport: "storagepb", TypeName: "GetBucketRequest"},
		"CreateBucketRequest": &alias{TypeImport: "storagepb", TypeName: "CreateBucketRequest"},
	})
	AddStorageV2Func(file, "GetBucket", "GetBucketRequest", "Bucket")
	AddStorageV2Func(file, "CreateBucket", "CreateBucketRequest", "Bucket")

	// Print the modified AST
	printer.Fprint(os.Stdout, fset, file)
}
