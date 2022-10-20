package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Vet runs linters on all .go files recursively from the given directory.
func Vet(dir string) error {
	log.Println("vetting generated code")
	c := exec.Command("gofmt", "-s", "-d", "-w", "-l", ".")
	c.Dir = dir
	return c.Run()
}

// ModTidy tidies go.mod file in the specified directory.
func ModTidy(dir string) error {
	c := exec.Command("go", "mod", "tidy")
	c.Dir = dir
	return c.Run()
}

// ModTidyAll tidies all mod files from the specified root directory.
func ModTidyAll(dir string) error {
	log.Printf("[%s] finding all modules", dir)
	var modDirs []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "go.mod" {
			modDirs = append(modDirs, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, modDir := range modDirs {
		if err := ModTidy(modDir); err != nil {
			return err
		}
	}
	return nil
}

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

		copyErr := copyFiles(srcPath, dstPath)
		if copyErr != nil {
			return err
		}

		return nil
	})

	ModTidyAll(dstPath)
	Vet(dstPath)

	// TODO: delete owl-bot-staging file
}
