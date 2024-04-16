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

package bigquery

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestIntegration_DatasetCreate(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	ds := client.Dataset(datasetIDs.New())
	wmd := &DatasetMetadata{Name: "name", Location: "EU"}
	err := ds.Create(ctx, wmd)
	if err != nil {
		t.Fatal(err)
	}
	gmd, err := ds.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := gmd.Name, wmd.Name; got != want {
		t.Errorf("name: got %q, want %q", got, want)
	}
	if got, want := gmd.Location, wmd.Location; got != want {
		t.Errorf("location: got %q, want %q", got, want)
	}
	if err := ds.Delete(ctx); err != nil {
		t.Fatalf("deleting dataset %v: %v", ds, err)
	}
}

func TestIntegration_DatasetMetadata(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	md, err := dataset.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := md.FullID, fmt.Sprintf("%s:%s", dataset.ProjectID, dataset.DatasetID); got != want {
		t.Errorf("FullID: got %q, want %q", got, want)
	}
	jan2016 := time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)
	if md.CreationTime.Before(jan2016) {
		t.Errorf("CreationTime: got %s, want > 2016-1-1", md.CreationTime)
	}
	if md.LastModifiedTime.Before(jan2016) {
		t.Errorf("LastModifiedTime: got %s, want > 2016-1-1", md.LastModifiedTime)
	}

	// Verify that we get a NotFound for a nonexistent dataset.
	_, err = client.Dataset("does_not_exist").Metadata(ctx)
	if err == nil || !hasStatusCode(err, http.StatusNotFound) {
		t.Errorf("got %v, want NotFound error", err)
	}
}

func TestIntegration_DatasetDelete(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	ds := client.Dataset(datasetIDs.New())
	if err := ds.Create(ctx, nil); err != nil {
		t.Fatalf("creating dataset %s: %v", ds.DatasetID, err)
	}
	if err := ds.Delete(ctx); err != nil {
		t.Fatalf("deleting dataset %s: %v", ds.DatasetID, err)
	}
}

func TestIntegration_DatasetDeleteWithContents(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	ds := client.Dataset(datasetIDs.New())
	if err := ds.Create(ctx, nil); err != nil {
		t.Fatalf("creating dataset %s: %v", ds.DatasetID, err)
	}
	table := ds.Table(tableIDs.New())
	if err := table.Create(ctx, nil); err != nil {
		t.Fatalf("creating table %s in dataset %s: %v", table.TableID, table.DatasetID, err)
	}
	// We expect failure here
	if err := ds.Delete(ctx); err == nil {
		t.Fatalf("non-recursive delete of dataset %s succeeded unexpectedly.", ds.DatasetID)
	}
	if err := ds.DeleteWithContents(ctx); err != nil {
		t.Fatalf("deleting recursively dataset %s: %v", ds.DatasetID, err)
	}
}

func TestIntegration_DatasetUpdateETags(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}

	check := func(md *DatasetMetadata, wantDesc, wantName string) {
		if md.Description != wantDesc {
			t.Errorf("description: got %q, want %q", md.Description, wantDesc)
		}
		if md.Name != wantName {
			t.Errorf("name: got %q, want %q", md.Name, wantName)
		}
	}

	ctx := context.Background()
	md, err := dataset.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if md.ETag == "" {
		t.Fatal("empty ETag")
	}
	// Write without ETag succeeds.
	desc := md.Description + "d2"
	name := md.Name + "n2"
	md2, err := dataset.Update(ctx, DatasetMetadataToUpdate{Description: desc, Name: name}, "")
	if err != nil {
		t.Fatal(err)
	}
	check(md2, desc, name)

	// Write with original ETag fails because of intervening write.
	_, err = dataset.Update(ctx, DatasetMetadataToUpdate{Description: "d", Name: "n"}, md.ETag)
	if err == nil {
		t.Fatal("got nil, want error")
	}

	// Write with most recent ETag succeeds.
	md3, err := dataset.Update(ctx, DatasetMetadataToUpdate{Description: "", Name: ""}, md2.ETag)
	if err != nil {
		t.Fatal(err)
	}
	check(md3, "", "")
}

