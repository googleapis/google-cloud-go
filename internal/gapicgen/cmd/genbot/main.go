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

//go:build !windows
// +build !windows

// genbot is a binary for generating gapics and creating CLs/PRs with the results.
// It is intended to be used as a bot, though it can be run locally too.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strconv"

	"cloud.google.com/go/internal/gapicgen"
)

func main() {
	log.SetFlags(0)
	if err := gapicgen.VerifyAllToolsExist([]string{"git", "go", "protoc"}); err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	// General bot flags
	githubAccessToken := flag.String("githubAccessToken", os.Getenv("GITHUB_ACCESS_TOKEN"), "The token used to open pull requests.")
	githubUsername := flag.String("githubUsername", os.Getenv("GITHUB_USERNAME"), "The GitHub user name for the author.")
	githubName := flag.String("githubName", os.Getenv("GITHUB_NAME"), "The name of the author for git commits.")
	githubEmail := flag.String("githubEmail", os.Getenv("GITHUB_EMAIL"), "The email address of the author.")
	localMode := flag.Bool("local", strToBool(os.Getenv("GENBOT_LOCAL_MODE")), "Enables generating sources locally. This mode will not open any pull requests.")
	forceAll := flag.Bool("forceAll", strToBool(os.Getenv("GENBOT_FORCE_ALL")), "Enables regenerating everything regardless of changes in googleapis.")

	// flags for local mode
	googleapisDir := flag.String("googleapis-dir", os.Getenv("GOOGLEAPIS_DIR"), "Directory where sources of googleapis/googleapis resides. If unset the sources will be cloned to a temporary directory that is not cleaned up.")
	gocloudDir := flag.String("gocloud-dir", os.Getenv("GOCLOUD_DIR"), "Directory where sources of googleapis/google-cloud-go resides. If unset the sources will be cloned to a temporary directory that is not cleaned up.")
	genprotoDir := flag.String("genproto-dir", os.Getenv("GENPROTO_DIR"), "Directory where sources of googleapis/go-genproto resides. If unset the sources will be cloned to a temporary directory that is not cleaned up.")
	protoDir := flag.String("proto-dir", os.Getenv("PROTO_DIR"), "Directory where sources of google/protobuf resides. If unset the sources will be cloned to a temporary directory that is not cleaned up.")
	gapicToGenerate := flag.String("gapic", os.Getenv("GAPIC_TO_GENERATE"), `Specifies which gapic to generate. The value should be in the form of an import path (Ex: cloud.google.com/go/pubsub/apiv1). The default "" generates all gapics.`)
	onlyGapics := flag.Bool("only-gapics", strToBool(os.Getenv("ONLY_GAPICS")), "Enabling stops regenerating genproto.")
	regenOnly := flag.Bool("regen-only", strToBool(os.Getenv("REGEN_ONLY")), "Enabling means no vetting, manifest updates, or compilation.")
	genModule := flag.Bool("generate-module", strToBool(os.Getenv("GENERATE_MODULE")), "Enabling means a new module will be generated for API being generated.")
	genAlias := flag.Bool("generate-alias", strToBool(os.Getenv("GENERATE_ALIAS")), "Enabling means alias files will be generated.")

	flag.Parse()

	if *localMode {
		if err := genLocal(ctx, localConfig{
			googleapisDir:   *googleapisDir,
			gocloudDir:      *gocloudDir,
			genprotoDir:     *genprotoDir,
			protoDir:        *protoDir,
			gapicToGenerate: *gapicToGenerate,
			onlyGapics:      *onlyGapics,
			regenOnly:       *regenOnly,
			forceAll:        *forceAll,
			genModule:       *genModule,
			genAlias:        *genAlias,
		}); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := genBot(ctx, botConfig{
		githubAccessToken: *githubAccessToken,
		githubUsername:    *githubUsername,
		githubName:        *githubName,
		githubEmail:       *githubEmail,
		forceAll:          *forceAll,
	}); err != nil {
		log.Fatal(err)
	}
}

func strToBool(s string) bool {
	// Treat error as false
	b, _ := strconv.ParseBool(s)
	return b
}
