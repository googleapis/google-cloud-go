package main

import (
	"log"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/gapicgen/generator"
	"cloud.google.com/go/internal/gensnippets"
	"cloud.google.com/go/internal/postprocessor/execv"
	"cloud.google.com/go/internal/postprocessor/execv/gocmd"
)

// RegenSnippets regenerates the snippets for all GAPICs configured to be generated.
func (p *postProcessor) RegenSnippets() error {
	log.Println("regenerating snippets")
	snippetDir := filepath.Join(p.googleCloudDir, "internal", "generated", "snippets")
	confs := p.getChangedClientConfs()
	apiShortnames, err := generator.ParseAPIShortnames(p.googleapisDir, confs, generator.ManualEntries)
	if err != nil {
		return err
	}
	dirs := p.getDirs()
	if err := gensnippets.GenerateSnippetsDirs(p.googleCloudDir, snippetDir, apiShortnames, dirs); err != nil {
		log.Printf("warning: got the following non-fatal errors generating snippets: %v", err)
	}
	if err := p.replaceAllForSnippets(snippetDir); err != nil {
		return err
	}
	if err := gocmd.ModTidy(snippetDir); err != nil {
		return err
	}
	return nil
}

// getChangedClientConfs iterates through the MicrogenGapicConfigs and returns
// a slice of the entries corresponding to modified modules and clients
func (p *postProcessor) getChangedClientConfs() []*generator.MicrogenConfig {
	if len(p.modules) != 0 {
		runConfs := []*generator.MicrogenConfig{}
		for _, conf := range generator.MicrogenGapicConfigs {
			for _, scope := range p.modules {
				scopePathElement := "/" + scope + "/"
				if strings.Contains(conf.InputDirectoryPath, scopePathElement) {
					runConfs = append(runConfs, conf)
				}
			}
		}
		return runConfs
	}
	return generator.MicrogenGapicConfigs
}

func (p *postProcessor) replaceAllForSnippets(snippetDir string) error {
	return execv.ForEachMod(p.googleCloudDir, func(dir string) error {
		processMod := false
		if p.modules != nil {
			// Checking each path component in its entirety prevents mistaken addition of modules whose names
			// contain the scope as a substring. For example if the scope is "video" we do not want to regenerate
			// snippets for "videointelligence"
			dirSlice := strings.Split(dir, "/")
			for _, mod := range p.modules {
				for _, dirElem := range dirSlice {
					if mod == dirElem {
						processMod = true
						break
					}
				}
			}
		}
		if !processMod {
			return nil
		}
		if dir == snippetDir {
			return nil
		}
		mod, err := gocmd.ListModName(dir)
		if err != nil {
			return err
		}
		// Replace it. Use a relative path to avoid issues on different systems.
		rel, err := filepath.Rel(snippetDir, dir)
		if err != nil {
			return err
		}
		return gocmd.EditReplace(snippetDir, mod, rel)
	})
}
