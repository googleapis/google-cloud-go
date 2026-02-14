// Copyright 2014 Google LLC
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

package datastore

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	pb "cloud.google.com/go/datastore/apiv1/datastorepb"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"cloud.google.com/go/rpcreplay"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// TODO(djd): Make test entity clean up more robust: some test entities may
// be left behind if tests are aborted, the transport fails, etc.

var timeNow = time.Now()

// suffix is a timestamp-based suffix which is appended to key names,
// particularly for the root keys of entity groups. This reduces flakiness
// when the tests are run in parallel.
var suffix string

const (
	replayFilename = "datastore.replay"
	envDatabases   = "GCLOUD_TESTS_GOLANG_DATASTORE_DATABASES"
	keyPrefix      = "TestIntegration_"
	// readTimeConsistencyBuffer is a buffer duration to ensure the ReadTime timestamp
	// is not in the future relative to the backend's clock.
	readTimeConsistencyBuffer = 100 * time.Millisecond
)

type replayInfo struct {
	ProjectID string
	Time      time.Time
}

var (
	record = flag.Bool("record", false, "record RPCs")

	newTestClient = func(ctx context.Context, t *testing.T, opts ...option.ClientOption) *Client {
		return newClient(ctx, t, nil, opts...)
	}
	testParams map[string]interface{}

	// xGoogReqParamsHeaderChecker is a HeaderChecker that ensures that the "x-goog-request-params"
	// header is present on outgoing metadata.
	xGoogReqParamsHeaderChecker *testutil.HeaderChecker
)

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	flag.Parse()
	if testing.Short() {
		if *record {
			log.Fatal("cannot combine -short and -record")
		}
		if testutil.CanReplay(replayFilename) {
			initReplay()
		}
	} else if *record {
		if testutil.ProjID() == "" {
			log.Fatal("must record with a project ID")
		}
		b, err := json.Marshal(replayInfo{
			ProjectID: testutil.ProjID(),
			Time:      timeNow,
		})
		if err != nil {
			log.Fatal(err)
		}
		rec, err := rpcreplay.NewRecorder(replayFilename, b)
		if err != nil {
			log.Fatal(err)
		}
		defer func() {
			if err := rec.Close(); err != nil {
				log.Fatalf("closing recorder: %v", err)
			}
		}()
		newTestClient = func(ctx context.Context, t *testing.T, opts ...option.ClientOption) *Client {
			return newClient(ctx, t, rec.DialOptions(), opts...)
		}
		log.Printf("recording to %s", replayFilename)
	}
	suffix = fmt.Sprintf("-t%d", timeNow.UnixNano())

	// Run tests on multiple databases
	databaseIDs := []string{DefaultDatabaseID}
	databasesStr, ok := os.LookupEnv(envDatabases)
	if ok {
		databaseIDs = append(databaseIDs, strings.Split(databasesStr, ",")...)
	}

	testParams = make(map[string]interface{})
	for _, databaseID := range databaseIDs {
		log.Printf("Setting up tests to run on databaseID: %q\n", databaseID)
		testParams["databaseID"] = databaseID
		xGoogReqParamsHeaderChecker = &testutil.HeaderChecker{
			Key: reqParamsHeader,
			ValuesValidator: func(values ...string) error {
				if len(values) == 0 {
					return fmt.Errorf("missing values")
				}
				wantValue := fmt.Sprintf("project_id=%s", url.QueryEscape(testutil.ProjID()))
				if databaseID != DefaultDatabaseID && databaseID != "" {
					wantValue = fmt.Sprintf("%s&database_id=%s", wantValue, url.QueryEscape(databaseID))
				}
				for _, gotValue := range values {
					if gotValue != wantValue {
						return fmt.Errorf("got %s, want %s", gotValue, wantValue)
					}
				}
				return nil
			},
		}
		status := m.Run()
		if status != 0 {
			return status
		}
	}

	return 0
}

func initReplay() {
	rep, err := rpcreplay.NewReplayer(replayFilename)
	if err != nil {
		log.Fatal(err)
	}
	defer rep.Close()

	var ri replayInfo
	if err := json.Unmarshal(rep.Initial(), &ri); err != nil {
		log.Fatalf("unmarshaling initial replay info: %v", err)
	}
	timeNow = ri.Time.In(time.Local)

	conn, err := rep.Connection()
	if err != nil {
		log.Fatal(err)
	}

	newTestClient = func(ctx context.Context, t *testing.T, opts ...option.ClientOption) *Client {
		grpcHeadersEnforcer := &testutil.HeadersEnforcer{
			OnFailure: t.Fatalf,
			Checkers: []*testutil.HeaderChecker{
				testutil.XGoogClientHeaderChecker,
				xGoogReqParamsHeaderChecker,
			},
		}

		opts = append(opts, grpcHeadersEnforcer.CallOptions()...)
		opts = append(opts, option.WithGRPCConn(conn))
		client, err := NewClientWithDatabase(ctx, ri.ProjectID, testParams["databaseID"].(string), opts...)
		if err != nil {
			t.Fatalf("NewClientWithDatabase: %v", err)
		}
		return client
	}
	log.Printf("replaying from %s", replayFilename)
}

func newClient(ctx context.Context, t *testing.T, dialOpts []grpc.DialOption, opts ...option.ClientOption) *Client {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ts := testutil.TokenSource(ctx, ScopeDatastore)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}

	grpcHeadersEnforcer := &testutil.HeadersEnforcer{
		OnFailure: t.Fatalf,
		Checkers: []*testutil.HeaderChecker{
			testutil.XGoogClientHeaderChecker,
			xGoogReqParamsHeaderChecker,
		},
	}
	opts = append(opts, grpcHeadersEnforcer.CallOptions()...)
	opts = append(opts, option.WithTokenSource(ts))
	for _, opt := range dialOpts {
		opts = append(opts, option.WithGRPCDialOption(opt))
	}
	client, err := NewClientWithDatabase(ctx, testutil.ProjID(), testParams["databaseID"].(string), opts...)
	if err != nil {
		t.Fatalf("NewClientWithDatabase: %v", err)
	}
	return client
}

func TestIntegration_NewClient(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()
	client, err := NewClient(ctx, testutil.ProjID())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.databaseID != DefaultDatabaseID {
		t.Fatalf("NewClient: got %s, want %s", client.databaseID, DefaultDatabaseID)
	}
}

func TestIntegration_Basics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()
	client := newTestClient(ctx, t)
	defer client.Close()

	type X struct {
		I int
		S string
		T time.Time
		U interface{}
	}

	x0 := X{66, "99", timeNow.Truncate(time.Millisecond), "X"}
	k, err := client.Put(ctx, IncompleteKey("BasicsX", nil), &x0)
	if err != nil {
		t.Fatalf("client.Put: %v", err)
	}
	x1 := X{}
	err = client.Get(ctx, k, &x1)
	if err != nil {
		t.Errorf("client.Get: %v", err)
	}
	err = client.Delete(ctx, k)
	if err != nil {
		t.Errorf("client.Delete: %v", err)
	}
	if !testutil.Equal(x0, x1) {
		t.Errorf("compare: x0=%v, x1=%v", x0, x1)
	}
}

type OldX struct {
	I int
	J int
}
type NewX struct {
	I int
	j int
}

func TestIntegration_UpsertWithPropertyMask(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type Item struct {
		Count       int
		Name        string
		Description string
	}

	t.Run("EmptyMask_TransformOnly", func(t *testing.T) {
		key := NameKey("UpsertMask", "item1"+suffix, nil)
		initial := &Item{Count: 1, Name: "Initial", Description: "Desc"}
		if _, err := client.Put(ctx, key, initial); err != nil {
			t.Fatalf("client.Put: %v", err)
		}
		defer client.Delete(ctx, key)

		// Update ONLY the count (via transform) and implicitly preserve "Name" and "Description".
		mut := NewUpsert(key, &Item{}).WithTransforms(Increment("Count", 5)).WithPropertyMask()
		if _, err := client.Mutate(ctx, mut); err != nil {
			t.Fatalf("client.Mutate: %v", err)
		}

		var got Item
		if err := client.Get(ctx, key, &got); err != nil {
			t.Fatalf("client.Get: %v", err)
		}

		if got.Name != "Initial" {
			t.Errorf("Name mismatch: got %q, want %q", got.Name, "Initial")
		}
		if got.Description != "Desc" {
			t.Errorf("Description mismatch: got %q, want %q", got.Description, "Desc")
		}
		if got.Count != 6 {
			t.Errorf("Count mismatch: got %d, want 6", got.Count)
		}
	})

	t.Run("SpecificMask_PartialUpdate", func(t *testing.T) {
		key := NameKey("UpsertMask", "item2"+suffix, nil)
		initial := &Item{Count: 10, Name: "Initial", Description: "InitialDesc"}
		if _, err := client.Put(ctx, key, initial); err != nil {
			t.Fatalf("client.Put: %v", err)
		}
		defer client.Delete(ctx, key)

		// Update "Name" from payload, increment "Count", preserve "Description".
		// Payload has "Name" = "NewName", "Description" = "NewDesc" (should be ignored).
		updatePayload := &Item{Name: "NewName", Description: "ShouldBeIgnored"}

		mut := NewUpsert(key, updatePayload).
			WithTransforms(Increment("Count", 1)).
			WithPropertyMask("Name") // Only "Name" should be taken from payload

		if _, err := client.Mutate(ctx, mut); err != nil {
			t.Fatalf("client.Mutate: %v", err)
		}

		var got Item
		if err := client.Get(ctx, key, &got); err != nil {
			t.Fatalf("client.Get: %v", err)
		}

		if got.Name != "NewName" {
			t.Errorf("Name mismatch: got %q, want %q", got.Name, "NewName")
		}
		if got.Description != "InitialDesc" {
			t.Errorf("Description mismatch: got %q, want %q", got.Description, "InitialDesc")
		}
		if got.Count != 11 {
			t.Errorf("Count mismatch: got %d, want 11", got.Count)
		}
	})
}

