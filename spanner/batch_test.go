/*
Copyright 2018 Google Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"testing"
	"time"

	sppb "google.golang.org/genproto/googleapis/spanner/v1"
)

// serdesPartition is a helper that serialize a Partition then deserialize it
func serdesPartition(t *testing.T, i int64, p1 *Partition) (p2 Partition) {
	var (
		data []byte
		err  error
	)
	if data, err = p1.MarshalBinary(); err != nil {
		t.Fatalf("#%d: encoding failed %v", i, err)
	}
	if err = p2.UnmarshalBinary(data); err != nil {
		t.Fatalf("#%d: decoding failed %v", i, err)
	}
	return p2
}

// Test serdes of Partition.
func TestPartitionSerdes(t *testing.T) {
	t.Parallel()
	var (
		p1, p2 Partition
		rreq   = &sppb.ReadRequest{}
		qreq   = &sppb.ExecuteSqlRequest{}
		seed   = time.Now().UnixNano()
	)
	p1.rreq = rreq
	p2 = serdesPartition(t, seed, &p1)
	if !testEqual(p1, p2) {
		t.Errorf("Seed:%d, serdes of read Partition failed, \ngot: %#v, \nwant:%#v", seed, p2, p1)
	}
	p1.rreq = nil
	p1.qreq = qreq
	p2 = serdesPartition(t, seed, &p1)
	if !testEqual(p1, p2) {
		t.Errorf("Seed:%d, serdes of query Partition failed, \ngot: %#v, \nwant:%#v", seed, p2, p1)
	}
}
