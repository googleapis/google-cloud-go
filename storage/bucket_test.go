// Copyright 2017 Google LLC
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

package storage

import (
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	storagepb "cloud.google.com/go/storage/internal/apiv2/stubs"
	"github.com/google/go-cmp/cmp"
	gax "github.com/googleapis/gax-go/v2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	raw "google.golang.org/api/storage/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestBucketAttrsToRawBucket(t *testing.T) {
	t.Parallel()
	attrs := &BucketAttrs{
		Name: "name",
		ACL:  []ACLRule{{Entity: "bob@example.com", Role: RoleOwner, Domain: "d", Email: "e"}},
		DefaultObjectACL: []ACLRule{{Entity: AllUsers, Role: RoleReader, EntityID: "eid",
			ProjectTeam: &ProjectTeam{ProjectNumber: "17", Team: "t"}}},
		Etag:         "Zkyw9ACJZUvcYmlFaKGChzhmtnE/dt1zHSfweiWpwzdGsqXwuJZqiD0",
		Location:     "loc",
		StorageClass: "class",
		RetentionPolicy: &RetentionPolicy{
			RetentionPeriod: 3 * time.Second,
		},
		BucketPolicyOnly:         BucketPolicyOnly{Enabled: true},
		UniformBucketLevelAccess: UniformBucketLevelAccess{Enabled: true},
		PublicAccessPrevention:   PublicAccessPreventionEnforced,
		VersioningEnabled:        false,
		RPO:                      RPOAsyncTurbo,
		// should be ignored:
		MetaGeneration: 39,
		Created:        time.Now(),
		Labels:         map[string]string{"label": "value"},
		CORS: []CORS{
			{
				MaxAge:          time.Hour,
				Methods:         []string{"GET", "POST"},
				Origins:         []string{"*"},
				ResponseHeaders: []string{"FOO"},
			},
		},
		Encryption: &BucketEncryption{DefaultKMSKeyName: "key"},
		Logging:    &BucketLogging{LogBucket: "lb", LogObjectPrefix: "p"},
		Website:    &BucketWebsite{MainPageSuffix: "mps", NotFoundPage: "404"},
		Autoclass:  &Autoclass{Enabled: true},
		Lifecycle: Lifecycle{
			Rules: []LifecycleRule{{
				Action: LifecycleAction{
					Type:         SetStorageClassAction,
					StorageClass: "NEARLINE",
				},
				Condition: LifecycleCondition{
					AgeInDays:             10,
					Liveness:              Live,
					CreatedBefore:         time.Date(2017, 1, 2, 3, 4, 5, 6, time.UTC),
					MatchesStorageClasses: []string{"STANDARD"},
					NumNewerVersions:      3,
				},
			}, {
				Action: LifecycleAction{
					Type:         SetStorageClassAction,
					StorageClass: "ARCHIVE",
				},
				Condition: LifecycleCondition{
					CustomTimeBefore:      time.Date(2020, 1, 2, 3, 0, 0, 0, time.UTC),
					DaysSinceCustomTime:   100,
					Liveness:              Live,
					MatchesStorageClasses: []string{"STANDARD"},
				},
			}, {
				Action: LifecycleAction{
					Type: DeleteAction,
				},
				Condition: LifecycleCondition{
					DaysSinceNoncurrentTime: 30,
					Liveness:                Live,
					NoncurrentTimeBefore:    time.Date(2017, 1, 2, 3, 4, 5, 6, time.UTC),
					MatchesStorageClasses:   []string{"NEARLINE"},
					NumNewerVersions:        10,
				},
			}, {
				Action: LifecycleAction{
					Type: DeleteAction,
				},
				Condition: LifecycleCondition{
					AgeInDays:        10,
					MatchesPrefix:    []string{"testPrefix"},
					MatchesSuffix:    []string{"testSuffix"},
					NumNewerVersions: 3,
				},
			}, {
				Action: LifecycleAction{
					Type: DeleteAction,
				},
				Condition: LifecycleCondition{
					Liveness: Archived,
				},
			}, {
				Action: LifecycleAction{
					Type: AbortIncompleteMPUAction,
				},
				Condition: LifecycleCondition{
					AgeInDays: 20,
				},
			}, {
				Action: LifecycleAction{
					Type: DeleteAction,
				},
				Condition: LifecycleCondition{
					AllObjects: true,
				},
			}},
		},
	}
	got := attrs.toRawBucket()
	want := &raw.Bucket{
		Name: "name",
		Acl: []*raw.BucketAccessControl{
			{Entity: "bob@example.com", Role: "OWNER"}, // other fields ignored on create/update
		},
		DefaultObjectAcl: []*raw.ObjectAccessControl{
			{Entity: "allUsers", Role: "READER"}, // other fields ignored on create/update
		},
		Location:     "loc",
		StorageClass: "class",
		RetentionPolicy: &raw.BucketRetentionPolicy{
			RetentionPeriod: 3,
		},
		IamConfiguration: &raw.BucketIamConfiguration{
			UniformBucketLevelAccess: &raw.BucketIamConfigurationUniformBucketLevelAccess{
				Enabled: true,
			},
			PublicAccessPrevention: "enforced",
		},
		Versioning: nil, // ignore VersioningEnabled if false
		Rpo:        rpoAsyncTurbo,
		Labels:     map[string]string{"label": "value"},
		Cors: []*raw.BucketCors{
			{
				MaxAgeSeconds:  3600,
				Method:         []string{"GET", "POST"},
				Origin:         []string{"*"},
				ResponseHeader: []string{"FOO"},
			},
		},
		Encryption: &raw.BucketEncryption{DefaultKmsKeyName: "key"},
		Logging:    &raw.BucketLogging{LogBucket: "lb", LogObjectPrefix: "p"},
		Website:    &raw.BucketWebsite{MainPageSuffix: "mps", NotFoundPage: "404"},
		Autoclass:  &raw.BucketAutoclass{Enabled: true},
		Lifecycle: &raw.BucketLifecycle{
			Rule: []*raw.BucketLifecycleRule{{
				Action: &raw.BucketLifecycleRuleAction{
					Type:         SetStorageClassAction,
					StorageClass: "NEARLINE",
				},
				Condition: &raw.BucketLifecycleRuleCondition{
					Age:                 googleapi.Int64(10),
					IsLive:              googleapi.Bool(true),
					CreatedBefore:       "2017-01-02",
					MatchesStorageClass: []string{"STANDARD"},
					NumNewerVersions:    3,
				},
			},
				{
					Action: &raw.BucketLifecycleRuleAction{
						StorageClass: "ARCHIVE",
						Type:         SetStorageClassAction,
					},
					Condition: &raw.BucketLifecycleRuleCondition{
						IsLive:              googleapi.Bool(true),
						CustomTimeBefore:    "2020-01-02",
						DaysSinceCustomTime: 100,
						MatchesStorageClass: []string{"STANDARD"},
					},
				},
				{
					Action: &raw.BucketLifecycleRuleAction{
						Type: DeleteAction,
					},
					Condition: &raw.BucketLifecycleRuleCondition{
						DaysSinceNoncurrentTime: 30,
						IsLive:                  googleapi.Bool(true),
						NoncurrentTimeBefore:    "2017-01-02",
						MatchesStorageClass:     []string{"NEARLINE"},
						NumNewerVersions:        10,
					},
				},
				{
					Action: &raw.BucketLifecycleRuleAction{
						Type: DeleteAction,
					},
					Condition: &raw.BucketLifecycleRuleCondition{
						Age:              googleapi.Int64(10),
						MatchesPrefix:    []string{"testPrefix"},
						MatchesSuffix:    []string{"testSuffix"},
						NumNewerVersions: 3,
					},
				},
				{
					Action: &raw.BucketLifecycleRuleAction{
						Type: DeleteAction,
					},
					Condition: &raw.BucketLifecycleRuleCondition{
						IsLive: googleapi.Bool(false),
					},
				},
				{
					Action: &raw.BucketLifecycleRuleAction{
						Type: AbortIncompleteMPUAction,
					},
					Condition: &raw.BucketLifecycleRuleCondition{
						Age: googleapi.Int64(20),
					},
				},
				{
					Action: &raw.BucketLifecycleRuleAction{
						Type: DeleteAction,
					},
					Condition: &raw.BucketLifecycleRuleCondition{
						Age:             googleapi.Int64(0),
						ForceSendFields: []string{"Age"},
					},
				},
			},
		},
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Error(msg)
	}

	attrs.VersioningEnabled = true
	attrs.RequesterPays = true
	got = attrs.toRawBucket()
	want.Versioning = &raw.BucketVersioning{Enabled: true}
	want.Billing = &raw.BucketBilling{RequesterPays: true}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Error(msg)
	}

	// Test that setting either of BucketPolicyOnly or UniformBucketLevelAccess
	// will enable UniformBucketLevelAccess.
	// Set UBLA.Enabled = true --> UBLA should be set to enabled in the proto.
	attrs.BucketPolicyOnly = BucketPolicyOnly{}
	attrs.UniformBucketLevelAccess = UniformBucketLevelAccess{Enabled: true}
	got = attrs.toRawBucket()
	want.IamConfiguration = &raw.BucketIamConfiguration{
		UniformBucketLevelAccess: &raw.BucketIamConfigurationUniformBucketLevelAccess{
			Enabled: true,
		},
		PublicAccessPrevention: "enforced",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Set BucketPolicyOnly.Enabled = true --> UBLA should be set to enabled in
	// the proto.
	attrs.BucketPolicyOnly = BucketPolicyOnly{Enabled: true}
	attrs.UniformBucketLevelAccess = UniformBucketLevelAccess{}
	got = attrs.toRawBucket()
	want.IamConfiguration = &raw.BucketIamConfiguration{
		UniformBucketLevelAccess: &raw.BucketIamConfigurationUniformBucketLevelAccess{
			Enabled: true,
		},
		PublicAccessPrevention: "enforced",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Set both BucketPolicyOnly.Enabled = true and
	// UniformBucketLevelAccess.Enabled=true --> UBLA should be set to enabled
	// in the proto.
	attrs.BucketPolicyOnly = BucketPolicyOnly{Enabled: true}
	attrs.UniformBucketLevelAccess = UniformBucketLevelAccess{Enabled: true}
	got = attrs.toRawBucket()
	want.IamConfiguration = &raw.BucketIamConfiguration{
		UniformBucketLevelAccess: &raw.BucketIamConfigurationUniformBucketLevelAccess{
			Enabled: true,
		},
		PublicAccessPrevention: "enforced",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Set UBLA.Enabled=false and BucketPolicyOnly.Enabled=false --> UBLA
	// should be disabled in the proto.
	attrs.BucketPolicyOnly = BucketPolicyOnly{}
	attrs.UniformBucketLevelAccess = UniformBucketLevelAccess{}
	got = attrs.toRawBucket()
	want.IamConfiguration = &raw.BucketIamConfiguration{
		PublicAccessPrevention: "enforced",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Test that setting PublicAccessPrevention to "unspecified" leads to the
	// inherited setting being propagated in the proto.
	attrs.PublicAccessPrevention = PublicAccessPreventionUnspecified
	got = attrs.toRawBucket()
	want.IamConfiguration = &raw.BucketIamConfiguration{
		PublicAccessPrevention: "inherited",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Test that setting PublicAccessPrevention to "inherited" leads to the
	// setting being propagated in the proto.
	attrs.PublicAccessPrevention = PublicAccessPreventionInherited
	got = attrs.toRawBucket()
	want.IamConfiguration = &raw.BucketIamConfiguration{
		PublicAccessPrevention: "inherited",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Test that setting RPO to default is propagated in the proto.
	attrs.RPO = RPODefault
	got = attrs.toRawBucket()
	want.Rpo = rpoDefault
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Re-enable UBLA and confirm that it does not affect the PAP setting.
	attrs.UniformBucketLevelAccess = UniformBucketLevelAccess{Enabled: true}
	got = attrs.toRawBucket()
	want.IamConfiguration = &raw.BucketIamConfiguration{
		UniformBucketLevelAccess: &raw.BucketIamConfigurationUniformBucketLevelAccess{
			Enabled: true,
		},
		PublicAccessPrevention: "inherited",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Disable UBLA and reset PAP to default. Confirm that the IAM config is set
	// to nil in the proto.
	attrs.UniformBucketLevelAccess = UniformBucketLevelAccess{Enabled: false}
	attrs.PublicAccessPrevention = PublicAccessPreventionUnknown
	got = attrs.toRawBucket()
	want.IamConfiguration = nil
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}
}

func TestBucketAttrsToUpdateToRawBucket(t *testing.T) {
	t.Parallel()
	au := &BucketAttrsToUpdate{
		VersioningEnabled:        false,
		RequesterPays:            false,
		BucketPolicyOnly:         &BucketPolicyOnly{Enabled: false},
		UniformBucketLevelAccess: &UniformBucketLevelAccess{Enabled: false},
		DefaultEventBasedHold:    false,
		RetentionPolicy:          &RetentionPolicy{RetentionPeriod: time.Hour},
		Encryption:               &BucketEncryption{DefaultKMSKeyName: "key2"},
		Lifecycle: &Lifecycle{
			Rules: []LifecycleRule{
				{
					Action:    LifecycleAction{Type: "Delete"},
					Condition: LifecycleCondition{AgeInDays: 30},
				},
				{
					Action:    LifecycleAction{Type: AbortIncompleteMPUAction},
					Condition: LifecycleCondition{AgeInDays: 13},
				},
			},
		},
		Logging:      &BucketLogging{LogBucket: "lb", LogObjectPrefix: "p"},
		Website:      &BucketWebsite{MainPageSuffix: "mps", NotFoundPage: "404"},
		StorageClass: "NEARLINE",
		Autoclass:    &Autoclass{Enabled: false},
	}
	au.SetLabel("a", "foo")
	au.DeleteLabel("b")
	au.SetLabel("c", "")
	got := au.toRawBucket()
	want := &raw.Bucket{
		Versioning: &raw.BucketVersioning{
			Enabled:         false,
			ForceSendFields: []string{"Enabled"},
		},
		Labels: map[string]string{
			"a": "foo",
			"c": "",
		},
		Billing: &raw.BucketBilling{
			RequesterPays:   false,
			ForceSendFields: []string{"RequesterPays"},
		},
		DefaultEventBasedHold: false,
		RetentionPolicy:       &raw.BucketRetentionPolicy{RetentionPeriod: 3600},
		IamConfiguration: &raw.BucketIamConfiguration{
			UniformBucketLevelAccess: &raw.BucketIamConfigurationUniformBucketLevelAccess{
				Enabled:         false,
				ForceSendFields: []string{"Enabled"},
			},
		},
		Encryption: &raw.BucketEncryption{DefaultKmsKeyName: "key2"},
		NullFields: []string{"Labels.b"},
		Lifecycle: &raw.BucketLifecycle{
			Rule: []*raw.BucketLifecycleRule{
				{
					Action:    &raw.BucketLifecycleRuleAction{Type: "Delete"},
					Condition: &raw.BucketLifecycleRuleCondition{Age: googleapi.Int64(30)},
				},
				{
					Action:    &raw.BucketLifecycleRuleAction{Type: AbortIncompleteMPUAction},
					Condition: &raw.BucketLifecycleRuleCondition{Age: googleapi.Int64(13)},
				},
			},
		},
		Logging:         &raw.BucketLogging{LogBucket: "lb", LogObjectPrefix: "p"},
		Website:         &raw.BucketWebsite{MainPageSuffix: "mps", NotFoundPage: "404"},
		StorageClass:    "NEARLINE",
		Autoclass:       &raw.BucketAutoclass{Enabled: false, ForceSendFields: []string{"Enabled"}},
		ForceSendFields: []string{"DefaultEventBasedHold", "Lifecycle"},
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Error(msg)
	}

	var au2 BucketAttrsToUpdate
	au2.DeleteLabel("b")
	got = au2.toRawBucket()
	want = &raw.Bucket{
		Labels:          map[string]string{},
		ForceSendFields: []string{"Labels"},
		NullFields:      []string{"Labels.b"},
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Error(msg)
	}

	// Test nulls.
	au3 := &BucketAttrsToUpdate{
		RetentionPolicy: &RetentionPolicy{},
		Encryption:      &BucketEncryption{},
		Logging:         &BucketLogging{},
		Website:         &BucketWebsite{},
	}
	got = au3.toRawBucket()
	want = &raw.Bucket{
		NullFields: []string{"RetentionPolicy", "Encryption", "Logging", "Website"},
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Error(msg)
	}

	// Test that setting either of BucketPolicyOnly or UniformBucketLevelAccess
	// will enable UniformBucketLevelAccess.
	// Set UBLA.Enabled = true --> UBLA should be set to enabled in the proto.
	au4 := &BucketAttrsToUpdate{
		UniformBucketLevelAccess: &UniformBucketLevelAccess{Enabled: true},
	}
	got = au4.toRawBucket()
	want = &raw.Bucket{
		IamConfiguration: &raw.BucketIamConfiguration{
			UniformBucketLevelAccess: &raw.BucketIamConfigurationUniformBucketLevelAccess{
				Enabled:         true,
				ForceSendFields: []string{"Enabled"},
			},
		},
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Set BucketPolicyOnly.Enabled = true --> UBLA should be set to enabled in
	// the proto.
	au5 := &BucketAttrsToUpdate{
		BucketPolicyOnly: &BucketPolicyOnly{Enabled: true},
	}
	got = au5.toRawBucket()
	want = &raw.Bucket{
		IamConfiguration: &raw.BucketIamConfiguration{
			UniformBucketLevelAccess: &raw.BucketIamConfigurationUniformBucketLevelAccess{
				Enabled:         true,
				ForceSendFields: []string{"Enabled"},
			},
		},
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Set both BucketPolicyOnly.Enabled = true and
	// UniformBucketLevelAccess.Enabled=true --> UBLA should be set to enabled
	// in the proto.
	au6 := &BucketAttrsToUpdate{
		BucketPolicyOnly:         &BucketPolicyOnly{Enabled: true},
		UniformBucketLevelAccess: &UniformBucketLevelAccess{Enabled: true},
	}
	got = au6.toRawBucket()
	want = &raw.Bucket{
		IamConfiguration: &raw.BucketIamConfiguration{
			UniformBucketLevelAccess: &raw.BucketIamConfigurationUniformBucketLevelAccess{
				Enabled:         true,
				ForceSendFields: []string{"Enabled"},
			},
		},
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Set UBLA.Enabled=false and BucketPolicyOnly.Enabled=false --> UBLA
	// should be disabled in the proto.
	au7 := &BucketAttrsToUpdate{
		BucketPolicyOnly:         &BucketPolicyOnly{Enabled: false},
		UniformBucketLevelAccess: &UniformBucketLevelAccess{Enabled: false},
	}
	got = au7.toRawBucket()
	want = &raw.Bucket{
		IamConfiguration: &raw.BucketIamConfiguration{
			UniformBucketLevelAccess: &raw.BucketIamConfigurationUniformBucketLevelAccess{
				Enabled:         false,
				ForceSendFields: []string{"Enabled"},
			},
		},
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// UBLA.Enabled will have precedence above BucketPolicyOnly.Enabled if both
	// are set with different values.
	au8 := &BucketAttrsToUpdate{
		BucketPolicyOnly:         &BucketPolicyOnly{Enabled: true},
		UniformBucketLevelAccess: &UniformBucketLevelAccess{Enabled: false},
	}
	got = au8.toRawBucket()
	want = &raw.Bucket{
		IamConfiguration: &raw.BucketIamConfiguration{
			UniformBucketLevelAccess: &raw.BucketIamConfigurationUniformBucketLevelAccess{
				Enabled:         false,
				ForceSendFields: []string{"Enabled"},
			},
		},
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Set an empty Lifecycle and verify that it will be sent.
	au9 := &BucketAttrsToUpdate{
		Lifecycle: &Lifecycle{},
	}
	got = au9.toRawBucket()
	want = &raw.Bucket{
		Lifecycle: &raw.BucketLifecycle{
			ForceSendFields: []string{"Rule"},
		},
		ForceSendFields: []string{"Lifecycle"},
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}
}

func TestNewBucket(t *testing.T) {
	labels := map[string]string{"a": "b"}
	matchClasses := []string{"STANDARD"}
	aTime := time.Date(2017, 1, 2, 0, 0, 0, 0, time.UTC)
	rb := &raw.Bucket{
		Name:                  "name",
		Location:              "loc",
		DefaultEventBasedHold: true,
		Metageneration:        3,
		StorageClass:          "sc",
		TimeCreated:           "2017-10-23T04:05:06Z",
		Versioning:            &raw.BucketVersioning{Enabled: true},
		Labels:                labels,
		Billing:               &raw.BucketBilling{RequesterPays: true},
		Etag:                  "Zkyw9ACJZUvcYmlFaKGChzhmtnE/dt1zHSfweiWpwzdGsqXwuJZqiD0",
		Lifecycle: &raw.BucketLifecycle{
			Rule: []*raw.BucketLifecycleRule{{
				Action: &raw.BucketLifecycleRuleAction{
					Type:         "SetStorageClass",
					StorageClass: "NEARLINE",
				},
				Condition: &raw.BucketLifecycleRuleCondition{
					Age:                 googleapi.Int64(10),
					IsLive:              googleapi.Bool(true),
					CreatedBefore:       "2017-01-02",
					MatchesStorageClass: matchClasses,
					NumNewerVersions:    3,
				},
			}},
		},
		RetentionPolicy: &raw.BucketRetentionPolicy{
			RetentionPeriod: 3,
			EffectiveTime:   aTime.Format(time.RFC3339),
		},
		IamConfiguration: &raw.BucketIamConfiguration{
			BucketPolicyOnly: &raw.BucketIamConfigurationBucketPolicyOnly{
				Enabled:    true,
				LockedTime: aTime.Format(time.RFC3339),
			},
			UniformBucketLevelAccess: &raw.BucketIamConfigurationUniformBucketLevelAccess{
				Enabled:    true,
				LockedTime: aTime.Format(time.RFC3339),
			},
		},
		Cors: []*raw.BucketCors{
			{
				MaxAgeSeconds:  3600,
				Method:         []string{"GET", "POST"},
				Origin:         []string{"*"},
				ResponseHeader: []string{"FOO"},
			},
		},
		Acl: []*raw.BucketAccessControl{
			{Bucket: "name", Role: "READER", Email: "joe@example.com", Entity: "allUsers"},
		},
		LocationType:  "dual-region",
		Encryption:    &raw.BucketEncryption{DefaultKmsKeyName: "key"},
		Logging:       &raw.BucketLogging{LogBucket: "lb", LogObjectPrefix: "p"},
		Website:       &raw.BucketWebsite{MainPageSuffix: "mps", NotFoundPage: "404"},
		ProjectNumber: 123231313,
		Autoclass: &raw.BucketAutoclass{
			Enabled:    true,
			ToggleTime: "2017-10-23T04:05:06Z",
		},
	}
	want := &BucketAttrs{
		Name:                  "name",
		Location:              "loc",
		DefaultEventBasedHold: true,
		MetaGeneration:        3,
		StorageClass:          "sc",
		Created:               time.Date(2017, 10, 23, 4, 5, 6, 0, time.UTC),
		VersioningEnabled:     true,
		Labels:                labels,
		Etag:                  "Zkyw9ACJZUvcYmlFaKGChzhmtnE/dt1zHSfweiWpwzdGsqXwuJZqiD0",
		RequesterPays:         true,
		Lifecycle: Lifecycle{
			Rules: []LifecycleRule{
				{
					Action: LifecycleAction{
						Type:         SetStorageClassAction,
						StorageClass: "NEARLINE",
					},
					Condition: LifecycleCondition{
						AgeInDays:             10,
						Liveness:              Live,
						CreatedBefore:         time.Date(2017, 1, 2, 0, 0, 0, 0, time.UTC),
						MatchesStorageClasses: matchClasses,
						NumNewerVersions:      3,
					},
				},
			},
		},
		RetentionPolicy: &RetentionPolicy{
			EffectiveTime:   aTime,
			RetentionPeriod: 3 * time.Second,
		},
		BucketPolicyOnly:         BucketPolicyOnly{Enabled: true, LockedTime: aTime},
		UniformBucketLevelAccess: UniformBucketLevelAccess{Enabled: true, LockedTime: aTime},
		CORS: []CORS{
			{
				MaxAge:          time.Hour,
				Methods:         []string{"GET", "POST"},
				Origins:         []string{"*"},
				ResponseHeaders: []string{"FOO"},
			},
		},
		Encryption:       &BucketEncryption{DefaultKMSKeyName: "key"},
		Logging:          &BucketLogging{LogBucket: "lb", LogObjectPrefix: "p"},
		Website:          &BucketWebsite{MainPageSuffix: "mps", NotFoundPage: "404"},
		ACL:              []ACLRule{{Entity: "allUsers", Role: RoleReader, Email: "joe@example.com"}},
		DefaultObjectACL: nil,
		LocationType:     "dual-region",
		ProjectNumber:    123231313,
		Autoclass: &Autoclass{
			Enabled:    true,
			ToggleTime: time.Date(2017, 10, 23, 4, 5, 6, 0, time.UTC),
		},
	}
	got, err := newBucket(rb)
	if err != nil {
		t.Fatal(err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("got=-, want=+:\n%s", diff)
	}
}

func TestNewBucketFromProto(t *testing.T) {
	pb := &storagepb.Bucket{
		Name: "name",
		Acl: []*storagepb.BucketAccessControl{
			{Entity: "bob@example.com", Role: "OWNER"},
		},
		DefaultObjectAcl: []*storagepb.ObjectAccessControl{
			{Entity: "allUsers", Role: "READER"},
		},
		Location:     "loc",
		LocationType: "region",
		StorageClass: "class",
		RetentionPolicy: &storagepb.Bucket_RetentionPolicy{
			RetentionDuration: durationpb.New(3 * time.Second),
			EffectiveTime:     toProtoTimestamp(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
		},
		IamConfig: &storagepb.Bucket_IamConfig{
			UniformBucketLevelAccess: &storagepb.Bucket_IamConfig_UniformBucketLevelAccess{
				Enabled:  true,
				LockTime: toProtoTimestamp(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
			},
			PublicAccessPrevention: "enforced",
		},
		Rpo:            rpoAsyncTurbo,
		Metageneration: int64(39),
		CreateTime:     toProtoTimestamp(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
		Labels:         map[string]string{"label": "value"},
		Cors: []*storagepb.Bucket_Cors{
			{
				MaxAgeSeconds:  3600,
				Method:         []string{"GET", "POST"},
				Origin:         []string{"*"},
				ResponseHeader: []string{"FOO"},
			},
		},
		Encryption: &storagepb.Bucket_Encryption{DefaultKmsKey: "key"},
		Logging:    &storagepb.Bucket_Logging{LogBucket: "projects/_/buckets/lb", LogObjectPrefix: "p"},
		Website:    &storagepb.Bucket_Website{MainPageSuffix: "mps", NotFoundPage: "404"},
		Autoclass:  &storagepb.Bucket_Autoclass{Enabled: true, ToggleTime: toProtoTimestamp(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))},
		Lifecycle: &storagepb.Bucket_Lifecycle{
			Rule: []*storagepb.Bucket_Lifecycle_Rule{
				{
					Action: &storagepb.Bucket_Lifecycle_Rule_Action{Type: "Delete"},
					Condition: &storagepb.Bucket_Lifecycle_Rule_Condition{
						AgeDays: proto.Int32(int32(10)),
					},
				},
			},
		},
	}
	want := &BucketAttrs{
		Name:             "name",
		ACL:              []ACLRule{{Entity: "bob@example.com", Role: RoleOwner}},
		DefaultObjectACL: []ACLRule{{Entity: AllUsers, Role: RoleReader}},
		Location:         "loc",
		LocationType:     "region",
		StorageClass:     "class",
		RetentionPolicy: &RetentionPolicy{
			RetentionPeriod: 3 * time.Second,
			EffectiveTime:   time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		BucketPolicyOnly:         BucketPolicyOnly{Enabled: true, LockedTime: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
		UniformBucketLevelAccess: UniformBucketLevelAccess{Enabled: true, LockedTime: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
		PublicAccessPrevention:   PublicAccessPreventionEnforced,
		RPO:                      RPOAsyncTurbo,
		MetaGeneration:           39,
		Created:                  time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		Labels:                   map[string]string{"label": "value"},
		CORS: []CORS{
			{
				MaxAge:          time.Hour,
				Methods:         []string{"GET", "POST"},
				Origins:         []string{"*"},
				ResponseHeaders: []string{"FOO"},
			},
		},
		Encryption: &BucketEncryption{DefaultKMSKeyName: "key"},
		Logging:    &BucketLogging{LogBucket: "lb", LogObjectPrefix: "p"},
		Website:    &BucketWebsite{MainPageSuffix: "mps", NotFoundPage: "404"},
		Autoclass:  &Autoclass{Enabled: true, ToggleTime: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
		Lifecycle: Lifecycle{
			Rules: []LifecycleRule{{
				Action: LifecycleAction{
					Type: DeleteAction,
				},
				Condition: LifecycleCondition{
					AgeInDays: 10,
				},
			}},
		},
	}
	got := newBucketFromProto(pb)
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("got=-, want=+:\n%s", diff)
	}
}

func TestBucketAttrsToProtoBucket(t *testing.T) {
	t.Parallel()
	attrs := &BucketAttrs{
		Name: "name",
		ACL:  []ACLRule{{Entity: "bob@example.com", Role: RoleOwner, Domain: "d", Email: "e"}},
		DefaultObjectACL: []ACLRule{{Entity: AllUsers, Role: RoleReader, EntityID: "eid",
			ProjectTeam: &ProjectTeam{ProjectNumber: "17", Team: "t"}}},
		Location:     "loc",
		StorageClass: "class",
		RetentionPolicy: &RetentionPolicy{
			RetentionPeriod: 3 * time.Second,
		},
		BucketPolicyOnly:         BucketPolicyOnly{Enabled: true},
		UniformBucketLevelAccess: UniformBucketLevelAccess{Enabled: true},
		PublicAccessPrevention:   PublicAccessPreventionEnforced,
		VersioningEnabled:        false,
		RPO:                      RPOAsyncTurbo,
		Created:                  time.Now(),
		Labels:                   map[string]string{"label": "value"},
		CORS: []CORS{
			{
				MaxAge:          time.Hour,
				Methods:         []string{"GET", "POST"},
				Origins:         []string{"*"},
				ResponseHeaders: []string{"FOO"},
			},
		},
		Encryption: &BucketEncryption{DefaultKMSKeyName: "key"},
		Logging:    &BucketLogging{LogBucket: "lb", LogObjectPrefix: "p"},
		Website:    &BucketWebsite{MainPageSuffix: "mps", NotFoundPage: "404"},
		Autoclass:  &Autoclass{Enabled: true},
		Lifecycle: Lifecycle{
			Rules: []LifecycleRule{{
				Action: LifecycleAction{
					Type: DeleteAction,
				},
				Condition: LifecycleCondition{
					AgeInDays: 10,
				},
			}},
		},
		// Below fields should be ignored.
		MetaGeneration: 39,
		Etag:           "Zkyw9ACJZUvcYmlFaKGChzhmtnE/dt1zHSfweiWpwzdGsqXwuJZqiD0",
	}
	got := attrs.toProtoBucket()
	want := &storagepb.Bucket{
		Name: "name",
		Acl: []*storagepb.BucketAccessControl{
			{Entity: "bob@example.com", Role: "OWNER"},
		},
		DefaultObjectAcl: []*storagepb.ObjectAccessControl{
			{Entity: "allUsers", Role: "READER"},
		},
		Location:     "loc",
		StorageClass: "class",
		RetentionPolicy: &storagepb.Bucket_RetentionPolicy{
			RetentionDuration: durationpb.New(3 * time.Second),
		},
		IamConfig: &storagepb.Bucket_IamConfig{
			UniformBucketLevelAccess: &storagepb.Bucket_IamConfig_UniformBucketLevelAccess{
				Enabled: true,
			},
			PublicAccessPrevention: "enforced",
		},
		Versioning: nil, // ignore VersioningEnabled if false
		Rpo:        rpoAsyncTurbo,
		Labels:     map[string]string{"label": "value"},
		Cors: []*storagepb.Bucket_Cors{
			{
				MaxAgeSeconds:  3600,
				Method:         []string{"GET", "POST"},
				Origin:         []string{"*"},
				ResponseHeader: []string{"FOO"},
			},
		},
		Encryption: &storagepb.Bucket_Encryption{DefaultKmsKey: "key"},
		Logging:    &storagepb.Bucket_Logging{LogBucket: "projects/_/buckets/lb", LogObjectPrefix: "p"},
		Website:    &storagepb.Bucket_Website{MainPageSuffix: "mps", NotFoundPage: "404"},
		Autoclass:  &storagepb.Bucket_Autoclass{Enabled: true},
		Lifecycle: &storagepb.Bucket_Lifecycle{
			Rule: []*storagepb.Bucket_Lifecycle_Rule{
				{
					Action: &storagepb.Bucket_Lifecycle_Rule_Action{Type: "Delete"},
					Condition: &storagepb.Bucket_Lifecycle_Rule_Condition{
						AgeDays:                 proto.Int32(int32(10)),
						NumNewerVersions:        proto.Int32(int32(0)),
						DaysSinceCustomTime:     proto.Int32(int32(0)),
						DaysSinceNoncurrentTime: proto.Int32(int32(0)),
					},
				},
			},
		},
	}

	if msg := testutil.Diff(got, want); msg != "" {
		t.Error(msg)
	}

	attrs.VersioningEnabled = true
	attrs.RequesterPays = true
	got = attrs.toProtoBucket()
	want.Versioning = &storagepb.Bucket_Versioning{Enabled: true}
	want.Billing = &storagepb.Bucket_Billing{RequesterPays: true}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Error(msg)
	}

	// Test that setting either of BucketPolicyOnly or UniformBucketLevelAccess
	// will enable UniformBucketLevelAccess.
	// Set UBLA.Enabled = true --> UBLA should be set to enabled in the proto.
	attrs.BucketPolicyOnly = BucketPolicyOnly{}
	attrs.UniformBucketLevelAccess = UniformBucketLevelAccess{Enabled: true}
	got = attrs.toProtoBucket()
	want.IamConfig = &storagepb.Bucket_IamConfig{
		UniformBucketLevelAccess: &storagepb.Bucket_IamConfig_UniformBucketLevelAccess{
			Enabled: true,
		},
		PublicAccessPrevention: "enforced",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Set BucketPolicyOnly.Enabled = true --> UBLA should be set to enabled in
	// the proto.
	attrs.BucketPolicyOnly = BucketPolicyOnly{Enabled: true}
	attrs.UniformBucketLevelAccess = UniformBucketLevelAccess{}
	got = attrs.toProtoBucket()
	want.IamConfig = &storagepb.Bucket_IamConfig{
		UniformBucketLevelAccess: &storagepb.Bucket_IamConfig_UniformBucketLevelAccess{
			Enabled: true,
		},
		PublicAccessPrevention: "enforced",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Set both BucketPolicyOnly.Enabled = true and
	// UniformBucketLevelAccess.Enabled=true --> UBLA should be set to enabled
	// in the proto.
	attrs.BucketPolicyOnly = BucketPolicyOnly{Enabled: true}
	attrs.UniformBucketLevelAccess = UniformBucketLevelAccess{Enabled: true}
	got = attrs.toProtoBucket()
	want.IamConfig = &storagepb.Bucket_IamConfig{
		UniformBucketLevelAccess: &storagepb.Bucket_IamConfig_UniformBucketLevelAccess{
			Enabled: true,
		},
		PublicAccessPrevention: "enforced",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Set UBLA.Enabled=false and BucketPolicyOnly.Enabled=false --> UBLA
	// should be disabled in the proto.
	attrs.BucketPolicyOnly = BucketPolicyOnly{}
	attrs.UniformBucketLevelAccess = UniformBucketLevelAccess{}
	got = attrs.toProtoBucket()
	want.IamConfig = &storagepb.Bucket_IamConfig{
		PublicAccessPrevention: "enforced",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Test that setting PublicAccessPrevention to "unspecified" leads to the
	// inherited setting being propagated in the proto.
	attrs.PublicAccessPrevention = PublicAccessPreventionUnspecified
	got = attrs.toProtoBucket()
	want.IamConfig = &storagepb.Bucket_IamConfig{
		PublicAccessPrevention: "inherited",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Test that setting PublicAccessPrevention to "inherited" leads to the
	// setting being propagated in the proto.
	attrs.PublicAccessPrevention = PublicAccessPreventionInherited
	got = attrs.toProtoBucket()
	want.IamConfig = &storagepb.Bucket_IamConfig{
		PublicAccessPrevention: "inherited",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Test that setting RPO to default is propagated in the proto.
	attrs.RPO = RPODefault
	got = attrs.toProtoBucket()
	want.Rpo = rpoDefault
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Re-enable UBLA and confirm that it does not affect the PAP setting.
	attrs.UniformBucketLevelAccess = UniformBucketLevelAccess{Enabled: true}
	got = attrs.toProtoBucket()
	want.IamConfig = &storagepb.Bucket_IamConfig{
		UniformBucketLevelAccess: &storagepb.Bucket_IamConfig_UniformBucketLevelAccess{
			Enabled: true,
		},
		PublicAccessPrevention: "inherited",
	}
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}

	// Disable UBLA and reset PAP to default. Confirm that the IAM config is set
	// to nil in the proto.
	attrs.UniformBucketLevelAccess = UniformBucketLevelAccess{Enabled: false}
	attrs.PublicAccessPrevention = PublicAccessPreventionUnknown
	got = attrs.toProtoBucket()
	want.IamConfig = nil
	if msg := testutil.Diff(got, want); msg != "" {
		t.Errorf(msg)
	}
}

func TestBucketRetryer(t *testing.T) {
	testCases := []struct {
		name string
		call func(b *BucketHandle) *BucketHandle
		want *retryConfig
	}{
		{
			name: "all defaults",
			call: func(b *BucketHandle) *BucketHandle {
				return b.Retryer()
			},
			want: &retryConfig{},
		},
		{
			name: "set all options",
			call: func(b *BucketHandle) *BucketHandle {
				return b.Retryer(
					WithBackoff(gax.Backoff{
						Initial:    2 * time.Second,
						Max:        30 * time.Second,
						Multiplier: 3,
					}),
					WithPolicy(RetryAlways),
					WithErrorFunc(func(err error) bool { return false }))
			},
			want: &retryConfig{
				backoff: &gax.Backoff{
					Initial:    2 * time.Second,
					Max:        30 * time.Second,
					Multiplier: 3,
				},
				policy:      RetryAlways,
				shouldRetry: func(err error) bool { return false },
			},
		},
		{
			name: "set some backoff options",
			call: func(b *BucketHandle) *BucketHandle {
				return b.Retryer(
					WithBackoff(gax.Backoff{
						Multiplier: 3,
					}))
			},
			want: &retryConfig{
				backoff: &gax.Backoff{
					Multiplier: 3,
				}},
		},
		{
			name: "set policy only",
			call: func(b *BucketHandle) *BucketHandle {
				return b.Retryer(WithPolicy(RetryNever))
			},
			want: &retryConfig{
				policy: RetryNever,
			},
		},
		{
			name: "set ErrorFunc only",
			call: func(b *BucketHandle) *BucketHandle {
				return b.Retryer(
					WithErrorFunc(func(err error) bool { return false }))
			},
			want: &retryConfig{
				shouldRetry: func(err error) bool { return false },
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(s *testing.T) {
			b := tc.call(&BucketHandle{})
			if diff := cmp.Diff(
				b.retry,
				tc.want,
				cmp.AllowUnexported(retryConfig{}, gax.Backoff{}),
				// ErrorFunc cannot be compared directly, but we check if both are
				// either nil or non-nil.
				cmp.Comparer(func(a, b func(err error) bool) bool {
					return (a == nil && b == nil) || (a != nil && b != nil)
				}),
			); diff != "" {
				s.Fatalf("retry not configured correctly: %v", diff)
			}
		})
	}
}

func TestDetectDefaultGoogleAccessID(t *testing.T) {
	testCases := []struct {
		name           string
		serviceAccount string
		creds          func(string) string
		expectSuccess  bool
	}{
		{
			name:           "impersonated creds",
			serviceAccount: "default@my-project.iam.gserviceaccount.com",
			creds: func(sa string) string {
				return fmt.Sprintf(`{
					"delegates": [],
					"service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken",
					"source_credentials": {
					  "client_id": "id",
					  "client_secret": "secret",
					  "refresh_token": "token",
					  "type": "authorized_user"
					},
					"type": "impersonated_service_account"
				  }`, sa)
			},
			expectSuccess: true,
		},
		{
			name:           "gcloud ADC creds",
			serviceAccount: "default@my-project.iam.gserviceaccount.com",
			creds: func(sa string) string {
				return fmt.Sprint(`{
					"client_id": "my-id.apps.googleusercontent.com",
					"client_secret": "secret",
					"quota_project_id": "",
					"refresh_token": "token",
					"type": "authorized_user"
				}`)
			},
			expectSuccess: false,
		},
		{
			name:           "ADC private key",
			serviceAccount: "default@my-project.iam.gserviceaccount.com",
			creds: func(sa string) string {
				return fmt.Sprintf(`{
					"type": "service_account",
					"project_id": "my-project",
					"private_key_id": "my1",
					"private_key": "-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----\n",
					"client_email": "%s",
					"client_id": "01",
					"auth_uri": "https://accounts.google.com/o/oauth2/auth",
					"token_uri": "https://oauth2.googleapis.com/token",
					"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
					"client_x509_cert_url": "cert"
				}`, sa)
			},
			expectSuccess: true,
		},
		{
			name: "no creds",
			creds: func(_ string) string {
				return ""
			},
			expectSuccess: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bucket := BucketHandle{
				c: &Client{
					creds: &google.Credentials{
						JSON: []byte(tc.creds(tc.serviceAccount)),
					},
				},
				name: "my-bucket",
			}

			id, err := bucket.detectDefaultGoogleAccessID()
			if tc.expectSuccess {
				if err != nil {
					t.Fatal(err)
				}
				if id != tc.serviceAccount {
					t.Errorf("service account not found correctly; got: %s, want: %s", id, tc.serviceAccount)
				}
			} else if err == nil {
				t.Error("expected error but detectDefaultGoogleAccessID did not return one")
			}
		})
	}
}