func TestIntegration_IgnoreFieldMismatch(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t, WithIgnoreFieldMismatch())
	t.Cleanup(func() {
		client.Close()
	})

	// Save entities with an extra field
	kind := "IgnoreFieldMismatchX" + suffix
	keys := []*Key{
		NameKey(kind, "x1"+suffix, nil),
		NameKey(kind, "x2"+suffix, nil),
	}

	entitiesOld := []OldX{
		{I: 10, J: 20},
		{I: 30, J: 40},
	}
	_, gotErr := client.PutMulti(ctx, keys, entitiesOld)
	if gotErr != nil {
		t.Fatalf("Failed to save: %v\n", gotErr)
	}

	var wants []NewX
	for _, oldX := range entitiesOld {
		wants = append(wants, []NewX{{I: oldX.I}}...)
	}

	t.Cleanup(func() {
		client.DeleteMulti(ctx, keys)
	})

	tests := []struct {
		desc    string
		client  *Client
		wantErr error
	}{
		{
			desc:   "Without IgnoreFieldMismatch option",
			client: newTestClient(ctx, t),
			wantErr: &ErrFieldMismatch{
				StructType: reflect.TypeOf(NewX{}),
				FieldName:  "J",
				Reason:     "no such struct field",
			},
		},
		{
			desc:   "With IgnoreFieldMismatch option",
			client: newTestClient(ctx, t, WithIgnoreFieldMismatch()),
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			defer test.client.Close()
			// FieldMismatch error in Next
			query := NewQuery(kind).FilterField("I", ">=", 10)

			it := test.client.Run(ctx, query)
			resIndex := 0
			for {
				var newX NewX
				_, err := it.Next(&newX)
				if err == iterator.Done {
					break
				}
				if resIndex >= len(wants) {
					t.Errorf("Received more results than expected: got index %d, want length %d", resIndex, len(wants))
					break
				}
				compareIgnoreFieldMismatchResults(t, []NewX{wants[resIndex]}, []NewX{newX}, test.wantErr, err, "Next")
				resIndex++
			}

			// FieldMismatch error in Get
			var getX NewX
			gotErr = test.client.Get(ctx, keys[0], &getX)
			compareIgnoreFieldMismatchResults(t, []NewX{wants[0]}, []NewX{getX}, test.wantErr, gotErr, "Get")

			// FieldMismatch error in GetAll
			var getAllX []NewX
			_, gotErr = test.client.GetAll(ctx, query, &getAllX)
			compareIgnoreFieldMismatchResults(t, wants, getAllX, test.wantErr, gotErr, "GetAll")

			// FieldMismatch error in GetMulti
			getMultiX := make([]NewX, len(keys))
			gotErr = test.client.GetMulti(ctx, keys, getMultiX)
			compareIgnoreFieldMismatchResults(t, wants, getMultiX, test.wantErr, gotErr, "GetMulti")

			tx, err := test.client.NewTransaction(ctx)
			if err != nil {
				t.Fatalf("tx.GetMulti got: %v, want: nil\n", err)
			}

			// FieldMismatch error in tx.Get
			var txGetX NewX
			err = tx.Get(keys[0], &txGetX)
			compareIgnoreFieldMismatchResults(t, []NewX{wants[0]}, []NewX{txGetX}, test.wantErr, err, "tx.Get")

			// FieldMismatch error in tx.GetMulti
			txGetMultiX := make([]NewX, len(keys))
			err = tx.GetMulti(keys, txGetMultiX)
			compareIgnoreFieldMismatchResults(t, wants, txGetMultiX, test.wantErr, err, "tx.GetMulti")

			tx.Commit()

		})
	}

}

func compareIgnoreFieldMismatchResults(t *testing.T, wantX []NewX, gotX []NewX, wantErr error, gotErr error, errPrefix string) {
	if !equalErrs(gotErr, wantErr) {
		t.Errorf("%v: error got: %v, want: %v", errPrefix, gotErr, wantErr)
	}
	if len(gotX) != len(wantX) {
		t.Fatalf("%v results length: got: %v, want: %v\n", errPrefix, len(gotX), len(wantX))
	}
	for resIndex := 0; resIndex < len(wantX) && gotErr == nil; resIndex++ {
		if wantX[resIndex].I != gotX[resIndex].I {
			t.Fatalf("%v %v: got: %v, want: %v\n", errPrefix, resIndex, wantX[resIndex].I, gotX[resIndex].I)
		}
	}
}

func TestIntegration_GetWithReadTime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	client := newTestClient(ctx, t)
	defer cancel()
	defer client.Close()

	type RT struct {
		TimeCreated time.Time
	}

	rt1 := RT{time.Now()}
	k := NameKey("RT", "ReadTime", nil)

	tx, err := client.NewTransaction(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := tx.Put(k, &rt1); err != nil {
		t.Fatalf("Transaction.Put: %v\n", err)
	}

	if _, err := tx.Commit(); err != nil {
		t.Fatalf("Transaction.Commit: %v\n", err)
	}

	testutil.Retry(t, 5, time.Duration(10*time.Second), func(r *testutil.R) {
		got := RT{}
		tm := ReadTime(time.Now())

		client.WithReadOptions(tm)

		newCtx, cancel := context.WithTimeout(context.Background(), time.Second*20)
		newClient := newTestClient(newCtx, t)
		defer cancel()
		defer newClient.Close()

		// If the Entity isn't available at the requested read time, we get
		// a "datastore: no such entity" error. The ReadTime is otherwise not
		// exposed in anyway in the response.
		err = newClient.Get(newCtx, k, &got)
		if err != nil {
			r.Errorf("newClient.Get: %v", err)
		}
	})

	// Cleanup
	_ = client.Delete(ctx, k)
}

func TestIntegration_RunWithReadTime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	client := newTestClient(ctx, t)
	defer cancel()
	defer client.Close()

	type RT struct {
		TimeCreated time.Time
	}

	rt1 := RT{time.Now()}
	k := NameKey("RT", "ReadTime", nil)

	tx, err := client.NewTransaction(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := tx.Put(k, &rt1); err != nil {
		t.Fatalf("Transaction.Put: %v\n", err)
	}

	if _, err := tx.Commit(); err != nil {
		t.Fatalf("Transaction.Commit: %v\n", err)
	}

	testutil.Retry(t, 5, time.Duration(10*time.Second), func(r *testutil.R) {
		got := RT{}
		time.Sleep(readTimeConsistencyBuffer)
		tm := ReadTime(time.Now().Truncate(time.Microsecond))

		runCtx, cancel := context.WithTimeout(context.Background(), time.Second*20)
		runClient := newTestClient(runCtx, t)
		defer cancel()
		defer runClient.Close()

		runClient.WithReadOptions(tm)

		// If the Entity isn't available at the requested read time, we get
		// a "datastore: no such entity" error. The ReadTime is otherwise not
		// exposed in anyway in the response.
		err = runClient.Get(runCtx, k, &got)
		if err != nil {
			r.Errorf("client.Get: %v", err)
		}

		it := runClient.Run(runCtx, NewQuery("RT"))
		_, err = it.Next(nil)
		if err != nil && err != iterator.Done {
			r.Errorf("client.Run: %v", err)
		}
	})

	// Cleanup
	_ = client.Delete(ctx, k)
}

func TestIntegration_TopLevelKeyLoaded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()
	client := newTestClient(ctx, t)
	defer client.Close()

	completeKey := NameKey("EntityWithKey", "myent", nil)

	type EntityWithKey struct {
		I int
		S string
		K *Key `datastore:"__key__"`
	}

	in := &EntityWithKey{
		I: 12,
		S: "abcd",
	}

	defer func() {
		if err := client.Delete(ctx, completeKey); err != nil {
			t.Helper()
			t.Errorf("client.Delete for key %v: %v", completeKey, err)
		}
	}()
	k, err := client.Put(ctx, completeKey, in)
	if err != nil {
		t.Fatalf("client.Put: %v", err)
	}

	testutil.Retry(t, 10, 10*time.Second, func(r *testutil.R) {
		var e EntityWithKey
		err = client.Get(ctx, k, &e)
		if err != nil {
			r.Errorf("client.Get: %v", err)
			return
		}

		// The two keys should be absolutely identical.
		if !testutil.Equal(e.K, k) {
			r.Errorf("e.K not equal to k; got %#v, want %#v", e.K, k)
		}
	})

}

func TestIntegration_ListValues(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	p0 := PropertyList{
		{Name: "L", Value: []interface{}{int64(12), "string", true}},
	}
	k, err := client.Put(ctx, IncompleteKey("ListValue", nil), &p0)
	if err != nil {
		t.Fatalf("client.Put: %v", err)
	}
	var p1 PropertyList
	if err := client.Get(ctx, k, &p1); err != nil {
		t.Errorf("client.Get: %v", err)
	}
	if !testutil.Equal(p0, p1) {
		t.Errorf("compare:\np0=%v\np1=%#v", p0, p1)
	}
	if err = client.Delete(ctx, k); err != nil {
		t.Errorf("client.Delete: %v", err)
	}
}

func TestIntegration_PutGetUntypedNil(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type X struct {
		I interface{} `json:"i"`
	}

	putX := &X{
		I: nil,
	}
	key, err := client.Put(ctx, IncompleteKey("X", nil), putX)
	if err != nil {
		t.Fatalf("client.Put got: %v, want: nil", err)
	}
	defer func() {
		if err := client.Delete(ctx, key); err != nil {
			t.Helper()
			t.Errorf("client.Delete for key %v: %v", key, err)
		}
	}()

	getX := &X{}
	err = client.Get(ctx, key, getX)
	if err != nil {
		t.Fatalf("client.Get got: %v, want: nil", err)
	}

	if !reflect.DeepEqual(getX, putX) {
		t.Fatalf("got: %v, want: %v", getX, putX)
	}
}

func TestIntegration_GetMulti(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type X struct {
		I int
	}
	p := NameKey("X", "x"+suffix, nil)

	cases := []struct {
		desc string
		key  *Key
		put  bool
		x    *X
	}{
		{desc: "Successful get", key: NameKey("X", "item1", p), put: true, x: &X{I: 1}},
		{desc: "No such entity error", key: NameKey("X", "item2", p), put: false},
		{desc: "No such entity error", key: NameKey("X", "item3", p), put: false},
		{desc: "Duplicate keys in GetMulti with no such entity error", key: NameKey("X", "item3", p), put: false},
		{desc: "First key in the pair of keys with same Kind and Name but different Namespace", key: &Key{Kind: "X", Name: "item5", Namespace: "nm1"}, put: true, x: &X{I: 5}},
		{desc: "Second key in the pair of keys with same Kind and Name but different Namespace", key: &Key{Kind: "X", Name: "item5", Namespace: "nm2"}, put: true, x: &X{I: 6}},
	}

	var src, dst, wantDst []*X
	var srcKeys, dstKeys []*Key
	for _, c := range cases {
		dst = append(dst, &X{})
		dstKeys = append(dstKeys, c.key)
		wantDst = append(wantDst, c.x)
		if c.put {
			src = append(src, c.x)
			srcKeys = append(srcKeys, c.key)
		}
	}
	if _, err := client.PutMulti(ctx, srcKeys, src); err != nil {
		t.Error(err)
	}
	defer func() {
		if err := client.DeleteMulti(ctx, srcKeys); err != nil {
			t.Helper()
			t.Errorf("client.DeleteMulti for keys %v: %v", srcKeys, err)
		}
	}()

	err := client.GetMulti(ctx, dstKeys, dst)
	if err == nil {
		t.Errorf("client.GetMulti got %v, expected error", err)
	}
	e, ok := err.(MultiError)
	if !ok {
		t.Errorf("client.GetMulti got %T, expected MultiError", err)
	}
	for i, err := range e {
		got, want := err, (error)(nil)
		if !cases[i].put {
			got, want = err, ErrNoSuchEntity
		}
		if got != want {
			t.Errorf("%s: MultiError[%d] == %v, want %v", cases[i].desc, i, got, want)
		}

		if got == nil && *dst[i] != *wantDst[i] {
			t.Errorf("%s: client.GetMulti got %+v, want %+v", cases[i].desc, dst[i], wantDst[i])
		}
	}
}

type Z struct {
	S string
	T string `datastore:",noindex"`
	P []byte
	K []byte `datastore:",noindex"`
}

func (z Z) String() string {
	var lens []string
	v := reflect.ValueOf(z)
	for i := 0; i < v.NumField(); i++ {
		if l := v.Field(i).Len(); l > 0 {
			lens = append(lens, fmt.Sprintf("len(%s)=%d", v.Type().Field(i).Name, l))
		}
	}
	return fmt.Sprintf("Z{ %s }", strings.Join(lens, ","))
}

