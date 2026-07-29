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

// Open opens a table. The returned *Table honors the Client's Diverter:
// when c.diverter is set, Apply and ReadRow are routed through an
// internal TableShim so calls can be diverted to the session data path
// under the diverter's SessionLoad ratio. Backward-compatible — the
// return type stays *Table and every existing method keeps its
// signature and behavior. sessionLoad=0.0 (the default at Client
// construction) means every call still lands on the classic path.
func (c *Client) Open(table string) *Table {
	t := &Table{
		c:     c,
		table: table,
		md: metadata.Join(metadata.Pairs(
			resourcePrefixHeader, c.fullTableName(table),
			requestParamsHeader, c.reqParamsHeaderValTable(table),
		), c.featureFlagsMD),
	}
	t.divertible = c.buildDivertible(t)
	return t
}

// OpenTable opens a table. Returns a TableShim that routes each RPC via
// the Client's Diverter — with sessionLoad=0.0 (the default) every call
// lands on the classic path. When a follow-up change wires in the
// session data path, callers get session routing without re-opening.
func (c *Client) OpenTable(table string) TableAPI {
	return c.buildDivertible(&Table{
		c:     c,
		table: table,
		md: metadata.Join(metadata.Pairs(
			resourcePrefixHeader, c.fullTableName(table),
			requestParamsHeader, c.reqParamsHeaderValTable(table),
		), c.featureFlagsMD),
	})
}

// OpenAuthorizedView opens an authorized view. See OpenTable for the
// diverter routing story.
func (c *Client) OpenAuthorizedView(table, authorizedView string) TableAPI {
	return c.buildDivertible(&Table{
		c:     c,
		table: table,
		md: metadata.Join(metadata.Pairs(
			resourcePrefixHeader, c.fullAuthorizedViewName(table, authorizedView),
			requestParamsHeader, c.reqParamsHeaderValTable(table),
		), c.featureFlagsMD),
		authorizedView: authorizedView,
	})
}

// OpenMaterializedView opens a materialized view. See OpenTable for the
// diverter routing story.
func (c *Client) OpenMaterializedView(materializedView string) TableAPI {
	return c.buildDivertible(&Table{
		c: c,
		md: metadata.Join(metadata.Pairs(
			resourcePrefixHeader, c.fullMaterializedViewName(materializedView),
			requestParamsHeader, c.reqParamsHeaderValTable(materializedView),
		), c.featureFlagsMD),
		materializedView: materializedView,
	})
}

// buildDivertible wraps a *Table's classic side in a TableShim so
// Apply / ReadRow can be routed to the session data path. Returns nil
// when the client has no Diverter (classic-only mode) so Open can
// assign the result straight into Table.divertible.
//
// The classic side is a *tableImpl over a snapshot of t with divertible
// EXPLICITLY nil-ed — that break in the loop is what prevents
// tableImpl.Apply/ReadRow from recursing back through the outer gate.
// The session TableAPI is nil today; a follow-up change wires it up
// from an internal session client. TableShim treats nil session as
// classic-only, so callers see no behavior change until that lands.
func (c *Client) buildDivertible(t *Table) TableAPI {
	if c.diverter == nil {
		return nil
	}
	inner := *t
	inner.divertible = nil
	return NewTableShim(&tableImpl{Table: inner}, nil, c.diverter)
}
