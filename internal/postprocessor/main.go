package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/postprocessor/execv/gocmd"
)

func copyFiles(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	nBytes, err := io.Copy(dst, src)
	_ = nBytes
	if err != nil {
		return err
	}

	return nil
}

func main() {
	localFlag := flag.Bool("local", false, "Local mode set")
	flag.Parse()

	var fromPrefix string

	if *localFlag {
		fmt.Println("local flag detected")
		fromPrefix = "../../owl-bot-staging/src/"
	} else {
		fmt.Println("no local flag set")
		fromPrefix = "owl-bot-staging/src/"
	}
	fmt.Println("fromPrefix set to", fromPrefix)

	var dstRoot string
	var dstPath string

	filepath.WalkDir(fromPrefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		srcPath := path

		if *localFlag {
			dstRoot = "../../"
			dstPath = filepath.Join(dstRoot, strings.TrimPrefix(path, fromPrefix))
		} else {
			dstPath = strings.TrimPrefix(path, fromPrefix)
		}

		if err := copyFiles(srcPath, dstPath); err != nil {
			return err
		}

		return nil
	})

	gocmd.ModTidyAll(dstPath)
	gocmd.Vet(dstPath)

	// TODO: delete owl-bot-staging file
}
