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
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/google/go-cmp/cmp"
	gax "github.com/googleapis/gax-go/v2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
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

// TestBucketSignedURL_Endpoint_Emulator_Host tests that Bucket.SignedURl
// respects the host set in STORAGE_EMULATOR_HOST and/or in option.WithEndpoint
// TODO: move this testing to conformance tests.
func TestBucketSignedURL_Endpoint_Emulator_Host(t *testing.T) {
	expires, _ := time.Parse(time.RFC3339, "2002-10-02T10:00:00-05:00")
	bucketName := "bucket-name"
	objectName := "obj-name"

	localhost9000 := "localhost:9000"
	localhost6000Https := "https://localhost:6000"

	tests := []struct {
		desc         string
		emulatorHost string
		endpoint     *string
		now          time.Time
		opts         *SignedURLOptions
		// Note for future implementors: X-Goog-Signature generated by having
		// the client run through its algorithm with pre-defined input and copy
		// pasting the output. These tests are not great for testing whether
		// the right signature is calculated - instead we rely on the backend
		// and integration tests for that.
		want string
	}{
		{
			desc:         "SignURLV4 creates link to resources in emulator",
			emulatorHost: localhost9000,
			now:          expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV4,
				Insecure:       true,
			},
			want: "http://localhost:9000/" + bucketName + "/" + objectName +
				"?X-Goog-Algorithm=GOOG4-RSA-SHA256" +
				"&X-Goog-Credential=xxx%40clientid%2F20021001%2Fauto%2Fstorage%2Fgoog4_request" +
				"&X-Goog-Date=20021001T100000Z&X-Goog-Expires=86400" +
				"&X-Goog-Signature=2ff5ff0e5f336c4f2e4a44b93673ea22c6f94153da070206077328ce9f33b51d668549454668e6a784fe99110e506d504d7199015e34b22f8faa3e5eee294a71d8729e55debe7d24fbc336193e217373124ec69db19d447c8b649b6ca0734a76cebe33e9ccacbe462cdf2dacb30809846a81f1f48c654eed45ddd26eb787947760d82fb5098d34e3aaa6d4a0b0b8b444a12436d1456b96bcd8a2acc5b74a948a42216a1f842802a0d41391fe9acc97744eb1a848f596d3284f95a56f134cd6b78387efbd514ae7d2b98e62241cf6466e7493822184e0bd192dee62dad2d1449bc9c8fed2e84ddfa26996a0c5a9238cf675bb4ffec05cdcec07cc57d272357fd2" +
				"&X-Goog-SignedHeaders=host",
		},
		{
			desc:     "SignURLV4 - endpoint",
			endpoint: &localhost9000,
			now:      expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV4,
				Insecure:       true,
			},
			want: "http://localhost:9000/" + bucketName + "/" + objectName +
				"?X-Goog-Algorithm=GOOG4-RSA-SHA256" +
				"&X-Goog-Credential=xxx%40clientid%2F20021001%2Fauto%2Fstorage%2Fgoog4_request" +
				"&X-Goog-Date=20021001T100000Z&X-Goog-Expires=86400" +
				"&X-Goog-Signature=2ff5ff0e5f336c4f2e4a44b93673ea22c6f94153da070206077328ce9f33b51d668549454668e6a784fe99110e506d504d7199015e34b22f8faa3e5eee294a71d8729e55debe7d24fbc336193e217373124ec69db19d447c8b649b6ca0734a76cebe33e9ccacbe462cdf2dacb30809846a81f1f48c654eed45ddd26eb787947760d82fb5098d34e3aaa6d4a0b0b8b444a12436d1456b96bcd8a2acc5b74a948a42216a1f842802a0d41391fe9acc97744eb1a848f596d3284f95a56f134cd6b78387efbd514ae7d2b98e62241cf6466e7493822184e0bd192dee62dad2d1449bc9c8fed2e84ddfa26996a0c5a9238cf675bb4ffec05cdcec07cc57d272357fd2" +
				"&X-Goog-SignedHeaders=host",
		},
		{
			desc:         "SignURLV4 - endpoint takes precedence over emulator",
			endpoint:     &localhost9000,
			emulatorHost: "localhost:8000",
			now:          expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV4,
				Insecure:       true,
			},
			want: "http://localhost:9000/" + bucketName + "/" + objectName +
				"?X-Goog-Algorithm=GOOG4-RSA-SHA256" +
				"&X-Goog-Credential=xxx%40clientid%2F20021001%2Fauto%2Fstorage%2Fgoog4_request" +
				"&X-Goog-Date=20021001T100000Z&X-Goog-Expires=86400" +
				"&X-Goog-Signature=2ff5ff0e5f336c4f2e4a44b93673ea22c6f94153da070206077328ce9f33b51d668549454668e6a784fe99110e506d504d7199015e34b22f8faa3e5eee294a71d8729e55debe7d24fbc336193e217373124ec69db19d447c8b649b6ca0734a76cebe33e9ccacbe462cdf2dacb30809846a81f1f48c654eed45ddd26eb787947760d82fb5098d34e3aaa6d4a0b0b8b444a12436d1456b96bcd8a2acc5b74a948a42216a1f842802a0d41391fe9acc97744eb1a848f596d3284f95a56f134cd6b78387efbd514ae7d2b98e62241cf6466e7493822184e0bd192dee62dad2d1449bc9c8fed2e84ddfa26996a0c5a9238cf675bb4ffec05cdcec07cc57d272357fd2" +
				"&X-Goog-SignedHeaders=host",
		},
		{
			desc:         "SigningSchemeV2 - emulator",
			emulatorHost: "localhost:8000",
			now:          expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV2,
			},
			want: "https://localhost:8000/" + bucketName + "/" + objectName +
				"?Expires=1033570800" +
				"&GoogleAccessId=xxx%40clientid" +
				"&Signature=oRi3y2tBTmoDto7FezNx4AjC0RXA6fpJjTBa0hINeVroZ%2ByOeRU8MRwJbKg1IkBbV0IjtlPaGwv5YoUH16UYdipBjCXOS%2B1qgRWyzl8AnzvU%2BfwSXSlCk9zPtHHoBkFT7G4cZQOdDTLRrSG%2FmRJ3K09KEHYg%2Fc6R5Dd92inD1tLE2tiFMyHFs5uQHRMsepY4wrWiIQ4u53tPvk%2Fwiq1%2B9yL6x3QGblhdWwjX0BTVBOxexyKTlwczJW0XlWX8wpcTFfzQnJZuujbhanf2g9MGzSmkv3ylyuQdHMJDYp4Bzq%2FmnkNUg0Vp6iEvh9tyVdRNkwXeg3D8qn%2BFSOxcF%2B9vJw%3D%3D",
		},
		{
			desc:         "SigningSchemeV2 - endpoint",
			emulatorHost: "localhost:8000",
			endpoint:     &localhost9000,
			now:          expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV2,
			},
			want: "https://localhost:9000/" + bucketName + "/" + objectName +
				"?Expires=1033570800" +
				"&GoogleAccessId=xxx%40clientid" +
				"&Signature=oRi3y2tBTmoDto7FezNx4AjC0RXA6fpJjTBa0hINeVroZ%2ByOeRU8MRwJbKg1IkBbV0IjtlPaGwv5YoUH16UYdipBjCXOS%2B1qgRWyzl8AnzvU%2BfwSXSlCk9zPtHHoBkFT7G4cZQOdDTLRrSG%2FmRJ3K09KEHYg%2Fc6R5Dd92inD1tLE2tiFMyHFs5uQHRMsepY4wrWiIQ4u53tPvk%2Fwiq1%2B9yL6x3QGblhdWwjX0BTVBOxexyKTlwczJW0XlWX8wpcTFfzQnJZuujbhanf2g9MGzSmkv3ylyuQdHMJDYp4Bzq%2FmnkNUg0Vp6iEvh9tyVdRNkwXeg3D8qn%2BFSOxcF%2B9vJw%3D%3D",
		},
		{
			desc:         "VirtualHostedStyle - emulator",
			emulatorHost: "localhost:8000",
			now:          expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV4,
				Style:          VirtualHostedStyle(),
				Insecure:       true,
			},
			want: "http://" + bucketName + ".localhost:8000/" + objectName +
				"?X-Goog-Algorithm=GOOG4-RSA-SHA256" +
				"&X-Goog-Credential=xxx%40clientid%2F20021001%2Fauto%2Fstorage%2Fgoog4_request" +
				"&X-Goog-Date=20021001T100000Z&X-Goog-Expires=86400" +
				"&X-Goog-Signature=9163ad1bfb8ca4c70aff3bc6ee5b2895d8fc6946f28ade641824c40efed922ec1f42c100ab98192f6db955620bf35f660fa6da0974a35d5599d56583f4dd8f9f8441b8dd70ebb3557a742db5d619e9c950b8b397da76317aeee4409c25dd8ac1af0454d331d49b6c3fc4b6118ddcf570154f3455d616c737e0b5891de7758dea438f734e1124e78ebc7bad657d68f9003f282e14f8c5dceb97f441efad70ff2f76eab89537b05bdf0fbb50d87c34e7583028979b87793d9bc1902f44d6e4b4c4564bc457b430584881b8ee4e8995fcca4e6050c4c28609c5d0026a3a4b2fc0121dcb11833c872e5bf9f154f8be582a65ad6f52b5bd2cf052f23fadd293f8362e" +
				"&X-Goog-SignedHeaders=host",
		},
		{
			desc:         "VirtualHostedStyle - endpoint does not have effect",
			emulatorHost: "localhost:9000",
			endpoint:     &localhost6000Https,
			now:          expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV4,
				Style:          VirtualHostedStyle(),
				Insecure:       true,
			},
			want: "http://" + bucketName + ".localhost:9000/" + objectName +
				"?X-Goog-Algorithm=GOOG4-RSA-SHA256" +
				"&X-Goog-Credential=xxx%40clientid%2F20021001%2Fauto%2Fstorage%2Fgoog4_request" +
				"&X-Goog-Date=20021001T100000Z&X-Goog-Expires=86400" +
				"&X-Goog-Signature=59577491e93738f05f2442166c787fdf6f5ad70619cd87d7ac2590b78f31c01cd3cef1948fd9150de756a834de0e7cf843d82b36f744fe8cf3c095fbb35205f8db17ddaa72925a7aae9179ebf811519ab04a9e778cc1a388fa961869f01f0ab6b70e8c4e39730752f8874d29809db15209a062df90c42b536237d371f1397916008a97db70c9cd011965f843e8ddbaf2d20aed4a213e71938aeb5c88411bdf40444ae6b07f34ee2fa91825c32f136a7e7411d33dda3d34260d61694a50f5a13d1187490d1a9301aaef745ec78db7620beb4501fd76bf54d989d227ca29d01e76c50fbb3cf796e1987c18c732a14f1c4c36fa630514f6edf029546a129136d28c" +
				"&X-Goog-SignedHeaders=host",
		},
		{
			desc:         "BucketBoundHostname - emulator",
			emulatorHost: "localhost:8000",
			now:          expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV4,
				Style:          BucketBoundHostname("myhost"),
			},
			want: "https://" + "myhost/" + objectName +
				"?X-Goog-Algorithm=GOOG4-RSA-SHA256" +
				"&X-Goog-Credential=xxx%40clientid%2F20021001%2Fauto%2Fstorage%2Fgoog4_request" +
				"&X-Goog-Date=20021001T100000Z&X-Goog-Expires=86400" +
				"&X-Goog-Signature=15fe19f6c61bcbdbd6473c32f2bec29caa8a5fa3b2ce32cfb5329a71edaa0d4e5ffe6469f32ed4c23ca2fbed3882fdf1ed107c6a98c2c4995dda6036c64bae51e6cb542c353618f483832aa1f3ef85342ddadd69c13ad4c69fd3f573ea5cf325a58056e3d5a37005217662af63b49fef8688de3c5c7a2f7b43651a030edd0813eb7f7713989a4c29a8add65133ce652895fea9de7dbc6248ee11b4d7c6c1e152df87700100e896e544ba8eeea96584078f56e699665140b750e90550b9b79633f4e7c8409efa807be5670d6e987eeee04a4180be9b9e30bb8557597beaf390a3805cc602c87a3e34800f8bc01449c3dd10ac2f2263e55e55b91e445052548d5e" +
				"&X-Goog-SignedHeaders=host",
		},
		{
			desc:     "BucketBoundHostname - endpoint",
			endpoint: &localhost9000,
			now:      expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV4,
				Style:          BucketBoundHostname("myhost"),
			},
			want: "https://" + "myhost/" + objectName +
				"?X-Goog-Algorithm=GOOG4-RSA-SHA256" +
				"&X-Goog-Credential=xxx%40clientid%2F20021001%2Fauto%2Fstorage%2Fgoog4_request" +
				"&X-Goog-Date=20021001T100000Z&X-Goog-Expires=86400" +
				"&X-Goog-Signature=15fe19f6c61bcbdbd6473c32f2bec29caa8a5fa3b2ce32cfb5329a71edaa0d4e5ffe6469f32ed4c23ca2fbed3882fdf1ed107c6a98c2c4995dda6036c64bae51e6cb542c353618f483832aa1f3ef85342ddadd69c13ad4c69fd3f573ea5cf325a58056e3d5a37005217662af63b49fef8688de3c5c7a2f7b43651a030edd0813eb7f7713989a4c29a8add65133ce652895fea9de7dbc6248ee11b4d7c6c1e152df87700100e896e544ba8eeea96584078f56e699665140b750e90550b9b79633f4e7c8409efa807be5670d6e987eeee04a4180be9b9e30bb8557597beaf390a3805cc602c87a3e34800f8bc01449c3dd10ac2f2263e55e55b91e445052548d5e" +
				"&X-Goog-SignedHeaders=host",
		},
		{
			desc:         "emulator host specifies scheme",
			emulatorHost: "https://localhost:6000",
			now:          expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV4,
				Insecure:       true,
			},
			want: "http://localhost:6000/" + bucketName + "/" + objectName +
				"?X-Goog-Algorithm=GOOG4-RSA-SHA256" +
				"&X-Goog-Credential=xxx%40clientid%2F20021001%2Fauto%2Fstorage%2Fgoog4_request" +
				"&X-Goog-Date=20021001T100000Z&X-Goog-Expires=86400" +
				"&X-Goog-Signature=1bdbbc7e8db59e51ae2e6593fb326d9b1aa49a0905c6b94ee5bcb3be9e329656c07564a14e209275c95065f752695bd394d09afadb6874c0c0121799482f6f496593a87cdce48afd3c125b18054730273727075845e0b7d64e90503ffb20e6b02d2609bb596b081ce994ab4aafa35ee0a53350a994329e73a0125bb0edc955792f942ea8a9df5f5e87adcda4be5005dfb0d44915dee708815ac1d023c760379a22bc3d43983a672cf06c664b81bf1b724525bc1d0b2a89649c5ca396abf817ff5543f113933eb9f009fc655508656bf0d4017b2f5412028d144ef782c7b64162471c3a518053bf488ad382db3b3806316d903fa94d8b247b910aea4aa109cc55" +
				"&X-Goog-SignedHeaders=host",
		},
		{
			desc:     "endpoint specifies scheme",
			endpoint: &localhost6000Https,
			now:      expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV4,
				Insecure:       true,
			},
			want: "http://localhost:6000/" + bucketName + "/" + objectName +
				"?X-Goog-Algorithm=GOOG4-RSA-SHA256" +
				"&X-Goog-Credential=xxx%40clientid%2F20021001%2Fauto%2Fstorage%2Fgoog4_request" +
				"&X-Goog-Date=20021001T100000Z&X-Goog-Expires=86400" +
				"&X-Goog-Signature=1bdbbc7e8db59e51ae2e6593fb326d9b1aa49a0905c6b94ee5bcb3be9e329656c07564a14e209275c95065f752695bd394d09afadb6874c0c0121799482f6f496593a87cdce48afd3c125b18054730273727075845e0b7d64e90503ffb20e6b02d2609bb596b081ce994ab4aafa35ee0a53350a994329e73a0125bb0edc955792f942ea8a9df5f5e87adcda4be5005dfb0d44915dee708815ac1d023c760379a22bc3d43983a672cf06c664b81bf1b724525bc1d0b2a89649c5ca396abf817ff5543f113933eb9f009fc655508656bf0d4017b2f5412028d144ef782c7b64162471c3a518053bf488ad382db3b3806316d903fa94d8b247b910aea4aa109cc55" +
				"&X-Goog-SignedHeaders=host",
		},
		{
			desc:         "emulator host specifies scheme using SigningSchemeV2",
			emulatorHost: "https://localhost:8000",
			now:          expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV2,
			},
			want: "https://localhost:8000/" + bucketName + "/" + objectName +
				"?Expires=1033570800" +
				"&GoogleAccessId=xxx%40clientid" +
				"&Signature=oRi3y2tBTmoDto7FezNx4AjC0RXA6fpJjTBa0hINeVroZ%2ByOeRU8MRwJbKg1IkBbV0IjtlPaGwv5YoUH16UYdipBjCXOS%2B1qgRWyzl8AnzvU%2BfwSXSlCk9zPtHHoBkFT7G4cZQOdDTLRrSG%2FmRJ3K09KEHYg%2Fc6R5Dd92inD1tLE2tiFMyHFs5uQHRMsepY4wrWiIQ4u53tPvk%2Fwiq1%2B9yL6x3QGblhdWwjX0BTVBOxexyKTlwczJW0XlWX8wpcTFfzQnJZuujbhanf2g9MGzSmkv3ylyuQdHMJDYp4Bzq%2FmnkNUg0Vp6iEvh9tyVdRNkwXeg3D8qn%2BFSOxcF%2B9vJw%3D%3D",
		},
		{
			desc:     "endpoint specifies scheme using SigningSchemeV2",
			endpoint: &localhost6000Https,
			now:      expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV2,
			},
			want: "https://localhost:6000/" + bucketName + "/" + objectName +
				"?Expires=1033570800" +
				"&GoogleAccessId=xxx%40clientid" +
				"&Signature=oRi3y2tBTmoDto7FezNx4AjC0RXA6fpJjTBa0hINeVroZ%2ByOeRU8MRwJbKg1IkBbV0IjtlPaGwv5YoUH16UYdipBjCXOS%2B1qgRWyzl8AnzvU%2BfwSXSlCk9zPtHHoBkFT7G4cZQOdDTLRrSG%2FmRJ3K09KEHYg%2Fc6R5Dd92inD1tLE2tiFMyHFs5uQHRMsepY4wrWiIQ4u53tPvk%2Fwiq1%2B9yL6x3QGblhdWwjX0BTVBOxexyKTlwczJW0XlWX8wpcTFfzQnJZuujbhanf2g9MGzSmkv3ylyuQdHMJDYp4Bzq%2FmnkNUg0Vp6iEvh9tyVdRNkwXeg3D8qn%2BFSOxcF%2B9vJw%3D%3D",
		},
		{
			desc:         "hostname in opts overrides all else",
			endpoint:     &localhost9000,
			emulatorHost: "https://localhost:8000",
			now:          expires.Add(-24 * time.Hour),
			opts: &SignedURLOptions{
				GoogleAccessID: "xxx@clientid",
				PrivateKey:     dummyKey("rsa"),
				Method:         "POST",
				Expires:        expires,
				Scheme:         SigningSchemeV2,
				Hostname:       "localhost:6000",
			},
			want: "https://localhost:6000/" + bucketName + "/" + objectName +
				"?Expires=1033570800" +
				"&GoogleAccessId=xxx%40clientid" +
				"&Signature=oRi3y2tBTmoDto7FezNx4AjC0RXA6fpJjTBa0hINeVroZ%2ByOeRU8MRwJbKg1IkBbV0IjtlPaGwv5YoUH16UYdipBjCXOS%2B1qgRWyzl8AnzvU%2BfwSXSlCk9zPtHHoBkFT7G4cZQOdDTLRrSG%2FmRJ3K09KEHYg%2Fc6R5Dd92inD1tLE2tiFMyHFs5uQHRMsepY4wrWiIQ4u53tPvk%2Fwiq1%2B9yL6x3QGblhdWwjX0BTVBOxexyKTlwczJW0XlWX8wpcTFfzQnJZuujbhanf2g9MGzSmkv3ylyuQdHMJDYp4Bzq%2FmnkNUg0Vp6iEvh9tyVdRNkwXeg3D8qn%2BFSOxcF%2B9vJw%3D%3D",
		},
	}
	oldUTCNow := utcNow
	defer func() {
		utcNow = oldUTCNow
	}()

	for _, test := range tests {
		t.Run(test.desc, func(s *testing.T) {
			utcNow = func() time.Time {
				return test.now
			}

			t.Setenv("STORAGE_EMULATOR_HOST", test.emulatorHost)

			var opts []option.ClientOption
			if test.endpoint != nil {
				opts = append(opts, option.WithEndpoint(*test.endpoint))
			}
			c, err := NewClient(context.Background(), opts...)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}

			got, err := c.Bucket(bucketName).SignedURL(objectName, test.opts)
			if err != nil {
				s.Fatal(err)
			}

			if got != test.want {
				s.Fatalf("bucket.SidnedURL:\n\tgot:\t%v\n\twant:\t%v", got, test.want)
			}
		})
	}
}