func TestIntegration_UnindexableValues(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	x1500 := strings.Repeat("x", 1500)
	x1501 := strings.Repeat("x", 1501)
	testCases := []struct {
		in      Z
		wantErr bool
	}{
		{in: Z{S: x1500}, wantErr: false},
		{in: Z{S: x1501}, wantErr: true},
		{in: Z{T: x1500}, wantErr: false},
		{in: Z{T: x1501}, wantErr: false},
		{in: Z{P: []byte(x1500)}, wantErr: false},
		{in: Z{P: []byte(x1501)}, wantErr: true},
		{in: Z{K: []byte(x1500)}, wantErr: false},
		{in: Z{K: []byte(x1501)}, wantErr: false},
	}
	for _, tt := range testCases {
		gotKey, err := client.Put(ctx, IncompleteKey("BasicsZ", nil), &tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("client.Put %s got err %v, want err %t", tt.in, err, tt.wantErr)
		}
		if !tt.wantErr {
			defer func() {
				if err := client.Delete(ctx, gotKey); err != nil {
					t.Helper()
					t.Errorf("client.Delete for key %v: %v", gotKey, err)
				}
			}()
		}
	}
}

func TestIntegration_NilKey(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	testCases := []struct {
		in      K0
		wantErr bool
	}{
		{in: K0{K: testKey0}, wantErr: false},
		{in: K0{}, wantErr: false},
	}
	for _, tt := range testCases {
		gotKey, err := client.Put(ctx, IncompleteKey("NilKey", nil), &tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("client.Put %s got err %v, want err %t", tt.in, err, tt.wantErr)
		}
		defer func() {
			if err := client.Delete(ctx, gotKey); err != nil {
				t.Helper()
				t.Errorf("client.Delete for key %v: %v", gotKey, err)
			}
		}()
	}
}

type SQChild struct {
	I, J int
	T, U int64
	V    float64
	W    string
}

type SQTestCase struct {
	desc      string
	q         *Query
	wantCount int
	wantSum   int
}

func testSmallQueries(ctx context.Context, t *testing.T, client *Client, parent *Key, children []*SQChild,
	testCases []SQTestCase, extraTests ...func()) {
	keys := make([]*Key, len(children))
	for i := range keys {
		keys[i] = IncompleteKey("SQChild", parent)
	}
	keys, err := client.PutMulti(ctx, keys, children)
	if err != nil {
		t.Fatalf("client.PutMulti: %v", err)
	}
	defer func() {
		err := client.DeleteMulti(ctx, keys)
		if err != nil {
			t.Errorf("client.DeleteMulti: %v", err)
		}
	}()

	for _, tc := range testCases {
		count, err := client.Count(ctx, tc.q)
		if err != nil {
			t.Errorf("Count %q: %v", tc.desc, err)
			continue
		}
		if count != tc.wantCount {
			t.Errorf("Count %q: got %d want %d", tc.desc, count, tc.wantCount)
			continue
		}
	}

	for _, tc := range testCases {
		var got []SQChild
		_, err := client.GetAll(ctx, tc.q, &got)
		if err != nil {
			t.Errorf("client.GetAll %q: %v", tc.desc, err)
			continue
		}
		sum := 0
		for _, c := range got {
			sum += c.I + c.J
		}
		if sum != tc.wantSum {
			t.Errorf("sum %q: got %d want %d", tc.desc, sum, tc.wantSum)
			continue
		}
	}
	for _, x := range extraTests {
		x()
	}
}

func TestIntegration_FilterEntity(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	parent := NameKey("SQParent", "TestIntegration_Filters"+suffix, nil)
	now := timeNow.Truncate(time.Millisecond).Unix()
	tomorrow := timeNow.Truncate(time.Millisecond).AddDate(0, 0, 1).Unix()
	children := []*SQChild{
		{I: 0, J: 99, T: tomorrow, U: now},
		{I: 1, J: 98, T: tomorrow, U: now},
		{I: 2, J: 97, T: tomorrow, U: now},
		{I: 3, J: 96, T: now, U: now},
		{I: 4, J: 95, T: now, U: now},
		{I: 5, J: 94, T: now, U: now},
		{I: 6, J: 93, T: now, U: now},
		{I: 7, J: 92, T: now, U: now},
	}
	baseQuery := NewQuery("SQChild").Ancestor(parent)
	testSmallQueries(ctx, t, client, parent, children, []SQTestCase{
		{
			desc: "I>1",
			q: baseQuery.Filter("T=", now).FilterEntity(
				PropertyFilter{FieldName: "I", Operator: ">", Value: 1}),
			wantCount: 5,
			wantSum:   3 + 4 + 5 + 6 + 7 + 96 + 95 + 94 + 93 + 92,
		},
		{
			desc: "I<=1 or I >= 6",
			q: baseQuery.Filter("T=", now).FilterEntity(OrFilter{
				[]EntityFilter{
					PropertyFilter{FieldName: "I", Operator: "<", Value: 4},
					PropertyFilter{FieldName: "I", Operator: ">=", Value: 6},
				},
			}),
			wantCount: 3,
			wantSum:   3 + 6 + 7 + 92 + 93 + 96,
		},
		{
			desc: "(T = now) and (((J > 97) and (T = tomorrow)) or (J < 94))",
			q: baseQuery.FilterEntity(
				AndFilter{
					Filters: []EntityFilter{
						OrFilter{
							Filters: []EntityFilter{
								AndFilter{
									[]EntityFilter{
										PropertyFilter{FieldName: "J", Operator: ">", Value: 97},
										PropertyFilter{FieldName: "T", Operator: "=", Value: tomorrow},
									},
								},
								PropertyFilter{FieldName: "J", Operator: "<", Value: 94},
							},
						},
						PropertyFilter{FieldName: "T", Operator: "=", Value: now},
					},
				},
			),
			wantCount: 2,
			wantSum:   6 + 7 + 92 + 93,
		},
	})
}

func TestIntegration_Filters(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	parent := NameKey("SQParent", "TestIntegration_Filters"+suffix, nil)
	now := timeNow.Truncate(time.Millisecond).Unix()
	children := []*SQChild{
		{I: 0, T: now, U: now},
		{I: 1, T: now, U: now},
		{I: 2, T: now, U: now},
		{I: 3, T: now, U: now},
		{I: 4, T: now, U: now},
		{I: 5, T: now, U: now},
		{I: 6, T: now, U: now},
		{I: 7, T: now, U: now},
	}
	baseQuery := NewQuery("SQChild").Ancestor(parent).Filter("T=", now)
	testSmallQueries(ctx, t, client, parent, children, []SQTestCase{
		{
			"I>1",
			baseQuery.Filter("I>", 1),
			6,
			2 + 3 + 4 + 5 + 6 + 7,
		},
		{
			"I>2 AND I<=5",
			baseQuery.Filter("I>", 2).Filter("I<=", 5),
			3,
			3 + 4 + 5,
		},
		{
			"I>=3 AND I<3",
			baseQuery.Filter("I>=", 3).Filter("I<", 3),
			0,
			0,
		},
		{
			"I=4",
			baseQuery.Filter("I=", 4),
			1,
			4,
		},
		{
			"I!=191",
			baseQuery.FilterField("I", "!=", 191),
			8,
			28,
		},
		{
			"I in {2, 4}",
			baseQuery.FilterField("I", "in", []interface{}{2, 4}),
			2,
			6,
		},
		{
			"I not in {1, 3, 5, 7}",
			baseQuery.FilterField("I", "not-in", []interface{}{1, 3, 5, 7}),
			4,
			12,
		},
	}, func() {
		got := []*SQChild{}
		want := []*SQChild{
			{I: 0, T: now, U: now},
			{I: 1, T: now, U: now},
			{I: 2, T: now, U: now},
			{I: 3, T: now, U: now},
			{I: 4, T: now, U: now},
			{I: 5, T: now, U: now},
			{I: 6, T: now, U: now},
			{I: 7, T: now, U: now},
		}
		_, err := client.GetAll(ctx, baseQuery.Order("I"), &got)
		if err != nil {
			t.Errorf("client.GetAll: %v", err)
		}
		if !testutil.Equal(got, want) {
			t.Errorf("compare: got=%v, want=%v", got, want)
		}
	}, func() {
		got := []*SQChild{}
		want := []*SQChild{
			{I: 7, T: now, U: now},
			{I: 6, T: now, U: now},
			{I: 5, T: now, U: now},
			{I: 4, T: now, U: now},
			{I: 3, T: now, U: now},
			{I: 2, T: now, U: now},
			{I: 1, T: now, U: now},
			{I: 0, T: now, U: now},
		}
		_, err := client.GetAll(ctx, baseQuery.Order("-I"), &got)
		if err != nil {
			t.Errorf("client.GetAll: %v", err)
		}
		if !testutil.Equal(got, want) {
			t.Errorf("compare: got=%v, want=%v", got, want)
		}
	})
}

func populateData(t *testing.T, client *Client, childrenCount int, time int64, testKey string) ([]*Key, *Key, func(client *Client)) {
	ctx := context.Background()
	parent := NameKey("SQParent", keyPrefix+testKey+suffix, nil)

	children := []*SQChild{}

	for i := 0; i < childrenCount; i++ {
		children = append(children, &SQChild{I: i, T: time, U: time, V: 1.5, W: "str"})
	}
	keys := make([]*Key, childrenCount)
	for i := range keys {
		keys[i] = NameKey("SQChild", "sqChild"+fmt.Sprint(i), parent)
	}
	keys, err := client.PutMulti(ctx, keys, children)
	if err != nil {
		t.Fatalf("client.PutMulti: %v", err)
	}

	cleanup := func(client *Client) {
		err := client.DeleteMulti(ctx, keys)
		if err != nil {
			t.Errorf("client.DeleteMulti: %v", err)
		}
	}
	return keys, parent, cleanup
}

type RunTransactionResult struct {
	runTime float64
	err     error
}

func TestIntegration_BeginLaterPerf(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	runOptions := []bool{true, false} // whether BeginLater transaction option is used
	var avgRunTimes [2]float64        // In seconds
	numRepetitions := 10
	numKeys := 10

	res := make(chan RunTransactionResult)
	for i, runOption := range runOptions {
		sumRunTime := float64(0)

		// Create client
		ctx := context.Background()
		client := newTestClient(ctx, t)
		defer client.Close()

		// Populate data
		now := timeNow.Truncate(time.Millisecond).Unix()
		keys, _, cleanupData := populateData(t, client, numKeys, now, "BeginLaterPerf"+fmt.Sprint(runOption)+fmt.Sprint(now))
		currentCleanup := cleanupData // Capture loop variable
		t.Cleanup(func() {
			currentCleanup(newTestClient(ctx, t))
		})

		for rep := 0; rep < numRepetitions; rep++ {
			go runTransaction(ctx, client, keys, res, runOption, t)
		}
		for rep := 0; rep < numRepetitions; rep++ {
			runTransactionResult := <-res
			if runTransactionResult.err != nil {
				t.Fatal(runTransactionResult.err)
			}
			sumRunTime += runTransactionResult.runTime
		}

		avgRunTimes[i] = sumRunTime / float64(numRepetitions)
	}
	improvement := ((avgRunTimes[1] - avgRunTimes[0]) / avgRunTimes[1]) * 100
	t.Logf("Run times:: with BeginLater: %.3fs, without BeginLater: %.3fs. improvement: %.2f%%", avgRunTimes[0], avgRunTimes[1], improvement)
	if improvement < 0 {
		// Using BeginLater option involves a lot of mutex lock / unlock. So, it may / may not lead to performance improvements
		t.Logf("No perf improvement because of new transaction consistency type.")
	}
}

