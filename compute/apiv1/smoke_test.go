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
	"cloud.google.com/go/internal"
	"context"
	"fmt"
	"github.com/googleapis/gax-go/v2"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"google.golang.org/api/iterator"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
	"google.golang.org/protobuf/proto"
)

var projectId = testutil.ProjID()
var defaultZone = "us-central1-a"

func TestCreateGetPutPatchListInstance(t *testing.T) {
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
		Operation: insert.Proto().GetName(),
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
		t.Fatalf("expected instance name: %s, got: %s", name, get.GetName())
	}
	if get.GetDescription() != "тест" {
		t.Fatalf("expected instance description: %s, got: %s", "тест", get.GetDescription())
	}
	if secureBootEnabled := get.GetShieldedInstanceConfig().GetEnableSecureBoot(); secureBootEnabled {
		t.Fatalf("expected instance secure boot: %t, got: %t", false, get.GetShieldedInstanceConfig().GetEnableSecureBoot())
	}

	get.Description = proto.String("updated")
	updateRequest := &computepb.UpdateInstanceRequest{
		Instance:         name,
		InstanceResource: get,
		Project:          projectId,
		Zone:             defaultZone,
	}
	updateOp, err := c.Update(ctx, updateRequest)
	if err != nil {
		t.Error(err)
	}
	_, err = zonesClient.Wait(ctx, &computepb.WaitZoneOperationRequest{
		Project:   projectId,
		Zone:      defaultZone,
		Operation: updateOp.Proto().GetName(),
	})
	if err != nil {
		t.Error(err)
	}

	patchReq := &computepb.UpdateShieldedInstanceConfigInstanceRequest{
		Instance: name,
		Project:  projectId,
		Zone:     defaultZone,
		ShieldedInstanceConfigResource: &computepb.ShieldedInstanceConfig{
			EnableSecureBoot: proto.Bool(true),
		},
	}
	patchOp, err := c.UpdateShieldedInstanceConfig(ctx, patchReq)
	if err != nil {
		return
	}
	_, err = zonesClient.Wait(ctx, &computepb.WaitZoneOperationRequest{
		Project:   projectId,
		Zone:      defaultZone,
		Operation: patchOp.Proto().GetName(),
	})
	if err != nil {
		t.Error(err)
	}

	fetched, err := c.Get(ctx, getRequest)
	if err != nil {
		t.Error(err)
	}
	if fetched.GetDescription() != "updated" {
		t.Fatal(fmt.Sprintf("expected instance description: %s, got: %s", "updated", fetched.GetDescription()))
	}
	if secureBootEnabled := fetched.GetShieldedInstanceConfig().GetEnableSecureBoot(); !secureBootEnabled {
		t.Fatal(fmt.Sprintf("expected instance secure boot: %t, got: %t", true, secureBootEnabled))
	}
	listRequest := &computepb.ListInstancesRequest{
		Project: projectId,
		Zone:    defaultZone,
	}

	itr := c.List(ctx, listRequest)
	if err != nil {
		t.Error(err)
	}
	found := false
	element, err := itr.Next()
	for err == nil {
		if element.GetName() == name {
			found = true
		}
		element, err = itr.Next()
	}
	if err != nil && err != iterator.Done {
		t.Fatal(err)
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
		Operation: insert.Proto().GetName(),
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
		Operation: rule.Proto().GetName(),
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

func TestPaginationWithMaxRes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}
	ctx := context.Background()
	c, err := NewAcceleratorTypesRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	req := &computepb.ListAcceleratorTypesRequest{
		Project:    projectId,
		Zone:       defaultZone,
		MaxResults: proto.Uint32(1),
	}
	itr := c.List(ctx, req)

	found := false
	element, err := itr.Next()
	for err == nil {
		if element.GetName() == "nvidia-tesla-t4" {
			found = true
			break
		}
		element, err = itr.Next()
	}
	if err != nil && err != iterator.Done {
		t.Fatal(err)
	}
	if !found {
		t.Error("Couldn't find the accelerator in the response")
	}
}

func TestPaginationDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}
	ctx := context.Background()
	c, err := NewAcceleratorTypesRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	req := &computepb.ListAcceleratorTypesRequest{
		Project: projectId,
		Zone:    defaultZone,
	}
	itr := c.List(ctx, req)

	found := false
	element, err := itr.Next()
	for err == nil {
		if element.GetName() == "nvidia-tesla-t4" {
			found = true
			break
		}
		element, err = itr.Next()
	}
	if err != nil && err != iterator.Done {
		t.Fatal(err)
	}
	if !found {
		t.Error("Couldn't find the accelerator in the response")
	}
}

func TestPaginationMapResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}
	ctx := context.Background()
	c, err := NewAcceleratorTypesRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	req := &computepb.AggregatedListAcceleratorTypesRequest{
		Project: projectId,
	}
	itr := c.AggregatedList(ctx, req)

	found := false
	element, err := itr.Next()
	for err == nil {
		if element.Key == "zones/us-central1-a" {
			types := element.Value.GetAcceleratorTypes()
			for _, item := range types {
				if item.GetName() == "nvidia-tesla-t4" {
					found = true
					break
				}
			}
		}
		element, err = itr.Next()
	}
	if err != iterator.Done {
		t.Fatal(err)
	}
	if !found {
		t.Error("Couldn't find the accelerator in the response")
	}
}

func TestPaginationMapResponseMaxRes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}
	ctx := context.Background()
	c, err := NewAcceleratorTypesRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	req := &computepb.AggregatedListAcceleratorTypesRequest{
		Project:    projectId,
		MaxResults: proto.Uint32(10),
	}
	itr := c.AggregatedList(ctx, req)
	found := false
	element, err := itr.Next()
	for err == nil {
		if element.Key == "zones/us-central1-a" {
			types := element.Value.GetAcceleratorTypes()
			for _, item := range types {
				if item.GetName() == "nvidia-tesla-t4" {
					found = true
					break
				}
			}
		}
		element, err = itr.Next()
	}
	if err != iterator.Done {
		t.Fatal(err)
	}
	if !found {
		t.Error("Couldn't find the accelerator in the response")
	}
}

func TestTypeInt64(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}
	ctx := context.Background()
	c, err := NewImagesRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	space := uid.NewSpace("gogapic", nil)
	name := space.New()

	codes := []int64{
		5543610867827062957,
	}
	req := &computepb.InsertImageRequest{
		Project: projectId,
		ImageResource: &computepb.Image{
			Name:         &name,
			LicenseCodes: codes,
			SourceImage:  proto.String("projects/debian-cloud/global/images/debian-10-buster-v20210721"),
		},
	}
	globalClient, err := NewGlobalOperationsRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	insert, err := c.Insert(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	_, err = globalClient.Wait(ctx,
		&computepb.WaitGlobalOperationRequest{
			Project:   projectId,
			Operation: insert.Proto().GetName(),
		})
	if err != nil {
		t.Error(err)
	}
	defer func() {
		_, err := c.Delete(ctx,
			&computepb.DeleteImageRequest{
				Project: projectId,
				Image:   name,
			})
		if err != nil {
			t.Error(err)
		}
	}()

	fetched, err := c.Get(ctx,
		&computepb.GetImageRequest{
			Project: projectId,
			Image:   name,
		})
	if err != nil {
		t.Error(err)
	}
	if diff := cmp.Diff(fetched.GetLicenseCodes(), codes, cmp.Comparer(proto.Equal)); diff != "" {
		t.Fatalf("got(-),want(+):\n%s", diff)
	}
}

func TestCapitalLetter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}
	ctx := context.Background()
	c, err := NewFirewallsRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	space := uid.NewSpace("gogapic", nil)
	name := space.New()
	allowed := []*computepb.Allowed{
		{
			IPProtocol: proto.String("tcp"),
			Ports: []string{
				"80",
			},
		},
	}
	res := &computepb.Firewall{
		SourceRanges: []string{
			"0.0.0.0/0",
		},
		Name:    proto.String(name),
		Allowed: allowed,
	}
	req := &computepb.InsertFirewallRequest{
		Project:          projectId,
		FirewallResource: res,
	}
	insert, err := c.Insert(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	globalClient, err := NewGlobalOperationsRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = globalClient.Wait(ctx,
		&computepb.WaitGlobalOperationRequest{
			Project:   projectId,
			Operation: insert.Proto().GetName(),
		})
	if err != nil {
		t.Error(err)
	}
	defer func() {
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		err = internal.Retry(timeoutCtx, gax.Backoff{}, func() (stop bool, err error) {
			_, err = c.Delete(timeoutCtx,
				&computepb.DeleteFirewallRequest{
					Project:  projectId,
					Firewall: name,
				})
			return err == nil, err
		})
		if err != nil {
			t.Error(err)
		}
	}()
	fetched, err := c.Get(ctx, &computepb.GetFirewallRequest{
		Project:  projectId,
		Firewall: name,
	})
	if err != nil {
		t.Error(err)
	}
	if diff := cmp.Diff(fetched.GetAllowed(), allowed, cmp.Comparer(proto.Equal)); diff != "" {
		t.Fatalf("got(-),want(+):\n%s", diff)
	}
}
