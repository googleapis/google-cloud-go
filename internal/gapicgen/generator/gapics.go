// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

// generateGapics generates gapics.
func generateGapics(ctx context.Context, googleapisDir, protoDir, gocloudDir, genprotoDir string) error {
	for _, c := range microgenGapicConfigs {
		if err := microgen(c, googleapisDir, protoDir, gocloudDir); err != nil {
			return err
		}
	}

	if err := copyMicrogenFiles(gocloudDir); err != nil {
		return err
	}

	if err := manifest(microgenGapicConfigs, googleapisDir, gocloudDir); err != nil {
		return err
	}

	if err := setVersion(gocloudDir); err != nil {
		return err
	}

	for _, m := range gapicsWithManual {
		if err := setGoogleClientInfo(gocloudDir + "/" + m); err != nil {
			return err
		}
	}

	if err := addModReplaceGenproto(gocloudDir, genprotoDir); err != nil {
		return err
	}

	if err := vet(gocloudDir); err != nil {
		return err
	}

	if err := build(gocloudDir); err != nil {
		return err
	}

	if err := dropModReplaceGenproto(gocloudDir); err != nil {
		return err
	}

	return nil
}

// addModReplaceGenproto adds a genproto replace statement that points genproto
// to the local copy. This is necessary since the remote genproto may not have
// changes that are necessary for the in-flight regen.
func addModReplaceGenproto(gocloudDir, genprotoDir string) error {
	c := command("bash", "-c", `
set -ex

GENPROTO_VERSION=$(cat go.mod | cat go.mod | grep genproto | awk '{print $2}')
go mod edit -replace "google.golang.org/genproto@$GENPROTO_VERSION=$GENPROTO_DIR"
`)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = gocloudDir
	c.Env = []string{
		"GENPROTO_DIR=" + genprotoDir,
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	return c.Run()
}

// dropModReplaceGenproto drops the genproto replace statement. It is intended
// to be run after addModReplaceGenproto.
func dropModReplaceGenproto(gocloudDir string) error {
	c := command("bash", "-c", `
set -ex

GENPROTO_VERSION=$(cat go.mod | cat go.mod | grep genproto | grep -v replace | awk '{print $2}')
go mod edit -dropreplace "google.golang.org/genproto@$GENPROTO_VERSION"
`)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = gocloudDir
	c.Env = []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
		fmt.Sprintf("HOME=%s", os.Getenv("HOME")), // TODO(deklerk): Why do we need to do this? Doesn't seem to be necessary in other exec.Commands.
	}
	return c.Run()
}

// setGoogleClientInfo enters a directory and updates setGoogleClientInfo
// to be public. It is used for gapics which have manuals that use them, since
// the manual needs to call this function.
func setGoogleClientInfo(manualDir string) error {
	// TODO(deklerk): Migrate this all to Go instead of using bash.

	c := command("bash", "-c", `
find . -name '*.go' -exec sed -i.backup -e 's/setGoogleClientInfo/SetGoogleClientInfo/g' '{}' '+'
find . -name '*.backup' -delete
`)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = manualDir
	return c.Run()
}

// setVersion updates the versionClient constant in all .go files. It may create
// .backup files on certain systems (darwin), and so should be followed by a
// clean-up of .backup files.
func setVersion(gocloudDir string) error {
	// TODO(deklerk): Migrate this all to Go instead of using bash.

	c := command("bash", "-c", `
ver=$(date +%Y%m%d)
git ls-files -mo | while read modified; do
	dir=${modified%/*.*}
	find . -path "*/$dir/doc.go" -exec sed -i.backup -e "s/^const versionClient.*/const versionClient = \"$ver\"/" '{}' +;
done
find . -name '*.backup' -delete
`)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = gocloudDir
	return c.Run()
}

