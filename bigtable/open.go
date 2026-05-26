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
	"context"
	"fmt"

	"google.golang.org/grpc/metadata"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	btransport "cloud.google.com/go/bigtable/internal/transport"
)

// Open opens a table.
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

// OpenTable opens a table.
func (c *Client) OpenTable(table string) TableAPI {
	classic := c.Open(table)
	readStreamFactory := func(ctx context.Context) (btransport.Stream, error) {
		if c.sessionClient != nil {
			return c.sessionClient.OpenTable(ctx)
		}
		return c.client.OpenTable(ctx)
	}
	writeStreamFactory := func(ctx context.Context) (btransport.Stream, error) {
		if c.sessionClient != nil {
			return c.sessionClient.OpenTable(ctx)
		}
		return c.client.OpenTable(ctx)
	}

	readTableReq := &btpb.OpenTableRequest{
		TableName:    c.fullTableName(table),
		AppProfileId: c.appProfile,
		Permission:   btpb.OpenTableRequest_PERMISSION_READ,
	}
	writeTableReq := &btpb.OpenTableRequest{
		TableName:    c.fullTableName(table),
		AppProfileId: c.appProfile,
		Permission:   btpb.OpenTableRequest_PERMISSION_WRITE,
	}

	return c.sessionMgr.GetOrCreateSessionTable(
		c.fullTableName(table),
		classic,
		btransport.TABLE_SESSION,
		readStreamFactory,
		writeStreamFactory,
		readTableReq,
		writeTableReq,
		btransport.READ_ROW,
		btransport.MUTATE_ROW,
		fmt.Sprintf("table:%s", table),
	)
}

// OpenAuthorizedView opens an authorized view.
func (c *Client) OpenAuthorizedView(table, authorizedView string) TableAPI {
	classic := &Table{
		c:     c,
		table: table,
		md: metadata.Join(metadata.Pairs(
			resourcePrefixHeader, c.fullAuthorizedViewName(table, authorizedView),
			requestParamsHeader, c.reqParamsHeaderValTable(table),
		), c.featureFlagsMD),
		authorizedView: authorizedView,
	}

	readStreamFactory := func(ctx context.Context) (btransport.Stream, error) {
		if c.sessionClient != nil {
			return c.sessionClient.OpenAuthorizedView(ctx)
		}
		return c.client.OpenAuthorizedView(ctx)
	}
	writeStreamFactory := func(ctx context.Context) (btransport.Stream, error) {
		if c.sessionClient != nil {
			return c.sessionClient.OpenAuthorizedView(ctx)
		}
		return c.client.OpenAuthorizedView(ctx)
	}

	readTableReq := &btpb.OpenAuthorizedViewRequest{
		AuthorizedViewName: c.fullAuthorizedViewName(table, authorizedView),
		AppProfileId:       c.appProfile,
		Permission:         btpb.OpenAuthorizedViewRequest_PERMISSION_READ,
	}
	writeTableReq := &btpb.OpenAuthorizedViewRequest{
		AuthorizedViewName: c.fullAuthorizedViewName(table, authorizedView),
		AppProfileId:       c.appProfile,
		Permission:         btpb.OpenAuthorizedViewRequest_PERMISSION_WRITE,
	}

	return c.sessionMgr.GetOrCreateSessionTable(
		c.fullAuthorizedViewName(table, authorizedView),
		classic,
		btransport.AUTHORIZED_VIEW_SESSION,
		readStreamFactory,
		writeStreamFactory,
		readTableReq,
		writeTableReq,
		btransport.READ_ROW_AUTH_VIEW,
		btransport.MUTATE_ROW_AUTH_VIEW,
		fmt.Sprintf("auth_view:%s:%s", table, authorizedView),
	)
}

// OpenMaterializedView opens a materialized view.
func (c *Client) OpenMaterializedView(materializedView string) TableAPI {
	classic := &Table{
		c: c,
		md: metadata.Join(metadata.Pairs(
			resourcePrefixHeader, c.fullMaterializedViewName(materializedView),
			requestParamsHeader, c.reqParamsHeaderValTable(materializedView),
		), c.featureFlagsMD),
		materializedView: materializedView,
	}

	readStreamFactory := func(ctx context.Context) (btransport.Stream, error) {
		if c.sessionClient != nil {
			return c.sessionClient.OpenMaterializedView(ctx)
		}
		return c.client.OpenMaterializedView(ctx)
	}

	readTableReq := &btpb.OpenMaterializedViewRequest{
		MaterializedViewName: c.fullMaterializedViewName(materializedView),
		AppProfileId:         c.appProfile,
	}

	return c.sessionMgr.GetOrCreateSessionTable(
		c.fullMaterializedViewName(materializedView),
		classic,
		btransport.MATERIALIZED_VIEW_SESSION,
		readStreamFactory,
		nil,
		readTableReq,
		nil,
		btransport.READ_ROW_MAT_VIEW,
		nil,
		fmt.Sprintf("mat_view:%s", materializedView),
	)
}
