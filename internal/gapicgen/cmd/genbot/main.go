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
	"errors"
	"flag"
	"log"
	"os"
	"strconv"

	"cloud.google.com/go/internal/gapicgen"
	"cloud.google.com/go/internal/gapicgen/generator"
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
	genprotoDir := flag.String("genproto-dir", os.Getenv("GENPROTO_DIR"), "Directory where sources of googleapis/go-genproto resides. If unset the sources will be cloned to a temporary directory that is not cleaned up.")
	protoDir := flag.String("proto-dir", os.Getenv("PROTO_DIR"), "Directory where sources of google/protobuf resides. If unset the sources will be cloned to a temporary directory that is not cleaned up.")
	regenOnly := flag.Bool("regen-only", strToBool(os.Getenv("REGEN_ONLY")), "Enabling means no vetting, manifest updates, or compilation.")
	genAlias := flag.Bool("generate-alias", strToBool(os.Getenv("GENERATE_ALIAS")), "Enabling means alias files will be generated.")

	flag.Parse()

	if *localMode {
		if err := genLocal(ctx, localConfig{
			googleapisDir: *googleapisDir,
			genprotoDir:   *genprotoDir,
			protoDir:      *protoDir,
			regenOnly:     *regenOnly,
			forceAll:      *forceAll,
			genAlias:      *genAlias,
		}); err != nil {
			if errors.Is(err, generator.ErrNoProcessing) {
				log.Println(err)
				os.Exit(0)
				return
			}
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
		if errors.Is(err, generator.ErrNoProcessing) {
			log.Println(err)
			os.Exit(0)
			return
		}
		log.Fatal(err)
	}
}

func strToBool(s string) bool {
	// Treat error as false
	b, _ := strconv.ParseBool(s)
	return b
}
