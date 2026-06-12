package accelerator

import (
	"context"
	"testing"

	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNewAcceleratorChannel(t *testing.T) {
	channel := NewAcceleratorChannel()
	if channel == nil {
		t.Fatal("NewAcceleratorChannel returned nil")
	}
}

// Locks in the contract the future V2 shim depends on: until SessionPool's
// ReadRow/MutateRow stubs are replaced, the full translation pipeline
// runs and then surfaces codes.Unimplemented from the pool stub.
func TestInvoke_ReturnsUnimplemented(t *testing.T) {
	channel := NewAcceleratorChannel()
	ctx := context.Background()

	mutateReq := &v2pb.MutateRowRequest{
		TableName: "projects/p/instances/i/tables/t",
		RowKey:    []byte("k"),
	}
	err := channel.Invoke(ctx, "/google.bigtable.v2.Bigtable/MutateRow", mutateReq, &v2pb.MutateRowResponse{})
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("Invoke(MutateRow) code = %v; want Unimplemented", status.Code(err))
	}

	err = channel.Invoke(ctx, "unknown", nil, nil)
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("Invoke(unknown) code = %v; want Unimplemented", status.Code(err))
	}
}

func TestNewStream(t *testing.T) {
	channel := NewAcceleratorChannel()
	ctx := context.Background()

	stream, err := channel.NewStream(ctx, nil, "/google.bigtable.v2.Bigtable/ReadRows")
	if err != nil {
		t.Fatalf("NewStream(ReadRows) failed: %v", err)
	}
	if stream == nil {
		t.Fatal("NewStream(ReadRows) returned nil stream")
	}
	err = stream.RecvMsg(nil)
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("stream.RecvMsg() code = %v; want Unimplemented", status.Code(err))
	}

	_, err = channel.NewStream(ctx, nil, "unknown")
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("NewStream(unknown) code = %v; want Unimplemented", status.Code(err))
	}
}