func TestIntegration_DatasetUpdateDefaultExpirationAndMaxTimeTravel(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	_, err := dataset.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantExpiration := time.Hour
	wantTimeTravel := 48 * time.Hour
	// Set the default expiration time.
	md, err := dataset.Update(ctx, DatasetMetadataToUpdate{
		DefaultTableExpiration: wantExpiration,
		MaxTimeTravel:          wantTimeTravel,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := md.DefaultTableExpiration; got != wantExpiration {
		t.Fatalf("DefaultTableExpiration want %s got %s", wantExpiration, md.DefaultTableExpiration)
	}
	if got := md.MaxTimeTravel; got != wantTimeTravel {
		t.Fatalf("MaxTimeTravelHours want %s got %s", wantTimeTravel, md.MaxTimeTravel)
	}
	// Omitting DefaultTableExpiration doesn't change it.
	md, err = dataset.Update(ctx, DatasetMetadataToUpdate{Name: "xyz"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if md.DefaultTableExpiration != time.Hour {
		t.Fatalf("got %s, want 1h", md.DefaultTableExpiration)
	}
	// Setting it to 0 deletes it (which looks like a 0 duration).
	md, err = dataset.Update(ctx, DatasetMetadataToUpdate{DefaultTableExpiration: time.Duration(0)}, "")
	if err != nil {
		t.Fatal(err)
	}
	if md.DefaultTableExpiration != 0 {
		t.Fatalf("got %s, want 0", md.DefaultTableExpiration)
	}
}

func TestIntegration_DatasetUpdateDefaultPartitionExpiration(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	_, err := dataset.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Set the default partition expiration time.
	md, err := dataset.Update(ctx, DatasetMetadataToUpdate{DefaultPartitionExpiration: 24 * time.Hour}, "")
	if err != nil {
		t.Fatal(err)
	}
	if md.DefaultPartitionExpiration != 24*time.Hour {
		t.Fatalf("got %v, want 24h", md.DefaultPartitionExpiration)
	}
	// Omitting DefaultPartitionExpiration doesn't change it.
	md, err = dataset.Update(ctx, DatasetMetadataToUpdate{Name: "xyz"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if md.DefaultPartitionExpiration != 24*time.Hour {
		t.Fatalf("got %s, want 24h", md.DefaultPartitionExpiration)
	}
	// Setting it to 0 deletes it (which looks like a 0 duration).
	md, err = dataset.Update(ctx, DatasetMetadataToUpdate{DefaultPartitionExpiration: time.Duration(0)}, "")
	if err != nil {
		t.Fatal(err)
	}
	if md.DefaultPartitionExpiration != 0 {
		t.Fatalf("got %s, want 0", md.DefaultPartitionExpiration)
	}
}

func TestIntegration_DatasetUpdateDefaultCollation(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	caseInsensitiveCollation := "und:ci"
	caseSensitiveCollation := ""

	ctx := context.Background()
	ds := client.Dataset(datasetIDs.New())
	err := ds.Create(ctx, &DatasetMetadata{
		DefaultCollation: caseSensitiveCollation,
	})
	if err != nil {
		t.Fatal(err)
	}
	md, err := ds.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if md.DefaultCollation != caseSensitiveCollation {
		t.Fatalf("got %q, want %q", md.DefaultCollation, caseSensitiveCollation)
	}

	// Update the default collation
	md, err = ds.Update(ctx, DatasetMetadataToUpdate{
		DefaultCollation: caseInsensitiveCollation,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if md.DefaultCollation != caseInsensitiveCollation {
		t.Fatalf("got %q, want %q", md.DefaultCollation, caseInsensitiveCollation)
	}

	// Omitting DefaultCollation doesn't change it.
	md, err = ds.Update(ctx, DatasetMetadataToUpdate{Name: "xyz"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if md.DefaultCollation != caseInsensitiveCollation {
		t.Fatalf("got %q, want %q", md.DefaultCollation, caseInsensitiveCollation)
	}

	if err := ds.Delete(ctx); err != nil {
		t.Fatalf("deleting dataset %v: %v", ds, err)
	}
}

func TestIntegration_DatasetStorageBillingModel(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	t.Skip("BigQuery flat-rate commitments enabled for project, feature skipped")

	ctx := context.Background()
	md, err := dataset.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if md.StorageBillingModel != LogicalStorageBillingModel {
		t.Fatalf("got %q, want %q", md.StorageBillingModel, LogicalStorageBillingModel)
	}

	ds := client.Dataset(datasetIDs.New())
	err = ds.Create(ctx, &DatasetMetadata{
		StorageBillingModel: PhysicalStorageBillingModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	md, err = ds.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if md.StorageBillingModel != PhysicalStorageBillingModel {
		t.Fatalf("got %q, want %q", md.StorageBillingModel, PhysicalStorageBillingModel)
	}
	if err := ds.Delete(ctx); err != nil {
		t.Fatalf("deleting dataset %v: %v", ds, err)
	}
}

func TestIntegration_DatasetStorageUpdateBillingModel(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	t.Skip("BigQuery flat-rate commitments enabled for project, feature skipped")

	ctx := context.Background()
	ds := client.Dataset(datasetIDs.New())
	err := ds.Create(ctx, &DatasetMetadata{
		StorageBillingModel: LogicalStorageBillingModel,
	})
	if err != nil {
		t.Fatal(err)
	}

	md, err := ds.Metadata(ctx)
	if md.StorageBillingModel != LogicalStorageBillingModel {
		t.Fatalf("got %q, want %q", md.StorageBillingModel, LogicalStorageBillingModel)
	}

	// Update the Storage billing model
	md, err = ds.Update(ctx, DatasetMetadataToUpdate{
		StorageBillingModel: PhysicalStorageBillingModel,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if md.StorageBillingModel != PhysicalStorageBillingModel {
		t.Fatalf("got %q, want %q", md.StorageBillingModel, PhysicalStorageBillingModel)
	}

	// Omitting StorageBillingModel doesn't change it.
	md, err = ds.Update(ctx, DatasetMetadataToUpdate{Name: "xyz"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if md.StorageBillingModel != PhysicalStorageBillingModel {
		t.Fatalf("got %q, want %q", md.StorageBillingModel, PhysicalStorageBillingModel)
	}

	if err := ds.Delete(ctx); err != nil {
		t.Fatalf("deleting dataset %v: %v", ds, err)
	}
}

func TestIntegration_DatasetUpdateAccess(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	md, err := dataset.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Create a sample UDF so we can verify adding authorized UDFs
	routineID := routineIDs.New()
	routine := dataset.Routine(routineID)
	routineSQLID, _ := routine.Identifier(StandardSQLID)

	sql := fmt.Sprintf(`
			CREATE FUNCTION %s(x INT64) AS (x * 3);`,
		routineSQLID)
	if _, _, err := runQuerySQL(ctx, sql); err != nil {
		t.Fatal(err)
	}
	defer routine.Delete(ctx)

	origAccess := append([]*AccessEntry(nil), md.Access...)
	newEntries := []*AccessEntry{
		{
			Role:       ReaderRole,
			Entity:     "Joe@example.com",
			EntityType: UserEmailEntity,
		},
		{
			Role:       ReaderRole,
			Entity:     "allUsers",
			EntityType: IAMMemberEntity,
		},
		{
			EntityType: RoutineEntity,
			Routine:    routine,
		},
		{
			EntityType: DatasetEntity,
			Dataset: &DatasetAccessEntry{
				Dataset:     otherDataset,
				TargetTypes: []string{"VIEWS"},
			},
		},
	}

	newAccess := append(md.Access, newEntries...)
	dm := DatasetMetadataToUpdate{Access: newAccess}
	md, err = dataset.Update(ctx, dm, md.ETag)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, err := dataset.Update(ctx, DatasetMetadataToUpdate{Access: origAccess}, md.ETag)
		if err != nil {
			t.Log("could not restore dataset access list")
		}
	}()

	if diff := testutil.Diff(md.Access, newAccess, cmpopts.SortSlices(lessAccessEntries), cmpopts.IgnoreUnexported(Routine{}, Dataset{})); diff != "" {
		t.Errorf("got=-, want=+:\n%s", diff)
	}
}

// Comparison function for AccessEntries to enable order insensitive equality checking.
func lessAccessEntries(x, y *AccessEntry) bool {
	if x.Entity < y.Entity {
		return true
	}
	if x.Entity > y.Entity {
		return false
	}
	if x.EntityType < y.EntityType {
		return true
	}
	if x.EntityType > y.EntityType {
		return false
	}
	if x.Role < y.Role {
		return true
	}
	if x.Role > y.Role {
		return false
	}
	if x.View == nil {
		return y.View != nil
	}
	if x.Routine == nil {
		return y.Routine == nil
	}
	if x.Dataset == nil {
		return y.Dataset == nil
	}
	return false
}

func TestIntegration_DatasetUpdateLabels(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	_, err := dataset.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var dm DatasetMetadataToUpdate
	dm.SetLabel("label", "value")
	md, err := dataset.Update(ctx, dm, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := md.Labels["label"], "value"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	dm = DatasetMetadataToUpdate{}
	dm.DeleteLabel("label")
	md, err = dataset.Update(ctx, dm, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := md.Labels["label"]; ok {
		t.Error("label still present after deletion")
	}
}
