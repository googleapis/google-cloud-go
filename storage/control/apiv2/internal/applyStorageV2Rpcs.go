package main

import (
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"os"
)

func main() {
	//Create a FileSet to work with
	fset := token.NewFileSet()
	//Parse the file and create an AST
	file, err := parser.ParseFile(fset, "../storage_control_client.go", nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	f, err := os.Open("../storage_control_client.go")
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer f.Close()
	format.Node(f, fset, file)
}