func runTransaction(ctx context.Context, client *Client, keys []*Key, res chan RunTransactionResult, beginLater bool, t *testing.T) {

	numKeys := len(keys)
	txOpts := []TransactionOption{}
	if beginLater {
		txOpts = append(txOpts, BeginLater)
	}

	start := time.Now()
	// Create transaction
	tx, err := client.NewTransaction(ctx, txOpts...)
	if err != nil {
		runTransactionResult := RunTransactionResult{
			err: fmt.Errorf("Failed to create transaction: %v", err),
		}
		res <- runTransactionResult
		return
	}

	// Perform operations in transaction
	dst := make([]*SQChild, numKeys)
	if err := tx.GetMulti(keys, dst); err != nil {
		runTransactionResult := RunTransactionResult{
			err: fmt.Errorf("GetMulti got: %v, want: nil", err),
		}
		res <- runTransactionResult
		return
	}
	if _, err := tx.PutMulti(keys, dst); err != nil {
		runTransactionResult := RunTransactionResult{
			err: fmt.Errorf("PutMulti got: %v, want: nil", err),
		}
		res <- runTransactionResult
		return
	}

	// Commit the transaction
	if _, err := tx.Commit(); err != nil {
		runTransactionResult := RunTransactionResult{
			err: fmt.Errorf("Commit got: %v, want: nil", err),
		}
		res <- runTransactionResult
		return
	}

	runTransactionResult := RunTransactionResult{
		runTime: time.Since(start).Seconds(),
	}
	res <- runTransactionResult
}

func TestIntegration_BeginLater(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	wantAggResult := AggregationResult(map[string]interface{}{
		"count": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 3}},
		"sum":   &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 3}},
		"avg":   &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: 1}},
	})

	mockErr := errors.New("Mock error")
	testcases := []struct {
		desc              string
		options           []TransactionOption
		hasReadOnlyOption bool
		failTransaction   bool
	}{
		{
			desc:              "Failed transaction with BeginLater, MaxAttempts(2), ReadOnly options",
			options:           []TransactionOption{BeginLater, MaxAttempts(2), ReadOnly},
			hasReadOnlyOption: true,
			failTransaction:   true,
		},
		{
			desc:              "BeginLater, MaxAttempts(2), ReadOnly",
			options:           []TransactionOption{BeginLater, MaxAttempts(2), ReadOnly},
			hasReadOnlyOption: true,
			failTransaction:   false,
		},
		{
			desc:              "BeginLater, MaxAttempts(2)",
			options:           []TransactionOption{BeginLater, MaxAttempts(2)},
			hasReadOnlyOption: false,
		},
		{
			desc:              "BeginLater, ReadOnly",
			options:           []TransactionOption{BeginLater, ReadOnly},
			hasReadOnlyOption: true,
		},
	}

	for _, testcase := range testcases {
		// Populate data
		now := timeNow.Truncate(time.Millisecond).Unix()
		keys, parent, cleanupData := populateData(t, client, 3, now, "BeginLater")
		currentCleanup := cleanupData // Capture loop variable
		t.Cleanup(func() {
			currentCleanup(newTestClient(ctx, t))
		})

		testutil.Retry(t, 5, 10*time.Second, func(r *testutil.R) {
			_, err := client.RunInTransaction(ctx, func(tx *Transaction) error {
				query := NewQuery("SQChild").Ancestor(parent).FilterField("T", "=", now).Transaction(tx)
				dst := []*SQChild{}
				if _, err := client.GetAll(ctx, query, &dst); err != nil {
					return err
				}

				aggQuery := query.NewAggregationQuery().
					WithCount("count").
					WithSum("I", "sum").
					WithAvg("I", "avg")
				gotAggResult, err := client.RunAggregationQuery(ctx, aggQuery)
				if err != nil {
					return err
				}
				if !reflect.DeepEqual(gotAggResult, wantAggResult) {
					return fmt.Errorf("Mismatch in aggregation result got: %+v, want: %+v", gotAggResult, wantAggResult)
				}

				if !testcase.hasReadOnlyOption {
					v := &SQChild{I: 22, T: now, U: now, V: 1.5, W: "str"}
					if _, err := tx.Put(keys[0], v); err != nil {
						return err
					}

					if err := tx.Delete(keys[1]); err != nil {
						return err
					}
				}
				if testcase.failTransaction {
					// Deliberately, fail the transaction to rollback it
					return mockErr
				}
				return nil
			}, testcase.options...)

			if !testcase.failTransaction {
				if err != nil {
					r.Errorf("%v got: %v, want: nil", testcase.desc, err)
				}
				if !testcase.hasReadOnlyOption {
					// Transactions are atomic. Check if Put and Delete succeeded ensuring they were run as transaction
					verifyBeginLater(r, testcase.desc+" Committed Put", client, parent, now, 22, 1)
					verifyBeginLater(r, testcase.desc+" Committed Delete", client, parent, now, 1, 0)
				}
			} else {
				if err == nil {
					r.Errorf("%v got: nil, want: %v", testcase.desc, mockErr)
				}
				if !testcase.hasReadOnlyOption {
					// Transactions are atomic. Check if Put and Delete rollbacked ensuring they were run as transaction
					verifyBeginLater(r, testcase.desc+" Rollbacked Put", client, parent, now, 22, 0)
					verifyBeginLater(r, testcase.desc+" Rollbacked Delete", client, parent, now, 1, 1)
				}
			}
		})
	}
}

func verifyBeginLater(r *testutil.R, errPrefix string, client *Client, parent *Key, tvalue int64, ivalue, wantDstLen int) {
	ctx := context.Background()
	query := NewQuery("SQChild").Ancestor(parent).FilterField("T", "=", tvalue).FilterField("I", "=", ivalue)
	dst := []*SQChild{}
	_, err := client.GetAll(ctx, query, &dst)
	if err != nil {
		r.Errorf("%v GetAll got: %v, want: nil", errPrefix, err)
	}
	if len(dst) != wantDstLen {
		r.Errorf("%v len(dst) got: %v, want: %v", errPrefix, len(dst), wantDstLen)
	}
}

func TestIntegration_AggregationQueriesInTransaction(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	parent := NameKey("SQParent", keyPrefix+"AggregationQueriesInTransaction"+suffix, nil)
	now := timeNow.Truncate(time.Millisecond).Unix()
	children := []*SQChild{
		{I: 0, T: now, U: now, V: 1.5, W: "str"},
		{I: 1, T: now, U: now, V: 1.5, W: "str"},
		{I: 2, T: now, U: now, V: 1.5, W: "str"},
	}

	keys := make([]*Key, len(children))
	for i := range keys {
		keys[i] = IncompleteKey("SQChild", parent)
	}

	keys, err := client.PutMulti(ctx, keys, children)
	if err != nil {
		t.Fatalf("client.PutMulti: %v", err)
	}
	defer func() {
		err := client.DeleteMulti(ctx, keys)
		if err != nil {
			t.Errorf("client.DeleteMulti: %v", err)
		}
	}()

	testcases := []struct {
		desc          string
		readTime      time.Time
		wantAggResult AggregationResult
	}{
		{
			desc:     "Aggregations in transaction before creating entities",
			readTime: time.Now().Add(-59 * time.Minute).Truncate(time.Microsecond),
			wantAggResult: map[string]interface{}{
				"count": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 0}},
				"sum":   &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 0}},
				"avg":   &pb.Value{ValueType: &pb.Value_NullValue{NullValue: structpb.NullValue_NULL_VALUE}},
			},
		},
		{
			desc: "Aggregations in transaction after creating entities",
			wantAggResult: map[string]interface{}{
				"count": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 3}},
				"sum":   &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 3}},
				"avg":   &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: 1}},
			},
		},
	}
	for _, tc := range testcases {
		testutil.Retry(t, 10, 10*time.Second, func(r *testutil.R) {
			readTime := tc.readTime
			if readTime.IsZero() {
				// Use current time as read time if read time is not specified in test case
				readTime = time.Now().Truncate(time.Microsecond)

				// Read time is truncated to microseconds. If immediately used in NewTransaction call,
				// it leads to "read_time cannot be in the future" error
				time.Sleep(time.Second)
			}

			tx, err := client.NewTransaction(ctx, []TransactionOption{ReadOnly, WithReadTime(readTime)}...)
			if err != nil {
				r.Errorf("client.NewTransaction: %v", err)
				return
			}
			aggQuery := NewQuery("SQChild").Ancestor(parent).Filter("T=", now).Transaction(tx).NewAggregationQuery().
				WithCount("count").
				WithSum("I", "sum").
				WithAvg("I", "avg")

			gotAggResult, gotErr := client.RunAggregationQuery(ctx, aggQuery)
			if gotErr != nil {
				r.Errorf("got: %v, want: nil", gotErr)
				return
			}
			if !aggResultsEquals(r, gotAggResult, tc.wantAggResult) {
				r.Errorf("%q: Mismatch in aggregation result got: %+v, want: %+v", tc.desc, gotAggResult, tc.wantAggResult)
				return
			}
		})
	}
}

