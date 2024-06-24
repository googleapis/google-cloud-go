/*
Copyright 2021 Google LLC

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

package bttest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAddLatency(t *testing.T) {
	var eib EmulatorInterceptor
	lt := latencyTarget{"MutateRows", 0, time.Duration(100 * time.Millisecond)}
	eib.LatencyTargets = append(eib.LatencyTargets, lt)

	ctx := context.Background()
	tbl, cleanup, err := setupFakeServer(eib.StreamInterceptor())
	if err != nil {
		t.Fatalf("fake server setup: %v", err)
	}
	defer cleanup()

	// Make sure we add latency to target Method (MutateRows)
	mutateRowsStartTime := time.Now()
	rowKeys, muts := getBulkRowKeysAndMuts(100)
	errs, err := tbl.ApplyBulk(ctx, rowKeys, muts)
	if err != nil || errs != nil {
		t.Fatal(err, errs)
	}
	actualMutateLatency := time.Now().Sub(mutateRowsStartTime)
	if actualMutateLatency < lt.expectedDuration {
		t.Errorf("Expected at least %q latency. Got %q", actualMutateLatency, lt.expectedDuration)
	}

	// Make sure we don't add latency to other Method (ReadRows)
	readRowsStartTime := time.Now()
	err = tbl.ReadRows(ctx, bigtable.PrefixRange("some-prefix"), readRowsNoop)
	if err != nil {
		t.Fatal(err)
	}
	actualReadLatency := time.Now().Sub(readRowsStartTime)
	if actualReadLatency > lt.expectedDuration {
		t.Errorf("Unexpected latency. Expected < %q. Got %q", lt.expectedDuration, actualReadLatency)
	}
}

func TestAddError(t *testing.T) {
	// Test triggering codes.FailedPrecondition (9) on 100% of ReadRows requests
	var eib EmulatorInterceptor
	err := eib.GrpcErrorCodeTargets.Set("ReadRows:100:9")
	if err != nil {
		t.Fatalf("Failed creating GrpcErrorCodeTargets: %v", err)
	}

	ctx := context.Background()
	tbl, cleanup, err := setupFakeServer(eib.StreamInterceptor())
	if err != nil {
		t.Fatalf("fake server setup: %v", err)
	}
	defer cleanup()

	// Test we add Errors to valid ReadRows request
	err = tbl.ReadRows(ctx, bigtable.PrefixRange("some-prefix"), readRowsNoop)
	if err == nil {
		t.Errorf("Expected error to be injected")
	}
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("Expected FailedPrecondition. Actual: %v", err)
	}

	// Test we add don't add Errors to MutateRows
	rowKeys, muts := getBulkRowKeysAndMuts(100)
	errs, err := tbl.ApplyBulk(ctx, rowKeys, muts)
	if err != nil || errs != nil {
		t.Errorf("Added error to MutateRows: %v %v", err, errs)
	}
}

func TestAddMultipleErrors(t *testing.T) {
	// Test failing 50/50 on FailedPrecondition (9) and NotFound (5)
	var eib EmulatorInterceptor
	args := []string{"ReadRows:50%:9", "ReadRows:50%:5"}
	for _, arg := range args {
		err := eib.GrpcErrorCodeTargets.Set(arg)
		if err != nil {
			t.Fatalf("Failed creating GrpcErrorCodeTargets: %v", err)
		}
	}

	ctx := context.Background()
	tbl, cleanup, err := setupFakeServer(eib.StreamInterceptor())
	if err != nil {
		t.Fatalf("fake server setup: %v", err)
	}
	defer cleanup()

	sawFailedPrecondition, sawNotFound := false, false
	for i := 0; i < 100; i++ {
		// Test we add Errors to valid ReadRows request
		err = tbl.ReadRows(ctx, bigtable.PrefixRange("some-prefix"), readRowsNoop)
		if status.Code(err) != codes.FailedPrecondition && status.Code(err) != codes.NotFound {
			t.Errorf("Expected FailedPrecondition or NotFound error. Actual: %v", err)
		}
		if status.Code(err) == codes.FailedPrecondition {
			sawFailedPrecondition = true
		}
		if status.Code(err) == codes.NotFound {
			sawNotFound = true
		}
	}
	if !sawFailedPrecondition || !sawNotFound {
		// Given 100 requests, there's a 10^-31 chance that both errors are not seen at least once
		t.Errorf("Expected to see both FailedPrecondition and NotFound errors")
	}

	// Test we still don't add Errors to MutateRows
	rowKeys, muts := getBulkRowKeysAndMuts(100)
	errs, err := tbl.ApplyBulk(ctx, rowKeys, muts)
	if err != nil || errs != nil {
		t.Errorf("Added error to MutateRows: %v %v", err, errs)
	}
}

func TestParseValidLatencyArgs(t *testing.T) {
	var eib EmulatorInterceptor
	tests := map[string]latencyTarget{
		"ReadRows:p0:0ms":      latencyTarget{"ReadRows", 0, time.Duration(0)},
		"ReadRows:p50:10ms":    latencyTarget{"ReadRows", 50, time.Duration(10 * time.Millisecond)},
		"ReadRows:p99.9:100ms": latencyTarget{"ReadRows", 99.9, time.Duration(100 * time.Millisecond)},
		"MutateRows:25:0s":     latencyTarget{"MutateRows", 25, time.Duration(0)},
		"MutateRows:75:88ms":   latencyTarget{"MutateRows", 75, time.Duration(88 * time.Millisecond)},
		"MutateRows:99:1s":     latencyTarget{"MutateRows", 99, time.Duration(1 * time.Second)},
	}
	for argString, expectedLatencyTarget := range tests {
		err := eib.LatencyTargets.Set(argString)
		if err != nil {
			t.Fatalf("failed to parse valid LatencyTarget %s\n%s", argString, err)
		}
		actualLatencyTarget := eib.LatencyTargets[len(eib.LatencyTargets)-1]
		if expectedLatencyTarget != actualLatencyTarget {
			t.Errorf("expected: %v, actual: %v", expectedLatencyTarget, actualLatencyTarget)
		}
	}
	if len(eib.LatencyTargets) != len(tests) {
		t.Errorf("Wrong number of LatencyTargets. expected: %d, actual: %d", len(eib.LatencyTargets), len(tests))
	}
}

func TestFailOnInvalidLatencyArgs(t *testing.T) {
	var eib EmulatorInterceptor
	tests := map[string]string{
		"Invalid Method":                "BadMethod:p50:10ms",
		"Invalid Percentile":            "ReadRows:BadPercentile:10ms",
		"Invalid (Negative) Percentile": "ReadRows:-1:10ms",
		"Invalid (>99) Percentile":      "MutateRows:100:10ms",
		"Invalid Duration":              "MutateRows:100:BadDuration",
	}
	for testName, argString := range tests {
		err := eib.LatencyTargets.Set(argString)
		if err == nil {
			t.Errorf("Expected to fail due to %s for %s", testName, argString)
		}
	}
	if len(eib.LatencyTargets) > 0 {
		t.Errorf("Expected 0 LatencyTargets from invalid args. Actual: %v", eib.LatencyTargets)
	}
}

func TestParseValidErrorArgs(t *testing.T) {
	var eib EmulatorInterceptor
	tests := []struct {
		argString      string
		expectedTarget grpcErrorCodeTarget
	}{
		{"ReadRows:1%:1", grpcErrorCodeTarget{"ReadRows", 1, 1, codes.Canceled}},
		{"ReadRows:2:2", grpcErrorCodeTarget{"ReadRows", 2, 3, codes.Unknown}},
		{"ReadRows:3:3", grpcErrorCodeTarget{"ReadRows", 3, 6, codes.InvalidArgument}},
		{"MutateRows:10%:14", grpcErrorCodeTarget{"MutateRows", 10, 10, codes.Unavailable}},
		{"MutateRows:20%:15", grpcErrorCodeTarget{"MutateRows", 20, 30, codes.DataLoss}},
		{"MutateRows:30:16", grpcErrorCodeTarget{"MutateRows", 30, 60, codes.Unauthenticated}},
	}
	for _, test := range tests {
		err := eib.GrpcErrorCodeTargets.Set(test.argString)
		if err != nil {
			t.Fatalf("failed to parse valid GrpcErrorCodeTargets %s\n%s", test.argString, err)
		}
		actualErrorTarget := eib.GrpcErrorCodeTargets[len(eib.GrpcErrorCodeTargets)-1]
		if test.expectedTarget != actualErrorTarget {
			t.Errorf("Set(%s): expected %v, actual: %v", test.argString, test.expectedTarget, actualErrorTarget)
		}
	}

}

func TestFailOnInvalidErrorArgs(t *testing.T) {
	var eib EmulatorInterceptor
	tests := map[string]string{
		"Invalid Method":                    "BadMethod:1%:1",
		"Invalid Error Rate":                "ReadRows::BadErrorRate:1",
		"Invalid (-1) Error Rate":           "ReadRows:-1:10ms",
		"Invalid (>100) Error Rate":         "MutateRows:101:10ms",
		"Invalid GrpcErrorCode":             "MutateRows:10%:BadCode",
		"Invalid (negative) GrpcErrorCode":  "MutateRows:10%:-1",
		"Invalid (undefined) GrpcErrorCode": "MutateRows:10%:9001",
	}
	for testName, argString := range tests {
		err := eib.GrpcErrorCodeTargets.Set(argString)
		if err == nil {
			t.Errorf("Set(%s): expected to fail due to %s", argString, testName)
		}
	}
	if len(eib.GrpcErrorCodeTargets) > 0 {
		t.Errorf("Expected 0 GrpcErrorCodeTargets from invalid args. Actual: %v", eib.GrpcErrorCodeTargets)
	}
}

func readRowsNoop(_ bigtable.Row) bool { return true }

func setupFakeServer(opt ...grpc.ServerOption) (tbl *bigtable.Table, cleanup func(), err error) {
	srv, err := NewServer("localhost:0", opt...)
	if err != nil {
		return nil, nil, err
	}
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, nil, err
	}

	client, err := bigtable.NewClient(context.Background(), "client", "instance", option.WithGRPCConn(conn), option.WithGRPCDialOption(grpc.WithBlock()))
	if err != nil {
		return nil, nil, err
	}

	adminClient, err := bigtable.NewAdminClient(context.Background(), "client", "instance", option.WithGRPCConn(conn), option.WithGRPCDialOption(grpc.WithBlock()))
	if err != nil {
		return nil, nil, err
	}
	if err := adminClient.CreateTable(context.Background(), "table"); err != nil {
		return nil, nil, err
	}
	if err := adminClient.CreateColumnFamily(context.Background(), "table", "cf"); err != nil {
		return nil, nil, err
	}
	t := client.Open("table")

	cleanupFunc := func() {
		adminClient.Close()
		client.Close()
		srv.Close()
	}
	return t, cleanupFunc, nil
}

func getBulkRowKeysAndMuts(count int) ([]string, []*bigtable.Mutation) {
	var rowKeys []string
	var muts []*bigtable.Mutation
	for i := 0; i < count; i++ {
		rowKeys = append(rowKeys, fmt.Sprintf("%d", i))
		mut := bigtable.NewMutation()
		mut.Set("cf", "col", bigtable.Now(), []byte("Gophers!"))
		muts = append(muts, mut)
	}
	return rowKeys, muts
}
