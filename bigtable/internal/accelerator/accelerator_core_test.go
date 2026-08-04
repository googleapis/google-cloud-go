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
	"context"
	"testing"

	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/session"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockSessionTableAPI is a session.TableAPI stub the tests hand back
// from mockSessionClient.OpenTable.
type mockSessionTableAPI struct {
	mutateRowFn func(ctx context.Context, req *v2pb.SessionMutateRowRequest) (*v2pb.SessionMutateRowResponse, error)
	readRowFn   func(ctx context.Context, req *v2pb.SessionReadRowRequest) (*v2pb.SessionReadRowResponse, error)
}

func (m *mockSessionTableAPI) ReadRow(ctx context.Context, req *v2pb.SessionReadRowRequest) (*v2pb.SessionReadRowResponse, error) {
	if m.readRowFn != nil {
		return m.readRowFn(ctx, req)
	}
	return &v2pb.SessionReadRowResponse{}, nil
}

func (m *mockSessionTableAPI) MutateRow(ctx context.Context, req *v2pb.SessionMutateRowRequest) (*v2pb.SessionMutateRowResponse, error) {
	if m.mutateRowFn != nil {
		return m.mutateRowFn(ctx, req)
	}
	return &v2pb.SessionMutateRowResponse{}, nil
}

func (m *mockSessionTableAPI) Close() error { return nil }

// mockSessionClient hands back a fixed SessionTableApi for any resource and
// records the leaf arguments each Open* method was called with, plus a per-kind
// open counter, so tests can assert dispatch routing.
type mockSessionClient struct {
	table          session.TableAPI
	lastTableName  string
	newTableCalled int

	lastAVTable string
	lastAVView  string
	newAVCalled int
	lastMVView  string
	newMVCalled int
}

func (m *mockSessionClient) OpenTable(name string) session.TableAPI {
	m.lastTableName = name
	m.newTableCalled++
	return m.table
}

func (m *mockSessionClient) OpenAuthorizedView(table, view string) session.TableAPI {
	m.lastAVTable, m.lastAVView = table, view
	m.newAVCalled++
	return m.table
}

func (m *mockSessionClient) OpenMaterializedView(view string) session.TableAPI {
	m.lastMVView = view
	m.newMVCalled++
	return m.table
}

func (m *mockSessionClient) MeterProvider() metric.MeterProvider { return nil }

func (m *mockSessionClient) SessionDebug() btransport.SessionDebugProvider { return nil }
func (m *mockSessionClient) ChannelDebug() btransport.ChannelDebugProvider { return nil }
func (m *mockSessionClient) ConfigDebug() btransport.ConfigDebugProvider   { return nil }

func (m *mockSessionClient) AddSessionLoadListener(func(float64)) func() { return func() {} }

func (m *mockSessionClient) Close() error { return nil }

// stubSessionClient swaps the package-level newSessionClient seam so NewChannel
// builds a Channel backed by the given mock. Restores via t.Cleanup. Must NOT be
// used with t.Parallel — the seam is package-level state.
func stubSessionClient(t *testing.T, sc *mockSessionClient) {
	t.Helper()
	orig := newSessionClient
	newSessionClient = func(
		_ context.Context,
		_, _, _ string,
		_ ...option.ClientOption,
	) (session.Client, error) {
		return sc, nil
	}
	t.Cleanup(func() { newSessionClient = orig })
}

// newTestChannel is a convenience for tests: stubs the SessionClient factory
// and builds an Channel against the given mock.
func newTestChannel(t *testing.T, sc *mockSessionClient) *Channel {
	t.Helper()
	stubSessionClient(t, sc)
	channel, err := NewChannel(context.Background(), "p", "i", "ap")
	if err != nil {
		t.Fatalf("NewChannel error: %v", err)
	}
	return channel
}

func TestNewChannel_Constructs(t *testing.T) {
	channel := newTestChannel(t, &mockSessionClient{table: &mockSessionTableAPI{}})
	if channel.sc == nil {
		t.Error("Channel.sc is nil after construction")
	}
}

func TestInvoke_UnknownMethod_ReturnsUnimplemented(t *testing.T) {
	channel := newTestChannel(t, &mockSessionClient{table: &mockSessionTableAPI{}})

	err := channel.Invoke(context.Background(), "/google.bigtable.v2.Bigtable/SampleRowKeys", nil, nil)
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("Invoke(unknown) code = %v; want Unimplemented", status.Code(err))
	}
}

func TestNewStream_UnknownMethod_ReturnsUnimplemented(t *testing.T) {
	channel := newTestChannel(t, &mockSessionClient{table: &mockSessionTableAPI{}})

	_, err := channel.NewStream(context.Background(), nil, "/google.bigtable.v2.Bigtable/SampleRowKeys")
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("NewStream(unknown) code = %v; want Unimplemented", status.Code(err))
	}
}