func TestIntegration_AggregationQueries(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	beforeCreate := time.Now().Add(-5 * time.Minute).Truncate(time.Millisecond)

	parent := NameKey("SQParent", keyPrefix+"AggregationQueries"+suffix, nil)
	now := timeNow.Truncate(time.Millisecond).Unix()
	children := []*SQChild{
		{I: 0, T: now, U: now, V: 1.5, W: "str"},
		{I: 1, T: now, U: now, V: 1.5, W: "str"},
		{I: 2, T: now, U: now, V: 1.5, W: "str"},
		{I: 3, T: now, U: now, V: 1.5, W: "str"},
		{I: 4, T: now, U: now, V: 1.5, W: "str"},
		{I: 5, T: now, U: now, V: 1.5, W: "str"},
		{I: 6, T: now, U: now, V: 1.5, W: "str"},
		{I: 7, T: now, U: now, V: 1.5, W: "str"},
	}

	keys := make([]*Key, len(children))
	for i := range keys {
		keys[i] = IncompleteKey("SQChild", parent)
	}

	keys, err := client.PutMulti(ctx, keys, children)
	if err != nil {
		t.Fatalf("client.PutMulti: %v", err)
	}
	defer func() {
		err := client.DeleteMulti(ctx, keys)
		if err != nil {
			t.Errorf("client.DeleteMulti: %v", err)
		}
	}()

	testCases := []struct {
		desc               string
		aggQuery           *AggregationQuery
		transactionOpts    []TransactionOption
		clientReadOptions  []ReadOption
		useCurrentReadTime bool
		wantFailure        bool
		wantErrMsg         string
		wantAggResult      AggregationResult
	}{

		{
			desc: "Count Failure - Missing index",
			aggQuery: NewQuery("SQChild").Ancestor(parent).Filter("T>=", now).
				NewAggregationQuery().
				WithCount("count"),
			wantFailure: true,
			wantErrMsg:  "no matching index found",
		},
		{
			desc: "Count Success",
			aggQuery: NewQuery("SQChild").Ancestor(parent).Filter("T=", now).Filter("I>=", 3).
				NewAggregationQuery().
				WithCount("count"),
			wantAggResult: map[string]interface{}{
				"count": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}},
			},
		},
		{
			desc: "Count success before create with client read time",
			aggQuery: NewQuery("SQChild").Ancestor(parent).Filter("T=", now).Filter("I>=", 3).
				NewAggregationQuery().
				WithCount("count"),
			clientReadOptions: []ReadOption{ReadTime(beforeCreate)},
			wantAggResult: map[string]interface{}{
				"count": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 0}},
			},
		},
		{
			desc: "Count success after create with client read time",
			aggQuery: NewQuery("SQChild").Ancestor(parent).Filter("T=", now).Filter("I>=", 3).
				NewAggregationQuery().
				WithCount("count"),
			useCurrentReadTime: true,
			wantAggResult: map[string]interface{}{
				"count": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}},
			},
		},
		{
			desc: "Multiple aggregations",
			aggQuery: NewQuery("SQChild").Ancestor(parent).Filter("T=", now).
				NewAggregationQuery().
				WithSum("I", "i_sum").
				WithAvg("I", "avg").
				WithSum("V", "v_sum"),
			wantAggResult: map[string]interface{}{
				"i_sum": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 28}},
				"v_sum": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: 12}},
				"avg":   &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: 3.5}},
			},
		},
		{
			desc: "Multiple aggregations with limit ",
			aggQuery: NewQuery("SQChild").Ancestor(parent).Filter("T=", now).Limit(2).
				NewAggregationQuery().
				WithSum("I", "sum").
				WithAvg("I", "avg"),
			wantAggResult: map[string]interface{}{
				"sum": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
				"avg": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: 0.5}},
			},
		},
		{
			desc: "Multiple aggregations on non-numeric field",
			aggQuery: NewQuery("SQChild").Ancestor(parent).Filter("T=", now).Limit(2).
				NewAggregationQuery().
				WithSum("W", "sum").
				WithAvg("W", "avg"),
			wantAggResult: map[string]interface{}{
				"sum": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(0)}},
				"avg": &pb.Value{ValueType: &pb.Value_NullValue{NullValue: structpb.NullValue_NULL_VALUE}},
			},
		},
		{
			desc: "Sum aggregation without alias",
			aggQuery: NewQuery("SQChild").Ancestor(parent).Filter("T=", now).
				NewAggregationQuery().
				WithSum("I", ""),
			wantAggResult: map[string]interface{}{
				"property_1": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 28}},
			},
		},
		{
			desc: "Average aggregation without alias",
			aggQuery: NewQuery("SQChild").Ancestor(parent).Filter("T=", now).
				NewAggregationQuery().
				WithAvg("I", ""),
			wantAggResult: map[string]interface{}{
				"property_1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: 3.5}},
			},
		},
		{
			desc: "Sum aggregation on '__key__'",
			aggQuery: NewQuery("SQChild").Ancestor(parent).Filter("T=", now).
				NewAggregationQuery().
				WithSum("__key__", ""),
			wantFailure: true,
			wantErrMsg:  "Aggregations are not supported for the property",
		},
		{
			desc: "Average aggregation on '__key__'",
			aggQuery: NewQuery("SQChild").Ancestor(parent).Filter("T=", now).
				NewAggregationQuery().
				WithAvg("__key__", ""),
			wantFailure: true,
			wantErrMsg:  "Aggregations are not supported for the property",
		},
	}

	for _, testCase := range testCases {
		testutil.Retry(t, 10, time.Second, func(r *testutil.R) {
			testClient := client
			clientReadOptions := testCase.clientReadOptions
			if testCase.useCurrentReadTime {
				clientReadOptions = append(clientReadOptions, ReadTime(time.Now().Truncate(time.Millisecond)))
			}

			if len(clientReadOptions) > 0 {
				clientWithReadTime := newTestClient(ctx, t)
				clientWithReadTime.WithReadOptions(clientReadOptions...)
				defer clientWithReadTime.Close()

				testClient = clientWithReadTime
			}

			gotAggResult, gotErr := testClient.RunAggregationQuery(ctx, testCase.aggQuery)
			gotFailure := gotErr != nil

			if gotFailure != testCase.wantFailure ||
				(gotErr != nil && !strings.Contains(gotErr.Error(), testCase.wantErrMsg)) {
				r.Errorf("%q: Mismatch in error got: %v, want: %q", testCase.desc, gotErr, testCase.wantErrMsg)
				return
			}
			if gotErr == nil && !aggResultsEquals(r, gotAggResult, testCase.wantAggResult) {
				r.Errorf("%q: Mismatch in aggregation result got: %+v, want: %+v", testCase.desc, gotAggResult, testCase.wantAggResult)
				return
			}
		})
	}

}

func TestIntegration_RunAggregationQueryWithOptions(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	_, _, now, parent, cleanup := createTestEntities(ctx, t, client, "RunAggregationQueryWithOptions", 3)
	defer cleanup()

	aggQuery := NewQuery("SQChild").Ancestor(parent).Filter("T=", now).NewAggregationQuery().
		WithSum("I", "i_sum").WithAvg("I", "i_avg").WithCount("count")
	wantAggResult := map[string]interface{}{
		"i_sum": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 6}},
		"i_avg": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: 2}},
		"count": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: 3}},
	}

	testCases := []struct {
		desc        string
		wantFailure bool
		wantErrMsg  string
		wantRes     AggregationWithOptionsResult
		opts        []RunOption
	}{
		{
			desc: "No options",
			wantRes: AggregationWithOptionsResult{
				Result: wantAggResult,
			},
		},
		{
			desc: "ExplainOptions.Analyze is false",
			wantRes: AggregationWithOptionsResult{
				ExplainMetrics: &ExplainMetrics{
					PlanSummary: &PlanSummary{
						IndexesUsed: []*map[string]interface{}{
							{
								"properties":  "(T ASC, I ASC, __name__ ASC)",
								"query_scope": "Includes ancestors",
							},
						},
					},
				},
			},
			opts: []RunOption{ExplainOptions{}},
		},
		{
			desc: "ExplainOptions.Analyze is true",
			wantRes: AggregationWithOptionsResult{
				Result: wantAggResult,
				ExplainMetrics: &ExplainMetrics{
					PlanSummary: &PlanSummary{
						IndexesUsed: []*map[string]interface{}{
							{
								"properties":  "(T ASC, I ASC, __name__ ASC)",
								"query_scope": "Includes ancestors",
							},
						},
					},
					ExecutionStats: &ExecutionStats{
						ReadOperations:  1,
						ResultsReturned: 1,
						DebugStats: &map[string]interface{}{
							"documents_scanned":     "0",
							"index_entries_scanned": "3",
						},
					},
				},
			},
			opts: []RunOption{ExplainOptions{Analyze: true}},
		},
	}

	for _, testcase := range testCases {
		testutil.Retry(t, 10, time.Second, func(r *testutil.R) {
			gotRes, gotErr := client.RunAggregationQueryWithOptions(ctx, aggQuery, testcase.opts...)
			if gotErr != nil {
				r.Errorf("err: got %v, want: nil", gotErr)
			}

			if gotErr == nil && !testutil.Equal(gotRes.Result, testcase.wantRes.Result,
				cmpopts.IgnoreFields(ExplainMetrics{})) {
				r.Errorf("%q: Mismatch in aggregation result got: %v, want: %v", testcase.desc, gotRes, testcase.wantRes)
				return
			}

			if err := cmpExplainMetrics(gotRes.ExplainMetrics, testcase.wantRes.ExplainMetrics); err != nil {
				r.Errorf("%q: Mismatch in ExplainMetrics %+v", testcase.desc, err)
			}
		})
	}
}

type ckey struct{}

func TestIntegration_LargeQuery(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	parent := NameKey("LQParent", "TestIntegration_LargeQuery"+suffix, nil)
	now := timeNow.Truncate(time.Millisecond).Unix()

	// Make a large number of children entities.
	const n = 800
	children := make([]*SQChild, 0, n)
	keys := make([]*Key, 0, n)
	for i := 0; i < n; i++ {
		children = append(children, &SQChild{I: i, T: now, U: now})
		keys = append(keys, IncompleteKey("SQChild", parent))
	}

	// Store using PutMulti in batches.
	const batchSize = 500
	for i := 0; i < n; i = i + 500 {
		j := i + batchSize
		if j > n {
			j = n
		}
		fullKeys, err := client.PutMulti(ctx, keys[i:j], children[i:j])
		if err != nil {
			t.Fatalf("PutMulti(%d, %d): %v", i, j, err)
		}
		defer func() {
			err := client.DeleteMulti(ctx, fullKeys)
			if err != nil {
				t.Errorf("client.DeleteMulti: %v", err)
			}
		}()
	}

	q := NewQuery("SQChild").Ancestor(parent).Filter("T=", now).Order("I")

	// Check we get the expected count and results for various limits/offsets.
	queryTests := []struct {
		limit, offset, want int
	}{
		// Just limit.
		{limit: 0, want: 0},
		{limit: 100, want: 100},
		{limit: 501, want: 501},
		{limit: n, want: n},
		{limit: n * 2, want: n},
		{limit: -1, want: n},
		// Just offset.
		{limit: -1, offset: 100, want: n - 100},
		{limit: -1, offset: 500, want: n - 500},
		{limit: -1, offset: n, want: 0},
		// Limit and offset.
		{limit: 100, offset: 100, want: 100},
		{limit: 1000, offset: 100, want: n - 100},
		{limit: 500, offset: 500, want: n - 500},
	}
	for i, tt := range queryTests {
		q := q.Limit(tt.limit).Offset(tt.offset)
		limit := tt.limit
		offset := tt.offset
		want := tt.want

		t.Run(fmt.Sprintf("queryTest_%d", i), func(t *testing.T) {
			// Check Count returns the expected number of results.
			count, err := client.Count(ctx, q)
			if err != nil {
				t.Errorf("client.Count(limit=%d offset=%d): %v", limit, offset, err)
				return
			}
			if count != want {
				t.Errorf("Count(limit=%d offset=%d) returned %d, want %d", limit, offset, count, want)
			}

			var got []SQChild
			_, err = client.GetAll(ctx, q, &got)
			if err != nil {
				t.Errorf("client.GetAll(limit=%d offset=%d): %v", limit, offset, err)
				return
			}
			if len(got) != want {
				t.Errorf("GetAll(limit=%d offset=%d) returned %d, want %d", limit, offset, len(got), want)
			}
			for i, child := range got {
				if got, want := child.I, i+offset; got != want {
					t.Errorf("GetAll(limit=%d offset=%d) got[%d].I == %d; want %d", limit, offset, i, got, want)
					break
				}
			}
			t.Parallel()
		})
	}

	// Also check iterator cursor behavior.
	cursorTests := []struct {
		limit, offset int // Query limit and offset.
		count         int // The number of times to call "next"
		want          int // The I value of the desired element, -1 for "Done".
	}{
		// No limits.
		{count: 0, limit: -1, want: 0},
		{count: 5, limit: -1, want: 5},
		{count: 500, limit: -1, want: 500},
		{count: 1000, limit: -1, want: -1}, // No more results.
		// Limits.
		{count: 5, limit: 5, want: 5},
		{count: 500, limit: 5, want: 5},
		{count: 1000, limit: 1000, want: -1}, // No more results.
		// Offsets.
		{count: 0, offset: 5, limit: -1, want: 5},
		{count: 5, offset: 5, limit: -1, want: 10},
		{count: 200, offset: 500, limit: -1, want: 700},
		{count: 200, offset: 1000, limit: -1, want: -1}, // No more results.
	}
	for i, tt := range cursorTests {
		count := tt.count
		limit := tt.limit
		offset := tt.offset
		want := tt.want

		t.Run(fmt.Sprintf("cursorTest_%d", i), func(t *testing.T) {
			ctx := context.WithValue(ctx, ckey{}, fmt.Sprintf("c=%d,l=%d,o=%d", count, limit, offset))
			// Run iterator through count calls to Next.
			it := client.Run(ctx, q.Limit(limit).Offset(offset).KeysOnly())
			for i := 0; i < count; i++ {
				_, err := it.Next(nil)
				if err == iterator.Done {
					break
				}
				if err != nil {
					t.Errorf("count=%d, limit=%d, offset=%d: it.Next failed at i=%d", count, limit, offset, i)
					return
				}
			}

			// Grab the cursor.
			cursor, err := it.Cursor()
			if err != nil {
				t.Errorf("count=%d, limit=%d, offset=%d: it.Cursor: %v", count, limit, offset, err)
				return
			}

			// Make a request for the next element.
			it = client.Run(ctx, q.Limit(1).Start(cursor))
			var entity SQChild
			_, err = it.Next(&entity)
			switch {
			case want == -1:
				if err != iterator.Done {
					t.Errorf("count=%d, limit=%d, offset=%d: it.Next from cursor %v, want Done", count, limit, offset, err)
				}
			case err != nil:
				t.Errorf("count=%d, limit=%d, offset=%d: it.Next from cursor: %v, want nil", count, limit, offset, err)
			case entity.I != want:
				t.Errorf("count=%d, limit=%d, offset=%d: got.I = %d, want %d", count, limit, offset, entity.I, want)
			}
			t.Parallel()
		})
	}
}

