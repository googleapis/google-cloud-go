package spanner

import (
	proto3 "github.com/golang/protobuf/ptypes/struct"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
)

// Fields returns raw grpc protobuf fields
func (r *Row) Fields() []*sppb.StructType_Field {
	return r.fields
}

// Values returns raw grpc protobuf row values
func (r *Row) Values() []*proto3.Value {
	return r.vals
}
