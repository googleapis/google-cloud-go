package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("serving from %s\n", cwd)
	log.Fatal(http.ListenAndServe(":9001",
		logWrapper(http.FileServer(http.Dir(cwd)))))
}

func logWrapper(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("-> %s", r.URL)
		h.ServeHTTP(w, r)
		log.Printf("<- %s", r.URL)
	})
}
