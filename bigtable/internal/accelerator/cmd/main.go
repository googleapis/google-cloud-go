// Copyright 2026 Google LLC
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

// Binary accelerator runs the in-process gRPC proxy daemon: it listens on a
// Unix domain socket, accepts standard google.bigtable.v2.Bigtable RPCs, and
// forwards each one through an Channel that rides the Jetstream
// session transport.
//
// One daemon serves one (project, instance, appProfile) tuple. Cross-language
// clients spawn the binary, wait for the UDS to become connectable, then talk
// standard Bigtable V2 protos over it.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/bigtable/internal/accelerator"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

// identityResolveTimeout bounds the pre-bind identity resolution so a hung
// metadata server (GCE/GKE) can never stall daemon startup. On timeout the
// principal is left unresolved and the client falls back to its native path.
const identityResolveTimeout = 15 * time.Second

func main() {
	udsPath := flag.String("uds-path", "", "path to unix domain socket (required)")
	project := flag.String("project", "", "GCP project ID (required)")
	instance := flag.String("instance", "", "Bigtable instance ID (required)")
	appProfile := flag.String("app-profile", "", "Bigtable app profile (optional)")
	// Endpoint overrides. When empty, NewChannel falls back to the
	// default Bigtable data-plane endpoint (bigtable.googleapis.com:443). The
	// spawning client supplies these when it targets a non-default endpoint or a
	// non-GDU universe.
	dataEndpoint := flag.String("data-endpoint", "", "override Bigtable data-plane endpoint, e.g. bigtable.googleapis.com:443 (optional)")
	universeDomain := flag.String("universe-domain", "", "override the service universe domain, e.g. googleapis.com (optional)")
	scopesFlag := flag.String("scopes", "", "comma-separated OAuth scopes to override the default data scope (optional)")
	quotaProject := flag.String("quota-project", "", "quota/billing project override (optional)")
	flag.Parse()

	if *udsPath == "" {
		log.Fatal("Missing required flag: --uds-path")
	}
	if *project == "" {
		log.Fatal("Missing required flag: --project")
	}
	if *instance == "" {
		log.Fatal("Missing required flag: --instance")
	}

	// Only forward overrides that were actually set; leaving them unset lets
	// the default endpoint/universe-domain resolution apply.
	var opts []option.ClientOption
	if *dataEndpoint != "" {
		opts = append(opts, option.WithEndpoint(*dataEndpoint))
	}
	if *universeDomain != "" {
		opts = append(opts, option.WithUniverseDomain(*universeDomain))
	}
	// Caller opts are appended after the channel's defaults, so WithScopes here
	// overrides the hardcoded data scope.
	scopes := splitScopes(*scopesFlag)
	if len(scopes) > 0 {
		opts = append(opts, option.WithScopes(scopes...))
	}
	if *quotaProject != "" {
		opts = append(opts, option.WithQuotaProject(*quotaProject))
	}

	ctx := context.Background()

	// Resolve ADC once, using the effective scopes the channel will dial with,
	// then hand the resolved credentials to both the channel (via
	// WithCredentials) and the identity resolver below. This avoids a second
	// FindDefaultCredentials lookup in identity resolution and guarantees the
	// published principal matches the credentials the channel actually dials.
	effectiveScopes := scopes
	if len(effectiveScopes) == 0 {
		effectiveScopes = []string{accelerator.DataScope}
	}
	creds, err := google.FindDefaultCredentials(ctx, effectiveScopes...)
	if err != nil {
		log.Fatalf("failed to resolve credentials: %v", err)
	}
	// Appended last so it overrides the channel's default credential resolution;
	// the dial can no longer re-resolve to a different principal than the one we
	// publish in identity.json.
	opts = append(opts, option.WithCredentials(creds))

	channel, err := accelerator.NewChannel(ctx, *project, *instance, *appProfile, opts...)
	if err != nil {
		log.Fatalf("failed to construct accelerator channel: %v", err)
	}
	srv := accelerator.NewServer(*udsPath, channel)
	log.Printf("Starting accelerator daemon on UDS=%s project=%s instance=%s app-profile=%q data-endpoint=%q universe-domain=%q scopes=%q quota-project=%q",
		*udsPath, *project, *instance, *appProfile, *dataEndpoint, *universeDomain, *scopesFlag, *quotaProject)

	// Resolve and publish the daemon's identity BEFORE binding the socket, so
	// the file is present by the time the client detects a connectable UDS.
	// Non-fatal on failure: the client's verify step falls back to native. The
	// resolution is bound by a timeout so a hung metadata server can't stall
	// startup.
	identityCtx, cancelIdentity := context.WithTimeout(ctx, identityResolveTimeout)
	principal, err := principalFromCreds(identityCtx, creds)
	if err != nil {
		log.Printf("warning: principal resolution failed: %v", err)
	}
	cancelIdentity()
	// Record effectiveScopes (not the raw override), so identity.json reflects
	// the scope the daemon actually dials with rather than an empty list when no
	// --scopes flag was passed.
	if err := writeIdentity(*udsPath, principal, effectiveScopes); err != nil {
		log.Printf("warning: failed to write identity document: %v", err)
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("failed to start accelerator server: %v", err)
	}
	defer srv.Stop()

	// Block until either the watchdog (stdin EOF or parent-PID change) trips or
	// the process receives a termination signal, then shut down cleanly.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	select {
	case <-srv.ShutdownChan():
		log.Printf("accelerator: watchdog-initiated shutdown, exiting...")
	case s := <-sigCh:
		log.Printf("accelerator: received %s, shutting down...", s)
	}
}

// splitScopes parses a comma-separated scopes flag into a trimmed, non-empty
// slice. Returns an empty (non-nil) slice when the flag is empty or blank, so
// callers get a consistent slice value and it marshals to [] rather than null.
func splitScopes(raw string) []string {
	scopes := []string{}
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			scopes = append(scopes, s)
		}
	}
	return scopes
}
