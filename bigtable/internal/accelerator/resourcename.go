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
// A Channel serves exactly one (project, instance). The session layer
// re-derives the full resource name from that scope given only a leaf ID, so
// it never sees — and cannot check — the project/instance a caller named. If
// the channel simply stripped a full V2 name down to its leaf, a request
// naming a different project/instance would be silently rebound onto this
// daemon's scope (a cross-tenant confused-deputy). These parsers therefore
// validate the project/instance segments against the channel's scope and
// reject a mismatch, returning the leaf ID(s) only for an in-scope name.

// scopePrefixFor builds the "projects/<project>/instances/<instance>/" prefix a
// Channel validates and strips resource names against. Precomputed once at
// construction and stored on Channel.scopePrefix.
func scopePrefixFor(project, instance string) string {
	return "projects/" + project + "/instances/" + instance + "/"
}

// stripScope verifies name begins with this daemon's precomputed
// "projects/<project>/instances/<instance>/" prefix and returns the remainder
// after it. A name targeting any other project/instance is rejected with
// InvalidArgument rather than silently rebound onto this daemon's scope.
func (c *Channel) stripScope(name string) (string, error) {
	rest, ok := strings.CutPrefix(name, c.scopePrefix)
	if !ok {
		return "", status.Errorf(codes.InvalidArgument,
			"accelerator: resource %q is not in this daemon's scope (%s)",
			name, strings.TrimSuffix(c.scopePrefix, "/"))
	}
	return rest, nil
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
// (project, instance) and returns the table leaf ID.
func (c *Channel) parseTableName(name string) (tableID string, err error) {
	rest, err := c.stripScope(name)
	if err != nil {
		return "", err
	}
	return leafAfter(name, rest, "tables/")
}

// parseAuthorizedViewName validates that name is an authorized-view resource in
// this daemon's (project, instance) and returns the table and view leaf IDs.
func (c *Channel) parseAuthorizedViewName(name string) (tableID, viewID string, err error) {
	rest, err := c.stripScope(name)
	if err != nil {
		return "", "", err
	}
	afterTables, ok := strings.CutPrefix(rest, "tables/")
	if !ok {
		return "", "", malformedResource(name)
	}
	tableID, viewID, ok = strings.Cut(afterTables, "/authorizedViews/")
	if !ok || tableID == "" || viewID == "" ||
		strings.Contains(tableID, "/") || strings.Contains(viewID, "/") {
		return "", "", malformedResource(name)
	}
	return tableID, viewID, nil
}

// parseMaterializedViewName validates that name is a materialized-view resource
// in this daemon's (project, instance) and returns the view leaf ID.
func (c *Channel) parseMaterializedViewName(name string) (viewID string, err error) {
	rest, err := c.stripScope(name)
	if err != nil {
		return "", err
	}
	return leafAfter(name, rest, "materializedViews/")
}
