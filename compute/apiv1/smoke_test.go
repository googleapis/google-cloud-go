// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// To run these tests, set GCLOUD_TESTS_GOLANG_PROJECT_ID env var to your GCP projectID

package compute

import (
	"context"
	"fmt"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
	"google.golang.org/protobuf/proto"
)

var projectId = testutil.ProjID()
var defaultZone = "us-central1-a"

func TestCreateGetListInstance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}
	space := uid.NewSpace("gogapic", nil)
	name := space.New()
	ctx := context.Background()
	c, err := NewInstancesRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	zonesClient, err := NewZoneOperationsRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	createRequest := &computepb.InsertInstanceRequest{
		Project: projectId,
		Zone:    defaultZone,
		InstanceResource: &computepb.Instance{
			Name:        &name,
			Description: proto.String("тест"),
			MachineType: proto.String(fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/machineTypes/n1-standard-1", projectId, defaultZone)),
			Disks: []*computepb.AttachedDisk{
				{
					AutoDelete: proto.Bool(true),
					Boot:       proto.Bool(true),
					Type:       computepb.AttachedDisk_PERSISTENT.Enum(),
					InitializeParams: &computepb.AttachedDiskInitializeParams{
						SourceImage: proto.String("projects/debian-cloud/global/images/family/debian-10"),
					},
				},
			},
			NetworkInterfaces: []*computepb.NetworkInterface{
				{
					AccessConfigs: []*computepb.AccessConfig{
						{
							Name: proto.String("default"),
						},
					},
				},
			},
		},
	}

	insert, err := c.Insert(ctx, createRequest)
	if err != nil {
		t.Fatal(err)
	}

	waitZonalRequest := &computepb.WaitZoneOperationRequest{
		Project:   projectId,
		Zone:      defaultZone,
		Operation: insert.GetName(),
	}
	_, err = zonesClient.Wait(ctx, waitZonalRequest)
	if err != nil {
		t.Error(err)
	}
	defer ForceDeleteInstance(ctx, name, c)

	getRequest := &computepb.GetInstanceRequest{
		Project:  projectId,
		Zone:     defaultZone,
		Instance: name,
	}
	get, err := c.Get(ctx, getRequest)
	if err != nil {
		t.Error(err)
	}
	if get.GetName() != name {
		t.Fatal(fmt.Sprintf("expected instance name: %s, got: %s", name, get.GetName()))
	}
	if get.GetDescription() != "тест" {
		t.Fatal(fmt.Sprintf("expected instance description: %s, got: %s", "тест", get.GetDescription()))
	}
	listRequest := &computepb.ListInstancesRequest{
		Project: projectId,
		Zone:    defaultZone,
	}

	list, err := c.List(ctx, listRequest)
	if err != nil {
		t.Error(err)
	}
	items := list.GetItems()
	found := false
	for _, element := range items {
		if element.GetName() == name {
			found = true
		}
	}
	if !found {
		t.Error("Couldn't find the instance in list response")
	}

	deleteInstanceRequest := &computepb.DeleteInstanceRequest{
		Project:  projectId,
		Zone:     defaultZone,
		Instance: name,
	}
	_, err = c.Delete(ctx, deleteInstanceRequest)
	if err != nil {
		t.Error(err)
	}
}

func ForceDeleteInstance(ctx context.Context, name string, client *InstancesClient) {
	deleteInstanceRequest := &computepb.DeleteInstanceRequest{
		Project:  projectId,
		Zone:     defaultZone,
		Instance: name,
	}
	client.Delete(ctx, deleteInstanceRequest)
}

func TestCreateGetRemoveSecurityPolicies(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}
	space := uid.NewSpace("gogapic", nil)
	name := space.New()
	ctx := context.Background()
	c, err := NewSecurityPoliciesRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	globalCLient, err := NewGlobalOperationsRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	action := "allow"
	matcher := &computepb.SecurityPolicyRuleMatcher{
		Config: &computepb.SecurityPolicyRuleMatcherConfig{
			SrcIpRanges: []string{
				"*",
			},
		},
		VersionedExpr: computepb.SecurityPolicyRuleMatcher_SRC_IPS_V1.Enum(),
	}
	securityPolicyRule := &computepb.SecurityPolicyRule{
		Action:      &action,
		Priority:    proto.Int32(0),
		Description: proto.String("test rule"),
		Match:       matcher,
	}
	securityPolicyRuleDefault := &computepb.SecurityPolicyRule{
		Action:      &action,
		Priority:    proto.Int32(2147483647),
		Description: proto.String("default rule"),
		Match:       matcher,
	}
	insertRequest := &computepb.InsertSecurityPolicyRequest{
		Project: projectId,
		SecurityPolicyResource: &computepb.SecurityPolicy{
			Name: &name,
			Rules: []*computepb.SecurityPolicyRule{
				securityPolicyRule,
				securityPolicyRuleDefault,
			},
		},
	}
	insert, err := c.Insert(ctx, insertRequest)
	if err != nil {
		t.Fatal(err)
	}

	waitGlobalRequest := &computepb.WaitGlobalOperationRequest{
		Project:   projectId,
		Operation: insert.GetName(),
	}
	_, err = globalCLient.Wait(ctx, waitGlobalRequest)
	if err != nil {
		t.Error(err)
	}
	defer ForceDeleteSecurityPolicy(ctx, name, c)

	removeRuleRequest := &computepb.RemoveRuleSecurityPolicyRequest{
		Priority:       proto.Int32(0),
		Project:        projectId,
		SecurityPolicy: name,
	}

	rule, err := c.RemoveRule(ctx, removeRuleRequest)
	if err != nil {
		t.Error(err)
	}
	waitGlobalRequestRemove := &computepb.WaitGlobalOperationRequest{
		Project:   projectId,
		Operation: rule.GetName(),
	}
	_, err = globalCLient.Wait(ctx, waitGlobalRequestRemove)
	if err != nil {
		t.Error(err)
	}

	getRequest := &computepb.GetSecurityPolicyRequest{
		Project:        projectId,
		SecurityPolicy: name,
	}
	get, err := c.Get(ctx, getRequest)
	if err != nil {
		t.Error(err)
	}
	if len(get.GetRules()) != 1 {
		t.Fatal(fmt.Sprintf("expected count for rules: %d, got: %d", 1, len(get.GetRules())))
	}

	deleteRequest := &computepb.DeleteSecurityPolicyRequest{
		Project:        projectId,
		SecurityPolicy: name,
	}
	_, err = c.Delete(ctx, deleteRequest)
	if err != nil {
		t.Error(err)
	}
}

func ForceDeleteSecurityPolicy(ctx context.Context, name string, client *SecurityPoliciesClient) {
	deleteRequest := &computepb.DeleteSecurityPolicyRequest{
		Project:        projectId,
		SecurityPolicy: name,
	}
	client.Delete(ctx, deleteRequest)
}
