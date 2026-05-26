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
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	btransport "cloud.google.com/go/bigtable/internal/transport"
)

// SessionTable implements TableAPI by routing calls via virtual RPCs through dedicated session pools.
type SessionTable struct {
	tableName     string
	classic       TableAPI
	readPool      *btransport.SessionPoolImpl
	writePool     *btransport.SessionPoolImpl
	readVRpcDesc  btransport.VRpcDescriptor
	writeVRpcDesc btransport.VRpcDescriptor
}

// NewSessionTable creates a new SessionTable instance.
func NewSessionTable(
	tableName string,
	classic TableAPI,
	readPool *btransport.SessionPoolImpl,
	writePool *btransport.SessionPoolImpl,
	readVRpcDesc btransport.VRpcDescriptor,
	writeVRpcDesc btransport.VRpcDescriptor,
) *SessionTable {
	return &SessionTable{
		tableName:     tableName,
		classic:       classic,
		readPool:      readPool,
		writePool:     writePool,
		readVRpcDesc:  readVRpcDesc,
		writeVRpcDesc: writeVRpcDesc,
	}
}

// ReadRow reads a single row via vRPC.
func (t *SessionTable) ReadRow(ctx context.Context, row string, opts ...ReadOption) (Row, error) {
	if t.readPool == nil {
		return t.classic.ReadRow(ctx, row, opts...)
	}

	req := &btpb.ReadRowsRequest{
		TableName: t.tableName,
		Rows: &btpb.RowSet{
			RowKeys: [][]byte{[]byte(row)},
		},
	}
	settings := makeReadSettings(req, 0)
	for _, opt := range opts {
		opt.set(&settings)
	}

	fmt.Printf(">>> SessionTable ReadRow: row=%s via readPool=%p <<<\n", row, t.readPool)

	retryInterceptor := btransport.RetryingVRpc(btransport.RetryingOptions{
		MaxAttempts:       10, // Up to 10 attempts (initial attempt + 9 retries)
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 1.5,
	})

	args := btransport.ReadRowArgs{
		RowKey: row,
		Filter: req.Filter,
	}

	baseHandler := func(attemptCtx context.Context, request interface{}) (interface{}, error) {
		resp, clusterInfo, err := t.readPool.ExecuteVRpc(attemptCtx, t.readVRpcDesc, request)
		if err != nil {
			return nil, err
		}
		if clusterInfo != nil {
			fmt.Printf(">>> SessionTable ReadRow attempt served by Cluster: Id=%s, Zone=%s <<<\n", clusterInfo.ClusterId, clusterInfo.ZoneId)
		}
		return resp, nil
	}

	chained := btransport.ChainInterceptors(retryInterceptor)
	res, err := chained(ctx, args, baseHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to execute ReadRow vRPC: %w", err)
	}

	readResult, ok := res.(btransport.ReadRowResult)
	if !ok {
		return nil, fmt.Errorf("unexpected response type from vRPC: %T", res)
	}

	return protoRowToRow(readResult.Row), nil
}

// Apply applies a single mutation via vRPC.
func (t *SessionTable) Apply(ctx context.Context, row string, m *Mutation, opts ...ApplyOption) error {
	if t.writePool == nil || m.isConditional {
		return t.classic.Apply(ctx, row, m, opts...)
	}

	fmt.Printf(">>> SessionTable Apply: row=%s via writePool=%p <<<\n", row, t.writePool)

	retryInterceptor := btransport.RetryingVRpc(btransport.RetryingOptions{
		MaxAttempts:       10, // Up to 10 attempts
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 1.5,
	})

	args := btransport.MutateRowArgs{
		RowKey:    row,
		Mutations: m.ops,
	}

	baseHandler := func(attemptCtx context.Context, request interface{}) (interface{}, error) {
		resp, clusterInfo, err := t.writePool.ExecuteVRpc(attemptCtx, t.writeVRpcDesc, request)
		if err != nil {
			return nil, err
		}
		if clusterInfo != nil {
			fmt.Printf(">>> SessionTable Apply attempt served by Cluster: Id=%s, Zone=%s <<<\n", clusterInfo.ClusterId, clusterInfo.ZoneId)
		}
		return resp, nil
	}

	chained := btransport.ChainInterceptors(retryInterceptor)
	_, err := chained(ctx, args, baseHandler)
	if err != nil {
		return fmt.Errorf("failed to execute MutateRow vRPC: %w", err)
	}

	return nil
}

// ReadRows delegates to classic TableAPI.
func (t *SessionTable) ReadRows(ctx context.Context, arg RowSet, f func(Row) bool, opts ...ReadOption) error {
	return t.classic.ReadRows(ctx, arg, f, opts...)
}

// SampleRowKeys delegates to classic TableAPI.
func (t *SessionTable) SampleRowKeys(ctx context.Context) ([]string, error) {
	return t.classic.SampleRowKeys(ctx)
}

// ApplyBulk delegates to classic TableAPI.
func (t *SessionTable) ApplyBulk(ctx context.Context, rowKeys []string, muts []*Mutation, opts ...ApplyOption) ([]error, error) {
	return t.classic.ApplyBulk(ctx, rowKeys, muts, opts...)
}

// ApplyReadModifyWrite delegates to classic TableAPI.
func (t *SessionTable) ApplyReadModifyWrite(ctx context.Context, row string, m *ReadModifyWrite) (Row, error) {
	return t.classic.ApplyReadModifyWrite(ctx, row, m)
}

func protoRowToRow(pr *btpb.Row) Row {
	if pr == nil {
		return nil
	}
	rowMap := make(Row)
	rowKey := string(pr.Key)
	for _, fam := range pr.Families {
		familyName := fam.Name
		for _, col := range fam.Columns {
			columnName := familyName + ":" + string(col.Qualifier)
			var items []ReadItem
			for _, cell := range col.Cells {
				items = append(items, ReadItem{
					Row:       rowKey,
					Column:    columnName,
					Timestamp: Timestamp(cell.TimestampMicros),
					Value:     cell.Value,
					Labels:    cell.Labels,
				})
			}
			if len(items) > 0 {
				rowMap[familyName] = append(rowMap[familyName], items...)
			}
		}
	}
	return rowMap
}
