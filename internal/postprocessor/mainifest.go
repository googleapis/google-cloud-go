package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/postprocessor/execv/gocmd"
	"gopkg.in/yaml.v2"
)

const betaIndicator = "It is not stable"

// ManifestEntry is used for JSON marshaling in manifest.
type ManifestEntry struct {
	DistributionName  string      `json:"distribution_name"`
	Description       string      `json:"description"`
	Language          string      `json:"language"`
	ClientLibraryType string      `json:"client_library_type"`
	DocsURL           string      `json:"docs_url"`
	ReleaseLevel      string      `json:"release_level"`
	LibraryType       LibraryType `json:"library_type"`
}

type LibraryType string

const (
	GapicAutoLibraryType   LibraryType = "GAPIC_AUTO"
	GapicManualLibraryType LibraryType = "GAPIC_MANUAL"
	CoreLibraryType        LibraryType = "CORE"
	AgentLibraryType       LibraryType = "AGENT"
	OtherLibraryType       LibraryType = "OTHER"
)

// Manifest writes a manifest file with info about all of the confs.
func (p *postProcessor) Manifest() (map[string]ManifestEntry, error) {
	log.Println("updating gapic manifest")
	entries := map[string]ManifestEntry{} // Key is the package name.
	f, err := os.Create(filepath.Join(p.googleCloudDir, "internal", ".repo-metadata-full.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	for _, manual := range manualEntries {
		entries[manual.DistributionName] = manual
	}
	for inputDir, conf := range p.config.GoogleapisToImportPath {
		yamlPath := filepath.Join(p.googleapisDir, inputDir, conf.ServiceConfig)
		yamlFile, err := os.Open(yamlPath)
		if err != nil {
			return nil, err
		}
		yamlConfig := struct {
			Title string `yaml:"title"` // We only need the title field.
		}{}
		if err := yaml.NewDecoder(yamlFile).Decode(&yamlConfig); err != nil {
			return nil, fmt.Errorf("decode: %v", err)
		}
		docURL, err := docURL(p.googleCloudDir, conf.ImportPath)
		if err != nil {
			return nil, fmt.Errorf("unable to build docs URL: %v", err)
		}
		// TODO(codyoss): We can read a files doc.go to see if it has a warning
		// if needs. For beta/alpha use a guess
		releaseLevel, err := releaseLevel(p.googleCloudDir, conf.ImportPath)
		if err != nil {
			return nil, fmt.Errorf("unable to calculate release level for: %v", inputDir)
		}

		entry := ManifestEntry{
			DistributionName:  conf.ImportPath,
			Description:       yamlConfig.Title,
			Language:          "Go",
			ClientLibraryType: "generated",
			DocsURL:           docURL,
			ReleaseLevel:      releaseLevel,
			LibraryType:       GapicAutoLibraryType,
		}
		entries[conf.ImportPath] = entry
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return entries, enc.Encode(entries)
}

func docURL(cloudDir, importPath string) (string, error) {
	suffix := strings.TrimPrefix(importPath, "cloud.google.com/go/")
	mod, err := gocmd.CurrentMod(filepath.Join(cloudDir, suffix))
	if err != nil {
		return "", err
	}
	pkgPath := strings.TrimPrefix(strings.TrimPrefix(importPath, mod), "/")
	return "https://cloud.google.com/go/docs/reference/" + mod + "/latest/" + pkgPath, nil
}

func releaseLevel(cloudDir, importPath string) (string, error) {
	i := strings.LastIndex(importPath, "/")
	lastElm := importPath[i+1:]
	if strings.Contains(lastElm, "alpha") {
		return "alpha", nil
	} else if strings.Contains(lastElm, "beta") {
		return "beta", nil
	}

	// Determine by scanning doc.go for our beta disclaimer
	gapicRelPath := strings.TrimPrefix(importPath, "cloud.google.com/go/")
	docFile := filepath.Join(cloudDir, gapicRelPath, "doc.go")
	f, err := os.Open(docFile)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lineCnt int
	for scanner.Scan() && lineCnt < 50 {
		line := scanner.Text()
		if strings.Contains(line, betaIndicator) {
			return "beta", nil
		}
	}
	return "ga", nil
}

var manualEntries = []ManifestEntry{
	// Pure manual clients.
	{
		DistributionName:  "cloud.google.com/go/bigquery",
		Description:       "BigQuery",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/bigquery/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/bigtable",
		Description:       "Cloud BigTable",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/bigtable/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/datastore",
		Description:       "Cloud Datastore",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/datastore/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/iam",
		Description:       "Cloud IAM",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/iam/latest",
		ReleaseLevel:      "ga",
		LibraryType:       CoreLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/storage",
		Description:       "Cloud Storage (GCS)",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/storage/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/rpcreplay",
		Description:       "RPC Replay",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/latest/rpcreplay",
		ReleaseLevel:      "ga",
		LibraryType:       OtherLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/profiler",
		Description:       "Cloud Profiler",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/profiler/latest",
		ReleaseLevel:      "ga",
		LibraryType:       AgentLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/compute/metadata",
		Description:       "Service Metadata API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/compute/latest/metadata",
		ReleaseLevel:      "ga",
		LibraryType:       CoreLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/functions/metadata",
		Description:       "Cloud Functions",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/functions/latest/metadata",
		ReleaseLevel:      "alpha",
		LibraryType:       CoreLibraryType,
	},
	// Manuals with a GAPIC.
	{
		DistributionName:  "cloud.google.com/go/errorreporting",
		Description:       "Cloud Error Reporting API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/errorreporting/latest",
		ReleaseLevel:      "beta",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/firestore",
		Description:       "Cloud Firestore API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/firestore/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/logging",
		Description:       "Cloud Logging API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/logging/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/pubsub",
		Description:       "Cloud PubSub",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/pubsub/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/spanner",
		Description:       "Cloud Spanner",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/spanner/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
	{
		DistributionName:  "cloud.google.com/go/pubsublite",
		Description:       "Cloud PubSub Lite",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://cloud.google.com/go/docs/reference/cloud.google.com/go/pubsublite/latest",
		ReleaseLevel:      "ga",
		LibraryType:       GapicManualLibraryType,
	},
}
