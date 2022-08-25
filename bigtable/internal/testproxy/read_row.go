// Copyright 2022 Google LLC
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

package main

import (
	"context"
	"fmt"

	pb "github.com/googleapis/cloud-bigtable-clients-test/testproxypb"
	// btpb "google.golang.org/genproto/googleapis/bigtable/v2"
)

func (s *goTestProxyServer) ReadRow(ctx context.Context, req *pb.ReadRowRequest) (*pb.RowResult, error) {

	tName := req.TableName
	t := s.btClient.Open(tName)

	r, err := t.ReadRow(ctx, req.RowKey)

	if err != nil {
		return nil, err
	}

	if r != nil {
		return nil, fmt.Errorf("no error or row returned from ReadRow()")
	}

	// TODO(telpirion): translate Go client types to BT proto types
	res := &pb.RowResult{
		Row:    nil,
		Status: nil,
	}

	return res, nil
}