func TestIntegration_EventualConsistency(t *testing.T) {
	// TODO(jba): either make this actually test eventual consistency, or
	// delete it. Currently it behaves the same with or without the
	// EventualConsistency call.
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	parent := NameKey("SQParent", "TestIntegration_EventualConsistency"+suffix, nil)
	now := timeNow.Truncate(time.Millisecond).Unix()
	children := []*SQChild{
		{I: 0, T: now, U: now},
		{I: 1, T: now, U: now},
		{I: 2, T: now, U: now},
	}
	query := NewQuery("SQChild").Ancestor(parent).Filter("T =", now).EventualConsistency()
	testSmallQueries(ctx, t, client, parent, children, nil, func() {
		got, err := client.Count(ctx, query)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if got < 0 || 3 < got {
			t.Errorf("Count: got %d, want [0,3]", got)
		}
	})
}

func TestIntegration_Projection(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	parent := NameKey("SQParent", "TestIntegration_Projection"+suffix, nil)
	now := timeNow.Truncate(time.Millisecond).Unix()
	children := []*SQChild{
		{I: 1 << 0, J: 100, T: now, U: now},
		{I: 1 << 1, J: 100, T: now, U: now},
		{I: 1 << 2, J: 200, T: now, U: now},
		{I: 1 << 3, J: 300, T: now, U: now},
		{I: 1 << 4, J: 300, T: now, U: now},
	}
	baseQuery := NewQuery("SQChild").Ancestor(parent).Filter("T=", now).Filter("J>", 150)
	testSmallQueries(ctx, t, client, parent, children, []SQTestCase{
		{
			"project",
			baseQuery.Project("J"),
			3,
			200 + 300 + 300,
		},
		{
			"distinct",
			baseQuery.Project("J").Distinct(),
			2,
			200 + 300,
		},
		{
			"distinct on",
			baseQuery.Project("J").DistinctOn("J"),
			2,
			200 + 300,
		},
		{
			"project on meaningful (GD_WHEN) field",
			baseQuery.Project("U"),
			3,
			0,
		},
	})
}

func TestIntegration_ReserveIDs(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	keys := make([]*Key, 3)
	for i := range keys {
		keys[i] = NameKey("ReserveIDs", "id-"+fmt.Sprint(i), nil)
	}
	err := client.ReserveIDs(ctx, keys)
	if err != nil {
		t.Fatalf("ReserveIDs failed: %v", err)
	}
}

func TestIntegration_AllocateIDs(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	keys := make([]*Key, 5)
	for i := range keys {
		keys[i] = IncompleteKey("AllocID", nil)
	}
	keys, err := client.AllocateIDs(ctx, keys)
	if err != nil {
		t.Errorf("AllocID #0 failed: %v", err)
	}
	if want := len(keys); want != 5 {
		t.Errorf("Expected to allocate 5 keys, %d keys are found", want)
	}
	for _, k := range keys {
		if k.Incomplete() {
			t.Errorf("Unexpeceted incomplete key found: %v", k)
		}
	}
}

func TestIntegration_GetAllWithFieldMismatch(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type Fat struct {
		X, Y int
	}
	type Thin struct {
		X int
	}

	// Ancestor queries (those within an entity group) are strongly consistent
	// by default, which prevents a test from being flaky.
	// See https://cloud.google.com/appengine/docs/go/datastore/queries#Go_Data_consistency
	// for more information.
	parent := NameKey("SQParent", "TestIntegration_GetAllWithFieldMismatch"+suffix, nil)
	putKeys := make([]*Key, 3)
	for i := range putKeys {
		putKeys[i] = IDKey("GetAllThing", int64(10+i), parent)
		_, err := client.Put(ctx, putKeys[i], &Fat{X: 20 + i, Y: 30 + i})
		if err != nil {
			t.Fatalf("client.Put: %v", err)
		}
	}
	defer func() {
		if err := client.DeleteMulti(ctx, putKeys); err != nil {
			t.Helper()
			t.Errorf("client.DeleteMulti for keys %v: %v", putKeys, err)
		}
	}()

	var got []Thin
	want := []Thin{
		{X: 20},
		{X: 21},
		{X: 22},
	}
	getKeys, err := client.GetAll(ctx, NewQuery("GetAllThing").Ancestor(parent), &got)
	if len(getKeys) != 3 && !testutil.Equal(getKeys, putKeys) {
		t.Errorf("client.GetAll: keys differ\ngetKeys=%v\nputKeys=%v", getKeys, putKeys)
	}
	if !testutil.Equal(got, want) {
		t.Errorf("client.GetAll: entities differ\ngot =%v\nwant=%v", got, want)
	}
	if _, ok := err.(*ErrFieldMismatch); !ok {
		t.Errorf("client.GetAll: got err=%v, want ErrFieldMismatch", err)
	}
}

func createTestEntities(ctx context.Context, t *testing.T, client *Client, partialNameKey string, count int) ([]*Key, []SQChild, int64, *Key, func()) {
	parent := NameKey("SQParent", keyPrefix+partialNameKey+suffix, nil)
	now := timeNow.Truncate(time.Millisecond).Unix()

	entities := []SQChild{}
	for i := 0; i < count; i++ {
		entities = append(entities, SQChild{I: i + 1, T: now, U: now, V: 1.5, W: "str"})
	}

	keys := make([]*Key, len(entities))
	for i := range keys {
		keys[i] = IncompleteKey("SQChild", parent)
	}

	// Create entities
	keys, err := client.PutMulti(ctx, keys, entities)
	if err != nil {
		t.Fatalf("client.PutMulti: %v", err)
	}
	return keys, entities, now, parent, func() {
		err := client.DeleteMulti(ctx, keys)
		if err != nil {
			t.Errorf("client.DeleteMulti: %v", err)
		}
	}
}

type runWithOptionsTestcase struct {
	desc               string
	wantKeys           []*Key
	wantExplainMetrics *ExplainMetrics
	wantEntities       []SQChild
	opts               []RunOption
}

func getRunWithOptionsTestcases(ctx context.Context, t *testing.T, client *Client, partialNameKey string, count int) ([]runWithOptionsTestcase, int64, *Key, func()) {
	keys, entities, now, parent, cleanup := createTestEntities(ctx, t, client, partialNameKey, count)
	return []runWithOptionsTestcase{
		{
			desc:         "No ExplainOptions",
			wantKeys:     keys,
			wantEntities: entities,
		},
		{
			desc: "ExplainOptions.Analyze is false",
			opts: []RunOption{ExplainOptions{}},
			wantExplainMetrics: &ExplainMetrics{
				PlanSummary: &PlanSummary{
					IndexesUsed: []*map[string]interface{}{
						{
							"properties":  "(T ASC, I ASC, __name__ ASC)",
							"query_scope": "Includes ancestors",
						},
					},
				},
			},
		},
		{
			desc:     "ExplainOptions.Analyze is true",
			opts:     []RunOption{ExplainOptions{Analyze: true}},
			wantKeys: keys,
			wantExplainMetrics: &ExplainMetrics{
				ExecutionStats: &ExecutionStats{
					ReadOperations:  int64(count),
					ResultsReturned: int64(count),
					DebugStats: &map[string]interface{}{
						"documents_scanned":     fmt.Sprint(count),
						"index_entries_scanned": fmt.Sprint(count),
					},
				},
				PlanSummary: &PlanSummary{
					IndexesUsed: []*map[string]interface{}{
						{
							"properties":  "(T ASC, I ASC, __name__ ASC)",
							"query_scope": "Includes ancestors",
						},
					},
				},
			},
			wantEntities: entities,
		},
	}, now, parent, cleanup
}

func TestIntegration_GetAllWithOptions(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()
	testcases, now, parent, cleanup := getRunWithOptionsTestcases(ctx, t, client, "GetAllWithOptions", 3)
	defer cleanup()
	query := NewQuery("SQChild").Ancestor(parent).Filter("T=", now).Order("I")
	for _, testcase := range testcases {
		var gotSQChildsFromGetAll []SQChild
		gotRes, gotErr := client.GetAllWithOptions(ctx, query, &gotSQChildsFromGetAll, testcase.opts...)
		if gotErr != nil {
			t.Errorf("%v err: got: %+v, want: nil", testcase.desc, gotErr)
		}
		if !testutil.Equal(gotSQChildsFromGetAll, testcase.wantEntities) {
			t.Errorf("%v entities: got: %+v, want: %+v", testcase.desc, gotSQChildsFromGetAll, testcase.wantEntities)
		}
		if !testutil.Equal(gotRes.Keys, testcase.wantKeys) {
			t.Errorf("%v keys: got: %+v, want: %+v", testcase.desc, gotRes.Keys, testcase.wantKeys)
		}
		if err := cmpExplainMetrics(gotRes.ExplainMetrics, testcase.wantExplainMetrics); err != nil {
			t.Errorf("%v %+v", testcase.desc, err)
		}
	}
}

func TestIntegration_RunWithOptions(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()
	testcases, now, parent, cleanup := getRunWithOptionsTestcases(ctx, t, client, "RunWithOptions", 3)
	defer cleanup()
	query := NewQuery("SQChild").Ancestor(parent).Filter("T=", now).Order("I")
	for _, testcase := range testcases {
		var gotSQChildsFromRun []SQChild
		iter := client.RunWithOptions(ctx, query, testcase.opts...)
		for {
			var gotSQChild SQChild
			_, err := iter.Next(&gotSQChild)
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Errorf("%v iter.Next: %v", testcase.desc, err)
			}
			gotSQChildsFromRun = append(gotSQChildsFromRun, gotSQChild)
		}
		if !testutil.Equal(gotSQChildsFromRun, testcase.wantEntities) {
			t.Errorf("%v entities: got: %+v, want: %+v", testcase.desc, gotSQChildsFromRun, testcase.wantEntities)
		}

		if err := cmpExplainMetrics(iter.ExplainMetrics, testcase.wantExplainMetrics); err != nil {
			t.Errorf("%v %+v", testcase.desc, err)
		}
	}
}

