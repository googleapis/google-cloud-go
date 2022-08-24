package main

import (
	"flag"
	"log"

	"cloud.google.com/go/aliasfix"
)

func main() {
	flag.Parse()
	path := flag.Arg(0)
	if path == "" {
		log.Fatalf("expected one argument -- path to the directory needing updates")
	}
	if err := aliasfix.ProcessPath(path); err != nil {
		log.Fatal(err)
	}
}
