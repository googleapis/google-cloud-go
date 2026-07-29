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

import (
	"cloud.google.com/go/bigtable/internal/session"

	"google.golang.org/grpc/metadata"
)

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
// the Client's Diverter — with sessionLoad=0.0 every call lands on the
// classic path. The session TableAPI is wired from the Client's
// sessionImpl (always constructed by NewClientWithConfig); server-driven
// SessionLoad updates from ClientConfigurationManager retarget traffic
// without re-opening the table.
func (c *Client) OpenTable(table string) TableAPI {
	classic := &tableImpl{Table{
		c:     c,
		table: table,
		md: metadata.Join(metadata.Pairs(
			resourcePrefixHeader, c.fullTableName(table),
			requestParamsHeader, c.reqParamsHeaderValTable(table),
		), c.featureFlagsMD),
	}}
	return NewTableShim(classic, c.getOrCreateSessionTable(table), c.diverter)
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
	return NewTableShim(classic, c.getOrCreateSessionAuthorizedView(table, authorizedView), c.diverter)
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
	return NewTableShim(classic, c.getOrCreateSessionMaterializedView(materializedView), c.diverter)
}

// getOrCreateSessionTable returns a cached session TableAPI handle
// for this table. Returns nil when the session backend isn't wired
// (hand-built or emulator-only Clients where sessionImpl is nil, or
// sessionTables is nil); TableShim treats a nil session as
// classic-only. Cache key is "tbl:<table>".
//
// The cache exists because session.Client does not: each
// session.Client.OpenTable call would otherwise open a new pair of
// read/write session pools for the same resource. Handles evict
// after sessionTableCacheTTL of idle (default 1 h) or when the
// caller Close()s them explicitly. See session_table_cache.go.
func (c *Client) getOrCreateSessionTable(table string) session.TableAPI {
	if c.sessionImpl == nil {
		return nil
	}
	return c.sessionTables.getOrOpen("tbl:" + table)
}

// getOrCreateSessionAuthorizedView is the cache lookup for authorized
// views. Cache key is "av:<table>:<view>" (table-qualified so two AVs
// with the same view id on different tables get distinct session
// pools + distinct sessionz labels).
func (c *Client) getOrCreateSessionAuthorizedView(table, view string) session.TableAPI {
	if c.sessionImpl == nil {
		return nil
	}
	return c.sessionTables.getOrOpen("av:" + table + ":" + view)
}

// getOrCreateSessionMaterializedView is the cache lookup for
// materialized views. Cache key is "mv:<view>".
func (c *Client) getOrCreateSessionMaterializedView(view string) session.TableAPI {
	if c.sessionImpl == nil {
		return nil
	}
	return c.sessionTables.getOrOpen("mv:" + view)
}