func cmpExplainMetrics(got *ExplainMetrics, want *ExplainMetrics) error {
	if (got != nil && want == nil) || (got == nil && want != nil) {
		return fmt.Errorf("ExplainMetrics: got: %+v, want: %+v", got, want)
	}
	if got == nil {
		return nil
	}
	if !testutil.Equal(got.PlanSummary, want.PlanSummary) {
		return fmt.Errorf("Plan: got: %+v, want: %+v", got.PlanSummary, want.PlanSummary)
	}
	if err := cmpExecutionStats(got.ExecutionStats, want.ExecutionStats); err != nil {
		return err
	}
	return nil
}

func cmpExecutionStats(got *ExecutionStats, want *ExecutionStats) error {
	if (got != nil && want == nil) || (got == nil && want != nil) {
		return fmt.Errorf("ExecutionStats: got: %+v, want: %+v", got, want)
	}
	if got == nil {
		return nil
	}

	// Compare all fields except DebugStats
	if !testutil.Equal(want, got, cmpopts.IgnoreFields(ExecutionStats{}, "DebugStats", "ExecutionDuration")) {
		return fmt.Errorf("ExecutionStats: mismatch (-want +got):\n%s", testutil.Diff(want, got, cmpopts.IgnoreFields(ExecutionStats{}, "DebugStats")))
	}

	// Compare DebugStats
	gotDebugStats := *got.DebugStats
	for wantK, wantV := range *want.DebugStats {
		// ExecutionStats.Debugstats has some keys whose values cannot be predicted. So, those values have not been included in want
		// Here, compare only those values included in want
		gotV, ok := gotDebugStats[wantK]
		if !ok || !testutil.Equal(gotV, wantV) {
			return fmt.Errorf("ExecutionStats.DebugStats: wantKey: %v  gotValue: %+v, wantValue: %+v", wantK, gotV, wantV)
		}
	}

	return nil
}

func aggResultsEquals(r *testutil.R, m1, m2 AggregationResult) bool {
	if len(m1) != len(m2) {
		r.Errorf("aggResultsEquals: length mismatch, len(m1)=%d, len(m2)=%d", len(m1), len(m2))
		return false
	}
	for k, v1 := range m1 {
		v2, ok := m2[k]
		if !ok {
			r.Errorf("aggResultsEquals: key %q not found in m2", k)
			return false
		}
		pbVal1, ok1 := v1.(*pb.Value)
		pbVal2, ok2 := v2.(*pb.Value)
		if !ok1 || !ok2 {
			r.Errorf("aggResultsEquals: type assertion to *pb.Value failed for key %q (ok1=%t, ok2=%t)", k, ok1, ok2)
			return false
		}
		if diff := testutil.Diff(pbVal1, pbVal2, cmpopts.IgnoreUnexported(pb.Value{})); diff != "" {
			r.Errorf("aggResultsEquals: failed for key %q\nv1=%v\nv2=%v\ndiff: got=-, want=+\n%v", k, pbVal1, pbVal2, diff)
			return false
		}
	}
	return true
}

func TestIntegration_KindlessQueries(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type Dee struct {
		I   int
		Why string
	}
	type Dum struct {
		I     int
		Pling string
	}

	parent := NameKey("Tweedle", "tweedle"+suffix, nil)

	keys := []*Key{
		NameKey("Dee", "dee0", parent),
		NameKey("Dum", "dum1", parent),
		NameKey("Dum", "dum2", parent),
		NameKey("Dum", "dum3", parent),
	}
	src := []interface{}{
		&Dee{1, "binary0001"},
		&Dum{2, "binary0010"},
		&Dum{4, "binary0100"},
		&Dum{8, "binary1000"},
	}
	keys, err := client.PutMulti(ctx, keys, src)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	defer func() {
		if err := client.DeleteMulti(ctx, keys); err != nil {
			t.Helper()
			t.Errorf("client.DeleteMulti for keys %v: %v", keys, err)
		}
	}()

	testCases := []struct {
		desc    string
		query   *Query
		want    []int
		wantErr string
	}{
		{
			desc:  "Dee",
			query: NewQuery("Dee"),
			want:  []int{1},
		},
		{
			desc:  "Doh",
			query: NewQuery("Doh"),
			want:  nil},
		{
			desc:  "Dum",
			query: NewQuery("Dum"),
			want:  []int{2, 4, 8},
		},
		{
			desc:  "",
			query: NewQuery(""),
			want:  []int{1, 2, 4, 8},
		},
		{
			desc:  "Kindless filter",
			query: NewQuery("").Filter("__key__ =", keys[2]),
			want:  []int{4},
		},
		{
			desc:  "Kindless order",
			query: NewQuery("").Order("__key__"),
			want:  []int{1, 2, 4, 8},
		},
		{
			desc:    "Kindless bad filter",
			query:   NewQuery("").Filter("I =", 4),
			wantErr: "kind is required",
		},
		{
			desc:    "Kindless bad order",
			query:   NewQuery("").Order("-__key__"),
			wantErr: "kind is required for all orders except __key__ ascending",
		},
	}
	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			q := test.query.Ancestor(parent)
			gotCount, err := client.Count(ctx, q)
			if err != nil {
				if test.wantErr == "" || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("count %q: err %v, want err %q", test.desc, err, test.wantErr)
				}
				return
			}
			if test.wantErr != "" {
				t.Fatalf("count %q: want err %q", test.desc, test.wantErr)
			}
			if gotCount != len(test.want) {
				t.Fatalf("count %q: got %d want %d", test.desc, gotCount, len(test.want))
			}
			var got []int
			for iter := client.Run(ctx, q); ; {
				var dst struct {
					I          int
					Why, Pling string
				}
				_, err := iter.Next(&dst)
				if err == iterator.Done {
					break
				}
				if err != nil {
					t.Fatalf("iter.Next %q: %v", test.desc, err)
				}
				got = append(got, dst.I)
			}
			sort.Ints(got)
			if !testutil.Equal(got, test.want) {
				t.Fatalf("elems %q: got %+v want %+v", test.desc, got, test.want)
			}
		})
	}
}

func TestIntegration_Transaction(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type Counter struct {
		N int
		T time.Time
	}

	bangErr := errors.New("bang")
	tests := []struct {
		desc          string
		causeConflict []bool
		retErr        []error
		want          int
		wantErr       error
	}{
		{
			desc:          "3 attempts, no conflicts",
			causeConflict: []bool{false},
			retErr:        []error{nil},
			want:          11,
		},
		{
			desc:          "1 attempt, user error",
			causeConflict: []bool{false},
			retErr:        []error{bangErr},
			wantErr:       bangErr,
		},
		{
			desc:          "2 attempts, 1 conflict",
			causeConflict: []bool{true, false},
			retErr:        []error{nil, nil},
			want:          13, // Each conflict increments by 2.
		},
		{
			desc:          "3 attempts, 3 conflicts",
			causeConflict: []bool{true, true, true},
			retErr:        []error{nil, nil, nil},
			wantErr:       ErrConcurrentTransaction,
		},
	}
	for i, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			// Put a new counter.
			c := &Counter{N: 10, T: timeNow}
			key, err := client.Put(ctx, IncompleteKey("TransCounter", nil), c)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Delete(ctx, key)

			// Increment the counter in a transaction.
			// The test case can manually cause a conflict or return an
			// error at each attempt.
			var attempts int
			_, err = client.RunInTransaction(ctx, func(tx *Transaction) error {
				attempts++
				if attempts > len(test.causeConflict) {
					return fmt.Errorf("too many attempts. Got %d, max %d", attempts, len(test.causeConflict))
				}

				var c Counter
				if err := tx.Get(key, &c); err != nil {
					return err
				}
				c.N++
				if _, err := tx.Put(key, &c); err != nil {
					return err
				}

				if test.causeConflict[attempts-1] {
					c.N++
					if _, err := client.Put(ctx, key, &c); err != nil {
						return err
					}
				}

				return test.retErr[attempts-1]
			}, MaxAttempts(i))

			// Check the error returned by RunInTransaction.
			if err != test.wantErr {
				t.Fatalf("got err %v, want %v", err, test.wantErr)
			}
			if test.wantErr != nil {
				// If we were expecting an error, this is where the test ends.
				return
			}

			// Check the final value of the counter.
			if err := client.Get(ctx, key, c); err != nil {
				t.Fatal(err)
			}
			if c.N != test.want {
				t.Fatalf("counter N=%d, want N=%d", c.N, test.want)
			}
		})
	}
}

func TestIntegration_ReadOnlyTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()
	client := newClient(ctx, t, nil)
	defer client.Close()

	type value struct{ N int }

	// Put a value.
	const n = 5
	v := &value{N: n}
	key, err := client.Put(ctx, IncompleteKey("roTxn", nil), v)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Delete(ctx, key)

	// Read it from a read-only transaction.
	_, err = client.RunInTransaction(ctx, func(tx *Transaction) error {
		if err := tx.Get(key, v); err != nil {
			return err
		}
		return nil
	}, ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if v.N != n {
		t.Fatalf("got %d, want %d", v.N, n)
	}

	// Attempting to write from a read-only transaction is an error.
	_, err = client.RunInTransaction(ctx, func(tx *Transaction) error {
		if _, err := tx.Put(key, v); err != nil {
			return err
		}
		return nil
	}, ReadOnly)
	if err == nil {
		t.Fatal("got nil, want error")
	}
}

func TestIntegration_NilPointers(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type X struct {
		S string
	}

	src := []*X{{"zero"}, {"one"}}
	keys := []*Key{IncompleteKey("NilX", nil), IncompleteKey("NilX", nil)}
	keys, err := client.PutMulti(ctx, keys, src)
	if err != nil {
		t.Fatalf("PutMulti: %v", err)
	}
	originalKeys := make([]*Key, len(keys)) // Keep a copy of the original keys for deferred cleanup
	copy(originalKeys, keys)
	defer func() {
		if err := client.DeleteMulti(ctx, originalKeys); err != nil {
			t.Helper()
			t.Errorf("client.DeleteMulti for original keys %v: %v", originalKeys, err)
		}
	}()

	// It's okay to store into a slice of nil *X.
	xs := make([]*X, 2)
	if err := client.GetMulti(ctx, keys, xs); err != nil {
		t.Errorf("GetMulti: %v", err)
	} else if !testutil.Equal(xs, src) {
		t.Errorf("GetMulti fetched %v, want %v", xs, src)
	}

	// It isn't okay to store into a single nil *X.
	var x0 *X
	if err, want := client.Get(ctx, keys[0], x0), ErrInvalidEntityType; err != want {
		t.Errorf("Get: err %v; want %v", err, want)
	}

	// Test that deleting with duplicate keys work.
	keys = append(keys, keys...)
	if err := client.DeleteMulti(ctx, keys); err != nil {
		t.Errorf("Delete: %v", err)
	}
}

