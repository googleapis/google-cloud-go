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

package accelerator

import (
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Resource-name parsing for the channel's dispatch path.
//
// A Channel serves one GCP project (and optionally a pinned instance). The
// session layer re-derives the full resource name from a (project, instance)
// scope given only a leaf ID, so it never sees — and cannot check — the
// project/instance a caller named. If the channel simply stripped a full V2
// name down to its leaf, a request naming a different project (or a different
// instance when pinned) would be silently rebound onto this daemon's scope (a
// cross-tenant confused-deputy). These parsers therefore validate the project
// (and optional instance) segments against the channel's scope and reject a
// mismatch, returning the instance ID and leaf ID(s) only for an in-scope name.

// scopePrefixFor builds the "projects/<project>/instances/<instance>/" prefix
// when instance is non-empty, or "projects/<project>/instances/" when the
// daemon is project-scoped. Precomputed once at construction and stored on
// Channel.scopePrefix.
func scopePrefixFor(project, instance string) string {
	if instance == "" {
		return "projects/" + project + "/instances/"
	}
	return "projects/" + project + "/instances/" + instance + "/"
}

// stripScope verifies name begins with this daemon's precomputed project
// prefix (and pinned instance, if any) and returns the resolved instance ID
// and the remainder after "projects/<project>/instances/<instance>/". A name
// targeting any other project (or another instance when pinned) is rejected
// with InvalidArgument rather than silently rebound onto this daemon's scope.
func (c *Channel) stripScope(name string) (instanceID, rest string, err error) {
	if c == nil || c.scopePrefix == "" {
		return "", "", status.Errorf(codes.InvalidArgument,
			"accelerator: resource %q is not in this daemon's scope ()", name)
	}
	if c.instance != "" {
		rest, ok := strings.CutPrefix(name, c.scopePrefix)
		if !ok {
			return "", "", status.Errorf(codes.InvalidArgument,
				"accelerator: resource %q is not in this daemon's scope (%s)",
				name, strings.TrimSuffix(c.scopePrefix, "/"))
		}
		return c.instance, rest, nil
	}
	afterProject, ok := strings.CutPrefix(name, c.scopePrefix)
	if !ok {
		return "", "", status.Errorf(codes.InvalidArgument,
			"accelerator: resource %q is not in this daemon's scope (%s)",
			name, strings.TrimSuffix(strings.TrimSuffix(c.scopePrefix, "instances/"), "/"))
	}
	instanceID, rest, ok = strings.Cut(afterProject, "/")
	if !ok || instanceID == "" {
		return "", "", malformedResource(name)
	}
	return instanceID, rest, nil
}

// leafAfter returns the single path segment following seg at the front of
// rest, requiring exactly one non-empty segment with no further "/". For
// example leafAfter("tables/t", "tables/") returns "t".
func leafAfter(name, rest, seg string) (string, error) {
	v, ok := strings.CutPrefix(rest, seg)
	if !ok || v == "" || strings.Contains(v, "/") {
		return "", malformedResource(name)
	}
	return v, nil
}

func malformedResource(name string) error {
	return status.Errorf(codes.InvalidArgument, "accelerator: malformed resource name %q", name)
}

// parseTableName validates that name is a table resource in this daemon's
// scope and returns the instance ID and table leaf ID.
func (c *Channel) parseTableName(name string) (instanceID, tableID string, err error) {
	instanceID, rest, err := c.stripScope(name)
	if err != nil {
		return "", "", err
	}
	tableID, err = leafAfter(name, rest, "tables/")
	if err != nil {
		return "", "", err
	}
	return instanceID, tableID, nil
}

// parseAuthorizedViewName validates that name is an authorized-view resource in
// this daemon's scope and returns the instance ID, table ID, and view leaf IDs.
func (c *Channel) parseAuthorizedViewName(name string) (instanceID, tableID, viewID string, err error) {
	instanceID, rest, err := c.stripScope(name)
	if err != nil {
		return "", "", "", err
	}
	afterTables, ok := strings.CutPrefix(rest, "tables/")
	if !ok {
		return "", "", "", malformedResource(name)
	}
	tableID, viewID, ok = strings.Cut(afterTables, "/authorizedViews/")
	if !ok || tableID == "" || viewID == "" ||
		strings.Contains(tableID, "/") || strings.Contains(viewID, "/") {
		return "", "", "", malformedResource(name)
	}
	return instanceID, tableID, viewID, nil
}

// parseMaterializedViewName validates that name is a materialized-view resource
// in this daemon's scope and returns the instance ID and view leaf ID.
func (c *Channel) parseMaterializedViewName(name string) (instanceID, viewID string, err error) {
	instanceID, rest, err := c.stripScope(name)
	if err != nil {
		return "", "", err
	}
	viewID, err = leafAfter(name, rest, "materializedViews/")
	if err != nil {
		return "", "", err
	}
	return instanceID, viewID, nil
}
