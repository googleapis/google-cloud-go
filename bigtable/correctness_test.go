package bigtable

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"testing"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// localMockServer implements btpb.BigtableServer for testing.
type localMockServer struct {
	btpb.UnimplementedBigtableServer
}

func (s *localMockServer) GetClientConfiguration(ctx context.Context, req *btpb.GetClientConfigurationRequest) (*btpb.ClientConfiguration, error) {
	return &btpb.ClientConfiguration{
		SessionConfiguration: &btpb.SessionClientConfiguration{
			SessionLoad: 1.0, // Enable sessions fully
		},
	}, nil
}

func (s *localMockServer) OpenTable(stream btpb.Bigtable_OpenTableServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	if req.GetOpenSession() == nil {
		return fmt.Errorf("expected OpenSession request")
	}

	resp := &btpb.SessionResponse{
		Payload: &btpb.SessionResponse_OpenSession{
			OpenSession: &btpb.OpenSessionResponse{},
		},
	}
	if err := stream.Send(resp); err != nil {
		return err
	}

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		vrpcReq := req.GetVirtualRpc()
		if vrpcReq != nil {
			tableResp := &btpb.TableResponse{
				Payload: &btpb.TableResponse_ReadRow{
					ReadRow: &btpb.SessionReadRowResponse{
						Row: &btpb.Row{
							Key: []byte("row1"),
							Families: []*btpb.Family{
								{
									Name: "fam1",
									Columns: []*btpb.Column{
										{
											Qualifier: []byte("col1"),
											Cells: []*btpb.Cell{
												{
													Value: []byte("val1"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			payloadBytes, err := proto.Marshal(tableResp)
			if err != nil {
				return err
			}
			resp := &btpb.SessionResponse{
				Payload: &btpb.SessionResponse_VirtualRpc{
					VirtualRpc: &btpb.VirtualRpcResponse{
						RpcId:   1,
						Payload: payloadBytes,
					},
				},
			}
			if err := stream.Send(resp); err != nil {
				return err
			}
		}
	}
}

// ReadRows implements the classic path for comparison.
func (s *localMockServer) ReadRows(req *btpb.ReadRowsRequest, stream btpb.Bigtable_ReadRowsServer) error {
	resp := &btpb.ReadRowsResponse{
		Chunks: []*btpb.ReadRowsResponse_CellChunk{
			{
				RowKey:     []byte("row1"),
				FamilyName: &wrapperspb.StringValue{Value: "fam1"},
				Qualifier:  &wrapperspb.BytesValue{Value: []byte("col1")},
				Value:      []byte("val1"),
				RowStatus:  &btpb.ReadRowsResponse_CellChunk_CommitRow{CommitRow: true},
			},
		},
	}
	return stream.Send(resp)
}

func (s *localMockServer) PingAndWarm(ctx context.Context, req *btpb.PingAndWarmRequest) (*btpb.PingAndWarmResponse, error) {
	return &btpb.PingAndWarmResponse{}, nil
}

func TestSessionReadRowCorrectness(t *testing.T) {
	ctx := context.Background()

	// Setup local mock server
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	mock := &localMockServer{}
	btpb.RegisterBigtableServer(srv, mock)
	go func() {
		srv.Serve(lis)
	}()
	defer srv.Stop()
	defer lis.Close()

	addr := lis.Addr().String()

	// Client 1: Classic (forced via env)
	os.Setenv("CBT_DISABLE_SESSIONS", "true")
	defer os.Unsetenv("CBT_DISABLE_SESSIONS")

	clientClassic, err := NewClient(ctx, "test-project", "test-instance", option.WithEndpoint(addr), option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("Failed to create classic client: %v", err)
	}
	defer clientClassic.Close()

	// Client 2: Session (default, should be enabled by mock config)
	os.Unsetenv("CBT_DISABLE_SESSIONS") // Ensure it's not disabled
	clientSession, err := NewClient(ctx, "test-project", "test-instance", option.WithEndpoint(addr), option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("Failed to create session client: %v", err)
	}
	defer clientSession.Close()

	// Call ReadRow on both
	tableClassic := clientClassic.Open("test-table")
	rowClassic, err := tableClassic.ReadRow(ctx, "row1")
	if err != nil {
		t.Fatalf("Classic ReadRow failed: %v", err)
	}

	tableSession := clientSession.Open("test-table")
	rowSession, err := tableSession.ReadRow(ctx, "row1")
	if err != nil {
		t.Fatalf("Session ReadRow failed: %v", err)
	}

	// Compare results
	if rowClassic == nil || rowSession == nil {
		t.Fatalf("One of the rows is nil: classic=%v, session=%v", rowClassic, rowSession)
	}

	if len(rowClassic) != len(rowSession) {
		t.Errorf("Row length mismatch: classic=%d, session=%d", len(rowClassic), len(rowSession))
	}

	// Deep comparison of families and cells
	for fam, itemsClassic := range rowClassic {
		itemsSession, ok := rowSession[fam]
		if !ok {
			t.Errorf("Family %s missing in session row", fam)
			continue
		}
		if len(itemsClassic) != len(itemsSession) {
			t.Errorf("Family %s length mismatch: classic=%d, session=%d", fam, len(itemsClassic), len(itemsSession))
			continue
		}
		for i := range itemsClassic {
			if itemsClassic[i].Row != itemsSession[i].Row {
				t.Errorf("Item %d Row mismatch: classic=%s, session=%s", i, itemsClassic[i].Row, itemsSession[i].Row)
			}
			if itemsClassic[i].Column != itemsSession[i].Column {
				t.Errorf("Item %d Column mismatch: classic=%s, session=%s", i, itemsClassic[i].Column, itemsSession[i].Column)
			}
			if string(itemsClassic[i].Value) != string(itemsSession[i].Value) {
				t.Errorf("Item %d Value mismatch: classic=%s, session=%s", i, string(itemsClassic[i].Value), string(itemsSession[i].Value))
			}
		}
	}
}