// microgen runs the microgenerator on a single microgen config.
func microgen(conf *microgenConfig, googleapisDir, protoDir, gocloudDir string) error {
	log.Println("microgen generating", conf.pkg)

	var protoFiles []string
	if err := filepath.Walk(googleapisDir+"/"+conf.inputDirectoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(info.Name(), ".proto") {
			protoFiles = append(protoFiles, path)
		}
		return nil
	}); err != nil {
		return err
	}

	args := []string{"-I", googleapisDir,
		"-I", protoDir,
		"--go_gapic_out", gocloudDir,
		"--go_gapic_opt", fmt.Sprintf("go-gapic-package=%s;%s", conf.importPath, conf.pkg),
		"--go_gapic_opt", fmt.Sprintf("grpc-service-config=%s", conf.gRPCServiceConfigPath),
		"--go_gapic_opt", fmt.Sprintf("gapic-service-config=%s", conf.apiServiceConfigPath),
		"--go_gapic_opt", fmt.Sprintf("release-level=%s", conf.releaseLevel)}
	args = append(args, protoFiles...)
	c := command("protoc", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = googleapisDir
	return c.Run()
}

// manifestEntry is used for JSON marshaling in manifest.
type manifestEntry struct {
	DistributionName  string `json:"distribution_name"`
	Description       string `json:"description"`
	Language          string `json:"language"`
	ClientLibraryType string `json:"client_library_type"`
	DocsURL           string `json:"docs_url"`
	ReleaseLevel      string `json:"release_level"`
}

// TODO: consider getting Description from the gapic, if there is one.
var manualEntries = []manifestEntry{
	// Pure manual clients.
	{
		DistributionName:  "cloud.google.com/go/bigquery",
		Description:       "BigQuery",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/bigquery",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/bigtable",
		Description:       "Cloud BigTable",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/bigtable",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/datastore",
		Description:       "Cloud Datastore",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/datastore",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/iam",
		Description:       "Cloud IAM",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/iam",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/storage",
		Description:       "Cloud Storage (GCS)",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/storage",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/rpcreplay",
		Description:       "RPC Replay",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/rpcreplay",
		ReleaseLevel:      "ga",
	},
	// Manuals with a GAPIC.
	{
		DistributionName:  "cloud.google.com/go/errorreporting",
		Description:       "Stackdriver Error Reporting API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/errorreporting",
		ReleaseLevel:      "beta",
	},
	{
		DistributionName:  "cloud.google.com/go/firestore",
		Description:       "Cloud Firestore API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/firestore",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/logging",
		Description:       "Stackdriver Logging API",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/logging",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/pubsub",
		Description:       "Cloud PubSub",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/pubsub",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/spanner",
		Description:       "Cloud Spanner",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/spanner",
		ReleaseLevel:      "ga",
	},
	{
		DistributionName:  "cloud.google.com/go/trace",
		Description:       "Stackdriver Trace",
		Language:          "Go",
		ClientLibraryType: "manual",
		DocsURL:           "https://pkg.go.dev/cloud.google.com/go/trace",
		ReleaseLevel:      "ga",
	},
}

// manifest writes a manifest file with info about all of the confs.
func manifest(confs []*microgenConfig, googleapisDir, gocloudDir string) error {
	entries := map[string]manifestEntry{} // Key is the package name.
	f, err := os.Create(filepath.Join(gocloudDir, "internal", ".repo-metadata-full.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	for _, manual := range manualEntries {
		entries[manual.DistributionName] = manual
	}
	for _, conf := range confs {
		yamlPath := filepath.Join(googleapisDir, conf.apiServiceConfigPath)
		yamlFile, err := os.Open(yamlPath)
		if err != nil {
			return err
		}
		yamlConfig := struct {
			Title string `yaml:"title"` // We only need the title field.
		}{}
		if err := yaml.NewDecoder(yamlFile).Decode(&yamlConfig); err != nil {
			return fmt.Errorf("Decode: %v", err)
		}
		entry := manifestEntry{
			DistributionName:  conf.importPath,
			Description:       yamlConfig.Title,
			Language:          "Go",
			ClientLibraryType: "generated",
			DocsURL:           "https://pkg.go.dev/" + conf.importPath,
			ReleaseLevel:      conf.releaseLevel,
		}
		entries[conf.importPath] = entry
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

// copyMicrogenFiles takes microgen files from gocloudDir/cloud.google.com/go
// and places them in gocloudDir.
func copyMicrogenFiles(gocloudDir string) error {
	// The period at the end is analagous to * (copy everything in this dir).
	c := command("cp", "-R", gocloudDir+"/cloud.google.com/go/.", ".")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = gocloudDir
	if err := c.Run(); err != nil {
		return err
	}

	c = command("rm", "-rf", "cloud.google.com")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = gocloudDir
	return c.Run()
}