func TestIntegration_NestedRepeatedElementNoIndex(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type Inner struct {
		Name  string
		Value string `datastore:",noindex"`
	}
	type Outer struct {
		Config []Inner
	}
	m := &Outer{
		Config: []Inner{
			{Name: "short", Value: "a"},
			{Name: "long", Value: strings.Repeat("a", 2000)},
		},
	}

	key := NameKey("Nested", "Nested"+suffix, nil)
	if _, err := client.Put(ctx, key, m); err != nil {
		t.Fatalf("client.Put: %v", err)
	}
	if err := client.Delete(ctx, key); err != nil {
		t.Fatalf("client.Delete: %v", err)
	}
}

func TestIntegration_PointerFields(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	want := populatedPointers()
	key, err := client.Put(ctx, IncompleteKey("pointers", nil), want)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := client.Delete(ctx, key); err != nil {
			t.Helper()
			t.Errorf("client.Delete for key %v: %v", key, err)
		}
	}()
	var got Pointers
	if err := client.Get(ctx, key, &got); err != nil {
		t.Fatal(err)
	}
	if got.Pi == nil || *got.Pi != *want.Pi {
		t.Errorf("Pi: got %v, want %v", got.Pi, *want.Pi)
	}
	if got.Ps == nil || *got.Ps != *want.Ps {
		t.Errorf("Ps: got %v, want %v", got.Ps, *want.Ps)
	}
	if got.Pb == nil || *got.Pb != *want.Pb {
		t.Errorf("Pb: got %v, want %v", got.Pb, *want.Pb)
	}
	if got.Pf == nil || *got.Pf != *want.Pf {
		t.Errorf("Pf: got %v, want %v", got.Pf, *want.Pf)
	}
	if got.Pg == nil || *got.Pg != *want.Pg {
		t.Errorf("Pg: got %v, want %v", got.Pg, *want.Pg)
	}
	if got.Pt == nil || !got.Pt.Equal(*want.Pt) {
		t.Errorf("Pt: got %v, want %v", got.Pt, *want.Pt)
	}
}

func TestIntegration_Mutate(t *testing.T) {
	// test Client.Mutate
	testMutate(t, func(ctx context.Context, client *Client, muts ...*Mutation) ([]*Key, error) {
		return client.Mutate(ctx, muts...)
	})
	// test Transaction.Mutate
	testMutate(t, func(ctx context.Context, client *Client, muts ...*Mutation) ([]*Key, error) {
		var pkeys []*PendingKey
		commit, err := client.RunInTransaction(ctx, func(tx *Transaction) error {
			var err error
			pkeys, err = tx.Mutate(muts...)
			return err
		})
		if err != nil {
			return nil, err
		}
		var keys []*Key
		for _, pk := range pkeys {
			keys = append(keys, commit.Key(pk))
		}
		return keys, nil
	})
}

func testMutate(t *testing.T, mutate func(ctx context.Context, client *Client, muts ...*Mutation) ([]*Key, error)) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type T struct{ I int }

	var createdKeysForCleanup []*Key
	defer func() {
		if len(createdKeysForCleanup) > 0 {
			if err := client.DeleteMulti(ctx, createdKeysForCleanup); err != nil {
				t.Helper()
				t.Errorf("client.DeleteMulti for keys %v: %v", createdKeysForCleanup, err)
			}
		}
	}()

	check := func(k *Key, want interface{}) {
		var x T
		err := client.Get(ctx, k, &x)
		switch want := want.(type) {
		case error:
			if err != want {
				t.Errorf("key %s: got error %v, want %v", k, err, want)
			}
		case int:
			if err != nil {
				t.Fatalf("key %s: %v", k, err)
			}
			if x.I != want {
				t.Errorf("key %s: got %d, want %d", k, x.I, want)
			}
		default:
			panic("check: bad arg")
		}
	}

	keys, err := mutate(ctx, client,
		NewInsert(IncompleteKey("t", nil), &T{1}),
		NewUpsert(IncompleteKey("t", nil), &T{2}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) > 0 && keys[0] != nil {
		createdKeysForCleanup = append(createdKeysForCleanup, keys[0])
	}
	if len(keys) > 1 && keys[1] != nil {
		createdKeysForCleanup = append(createdKeysForCleanup, keys[1])
	}

	check(keys[0], 1)
	check(keys[1], 2)

	_, err = mutate(ctx, client,
		NewUpdate(keys[0], &T{3}),
		NewDelete(keys[1]),
	)
	if err != nil {
		t.Fatal(err)
	}
	check(keys[0], 3)
	check(keys[1], ErrNoSuchEntity)

	_, err = mutate(ctx, client, NewInsert(keys[0], &T{4}))
	if got, want := status.Code(err), codes.AlreadyExists; got != want {
		t.Errorf("Insert existing key: got %s, want %s", got, want)
	}

	_, err = mutate(ctx, client, NewUpdate(keys[1], &T{4}))
	if got, want := status.Code(err), codes.NotFound; got != want {
		t.Errorf("Update non-existing key: got %s, want %s", got, want)
	}
}

func TestIntegration_DetectProjectID(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()

	creds := testutil.Credentials(ctx, ScopeDatastore)
	if creds == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}

	// Use creds with project ID.
	if _, err := NewClientWithDatabase(ctx, DetectProjectID, testParams["databaseID"].(string), option.WithCredentials(creds)); err != nil {
		t.Errorf("NewClientWithDatabase: %v", err)
	}

	ts := testutil.ErroringTokenSource{}
	// Try to use creds without project ID.
	_, err := NewClientWithDatabase(ctx, DetectProjectID, testParams["databaseID"].(string), option.WithTokenSource(ts))
	if err == nil || err.Error() != "datastore: see the docs on DetectProjectID" {
		t.Errorf("expected an error while using TokenSource that does not have a project ID")
	}
}

var genKeyName = uid.NewSpace("datastore-integration", nil)

func TestIntegration_Project_TimestampStoreAndRetrieve(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	type T struct{ Created time.Time }

	keyName := genKeyName.New()

	now := time.Now()
	k, err := client.Put(ctx, IncompleteKey(keyName, nil), &T{Created: now})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := client.Delete(ctx, k); err != nil {
			log.Println(err)
		}
	}()

	// Without .Ancestor, this query is eventually consistent (so this test
	// would be flakey). Ancestor queries, however, are strongly consistent.
	// See more at https://cloud.google.com/datastore/docs/articles/balancing-strong-and-eventual-consistency-with-google-cloud-datastore/.
	q := NewQuery(k.Kind).Ancestor(k)
	res := []T{}
	if _, err := client.GetAll(ctx, q, &res); err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}
	if got, want := res[0].Created.Unix(), now.Unix(); got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

type TestEntity struct {
	I    int
	F    float64
	T    time.Time
	S    []string
	Nums []int
}

func TestIntegration_Transforms(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(ctx, t)
	defer client.Close()

	// Test PropertyTransform operations
	t.Run("PropertyTransforms", func(t *testing.T) {
		initial := &TestEntity{
			I:    100,
			F:    100.0,
			S:    []string{"a", "b"},
			Nums: []int{1, 2, 3, 1},
		}
		k, err := client.Put(ctx, IncompleteKey("Transform", nil), initial)
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		defer client.Delete(ctx, k)

		transforms := []PropertyTransform{
			Increment("I", 5),                    // I: 100 -> 105
			Increment("F", -2.5),                 // F: 100.0 -> 97.5
			SetToServerTime("T"),                 // T: zero -> now
			Maximum("I", 110),                    // I: 105 -> 110
			Minimum("F", 90.0),                   // F: 97.5 -> 90.0
			AppendMissingElements("S", "c", "a"), // S: {"a", "b"} -> {"a", "b", "c"}
			RemoveAllFromArray("Nums", 1, 4),     // Nums: {1, 2, 3, 1} -> {2, 3}
		}

		req := &PutRequest{
			Key:        k,
			Entity:     initial,
			Transforms: transforms,
		}
		if _, err := client.PutWithOptions(ctx, req); err != nil {
			t.Fatalf("PutWithOptions with transforms: %v", err)
		}

		var final TestEntity
		if err := client.Get(ctx, k, &final); err != nil {
			t.Fatalf("Get: %v", err)
		}

		if final.I != 110 {
			t.Errorf("got I=%d, want 110", final.I)
		}
		if final.F != 90.0 {
			t.Errorf("got F=%f, want 90.0", final.F)
		}
		if time.Since(final.T) > 15*time.Second {
			t.Errorf("Expected timestamp to be recent, but got %v", final.T)
		}
		sort.Strings(final.S)
		if !testutil.Equal(final.S, []string{"a", "b", "c"}) {
			t.Errorf("Append: got %v, want [a b c]", final.S)
		}
		sort.Ints(final.Nums)
		if !testutil.Equal(final.Nums, []int{2, 3}) {
			t.Errorf("Remove: got %v, want [2 3]", final.Nums)
		}
	})

	// Test Transaction with PutMultiWithOptions
	t.Run("Transaction", func(t *testing.T) {
		keys := []*Key{
			IncompleteKey("TransformTx", nil),
			IncompleteKey("TransformTx", nil),
		}

		inc := Increment("I", 10)
		if inc.err != nil {
			t.Fatal(inc.err)
		}
		appendMissing := AppendMissingElements("Nums", 30)
		if appendMissing.err != nil {
			t.Fatal(appendMissing.err)
		}

		// Test both transaction.PutWithOptions and transaction.PutMultiWithOptions
		pendingKeys := []*PendingKey{}
		commit, err := client.RunInTransaction(ctx, func(tx *Transaction) error {
			k, err := tx.PutWithOptions(&PutRequest{Key: keys[0], Entity: &TestEntity{I: 50}, Transforms: []PropertyTransform{inc}})
			if err != nil {
				return err
			}
			pendingKeys = append(pendingKeys, k)
			defer client.Delete(ctx, k.key)

			reqs := []*PutRequest{
				{Key: keys[1], Entity: &TestEntity{Nums: []int{10, 20}}, Transforms: []PropertyTransform{appendMissing}},
			}
			putMultiPendingkeys, err := tx.PutMultiWithOptions(reqs)
			pendingKeys = append(pendingKeys, putMultiPendingkeys...)
			return err
		})

		resolvedKeys := []*Key{}
		for _, pendingKey := range pendingKeys {
			k := commit.Key(pendingKey)
			resolvedKeys = append(resolvedKeys, k)
			defer client.Delete(ctx, k)
		}

		if err != nil {
			t.Fatalf("RunInTransaction: %v", err)
		}

		var results [2]TestEntity
		if err := client.GetMulti(ctx, resolvedKeys, results[:]); err != nil {
			t.Fatalf("GetMulti: %v", err)
		}

		if results[0].I != 60 {
			t.Errorf("Tx Increment: got %d, want 60", results[0].I)
		}
		if !testutil.Equal(results[1].Nums, []int{10, 20, 30}) {
			t.Errorf("Tx Append: got %v, want [10 20 30]", results[1].Nums)
		}
	})

	// Test Mutation.WithTransforms
	t.Run("Mutate", func(t *testing.T) {
		k, err := client.Put(ctx, IncompleteKey("Transform", nil), &TestEntity{I: 10})
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		defer client.Delete(ctx, k)

		inc := Increment("I", -5)
		if inc.err != nil {
			t.Fatal(inc.err)
		}
		mut := NewUpdate(k, &TestEntity{I: 10}).WithTransforms(inc)
		_, err = client.Mutate(ctx, mut)
		if err != nil {
			t.Fatalf("Mutate: %v", err)
		}

		var final TestEntity
		if err := client.Get(ctx, k, &final); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if final.I != 5 {
			t.Errorf("got I=%d, want 5", final.I)
		}
	})
}
