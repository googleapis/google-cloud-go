package main

import (
	"flag"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/postprocessor/execv/gocmd"
)

func main() {
	srcRoot := flag.String("src", "owl-bot-staging/src/", "Path to owl-bot-staging directory")
	dstRoot := flag.String("dst", "", "Path to clients")
	flag.Parse()

	srcPrefix := *srcRoot
	dstPrefix := *dstRoot

	log.Println("srcPrefix set to", srcPrefix)
	log.Println("dstPrefix set to", dstPrefix)

	if err := run(srcPrefix, dstPrefix); err != nil {
		log.Fatal(err)
	}

	// TODO: delete owl-bot-staging file

	log.Println("Files copied and formatted from owl-bot-staging to libraries.")
}

func run(srcPrefix, dstPrefix string) error {
	var srcPath string
	var dstPath string

	filepath.WalkDir(srcPrefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		srcPath = path
		dstPath = filepath.Join(dstPrefix, strings.TrimPrefix(path, srcPrefix))

		if err := copyFiles(srcPath, dstPath); err != nil {
			return err
		}

		return nil
	})

	gocmd.ModTidyAll(dstPath)
	gocmd.Vet(dstPath)
	return nil
}

func copyFiles(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}

	return nil
}
