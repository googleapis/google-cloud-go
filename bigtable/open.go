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

package bigtable

import "google.golang.org/grpc/metadata"

// Open opens a table for use with the classic data path helpers that
// still take *Table directly (BulkMutation, etc.). The returned *Table
// is always the classic implementation regardless of the Client's
// session-load ratio — callers who want the divertible surface should
// use OpenTable / OpenAuthorizedView / OpenMaterializedView.
func (c *Client) Open(table string) *Table {
	return &Table{
		c:     c,
		table: table,
		md: metadata.Join(metadata.Pairs(
			resourcePrefixHeader, c.fullTableName(table),
			requestParamsHeader, c.reqParamsHeaderValTable(table),
		), c.featureFlagsMD),
	}
}

// OpenTable opens a table. Returns a TableShim that routes each RPC via
// the Client's Diverter — with sessionLoad=0.0 (the default at
// construction time) every call lands on the classic path. When a
// follow-up change wires in the session data path, callers get session
// routing automatically without re-opening the table.
func (c *Client) OpenTable(table string) TableAPI {
	classic := &tableImpl{Table{
		c:     c,
		table: table,
		md: metadata.Join(metadata.Pairs(
			resourcePrefixHeader, c.fullTableName(table),
			requestParamsHeader, c.reqParamsHeaderValTable(table),
		), c.featureFlagsMD),
	}}
	return NewTableShim(classic, nil, c.diverter)
}

// OpenAuthorizedView opens an authorized view. See OpenTable for the
// diverter routing story.
func (c *Client) OpenAuthorizedView(table, authorizedView string) TableAPI {
	classic := &tableImpl{Table{
		c:     c,
		table: table,
		md: metadata.Join(metadata.Pairs(
			resourcePrefixHeader, c.fullAuthorizedViewName(table, authorizedView),
			requestParamsHeader, c.reqParamsHeaderValTable(table),
		), c.featureFlagsMD),
		authorizedView: authorizedView,
	}}
	return NewTableShim(classic, nil, c.diverter)
}

// OpenMaterializedView opens a materialized view. See OpenTable for the
// diverter routing story.
func (c *Client) OpenMaterializedView(materializedView string) TableAPI {
	classic := &tableImpl{Table{
		c: c,
		md: metadata.Join(metadata.Pairs(
			resourcePrefixHeader, c.fullMaterializedViewName(materializedView),
			requestParamsHeader, c.reqParamsHeaderValTable(materializedView),
		), c.featureFlagsMD),
		materializedView: materializedView,
	}}
	return NewTableShim(classic, nil, c.diverter)
}
