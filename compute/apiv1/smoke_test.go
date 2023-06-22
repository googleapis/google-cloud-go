// Copyright 2023 Google LLC
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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/option"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"google.golang.org/api/iterator"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
	"google.golang.org/protobuf/proto"
)

var projectId = testutil.ProjID()
var defaultZone = "us-central1-a"
var image = "projects/debian-cloud/global/images/family/debian-10"

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
					Type:       proto.String(computepb.AttachedDisk_PERSISTENT.String()),
					InitializeParams: &computepb.AttachedDiskInitializeParams{
						SourceImage: proto.String(image),
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

	err = insert.Wait(ctx)
	defer ForceDeleteInstance(ctx, name, c)
	if err != nil {
		t.Fatal(err)
	}

	getRequest := &computepb.GetInstanceRequest{
		Project:  projectId,
		Zone:     defaultZone,
		Instance: name,
	}
	get, err := c.Get(ctx, getRequest)
	if err != nil {
		t.Fatal(err)
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

	err = updateOp.Wait(ctx)
	if err != nil {
		t.Fatal(err)
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

	err = patchOp.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}

	fetched, err := c.Get(ctx, getRequest)
	if err != nil {
		t.Fatal(err)
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
		t.Fatal(err)
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
	action := "allow"
	matcher := &computepb.SecurityPolicyRuleMatcher{
		Config: &computepb.SecurityPolicyRuleMatcherConfig{
			SrcIpRanges: []string{
				"*",
			},
		},
		VersionedExpr: proto.String(computepb.SecurityPolicyRuleMatcher_SRC_IPS_V1.String()),
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

	err = insert.Wait(ctx)
	defer ForceDeleteSecurityPolicy(ctx, name, c)
	if err != nil {
		t.Fatal(err)
	}

	removeRuleRequest := &computepb.RemoveRuleSecurityPolicyRequest{
		Priority:       proto.Int32(0),
		Project:        projectId,
		SecurityPolicy: name,
	}

	rule, err := c.RemoveRule(ctx, removeRuleRequest)
	if err != nil {
		t.Error(err)
	}
	err = rule.Wait(ctx)
	if err != nil {
		t.Fatal(err)
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
		t.Fatalf("expected count for rules: %d, got: %d", 1, len(get.GetRules()))
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

func TestHeaders(t *testing.T) {
	ctx := context.Background()

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		xGoog := r.Header.Get("X-Goog-Api-Client")
		if contentType != "application/json" {
			t.Fatalf("Content-Type header was %s, expected `application/json`.", contentType)
		}
		if !strings.Contains(xGoog, "rest/") {
			t.Fatal("X-Goog-Api-Client header doesn't contain `rest/`")
		}
	}))
	defer svr.Close()
	opts := []option.ClientOption{
		option.WithEndpoint(svr.URL),
		option.WithoutAuthentication(),
	}
	c, err := NewAcceleratorTypesRESTClient(ctx, opts...)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(ctx, &computepb.GetAcceleratorTypeRequest{
		AcceleratorType: "test",
		Project:         "test",
		Zone:            "test",
	})
	if err != nil {
		return
	}
}

func TestInstanceGroupResize(t *testing.T) {
	// we test a required query-param field set to 0
	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}
	ctx := context.Background()
	instanceTemplatesClient, err := NewInstanceTemplatesRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	instanceGroupManagersClient, err := NewInstanceGroupManagersRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	space := uid.NewSpace("gogapic", nil)
	templateName := space.New()
	managerName := space.New()
	templateResource := &computepb.InstanceTemplate{
		Name: proto.String(templateName),
		Properties: &computepb.InstanceProperties{
			MachineType: proto.String("n2-standard-2"),
			Disks: []*computepb.AttachedDisk{
				{
					AutoDelete: proto.Bool(true),
					Boot:       proto.Bool(true),
					Type:       proto.String(computepb.AttachedDisk_PERSISTENT.String()),
					InitializeParams: &computepb.AttachedDiskInitializeParams{
						SourceImage: proto.String(image),
					},
				},
			},
			NetworkInterfaces: []*computepb.NetworkInterface{
				{
					AccessConfigs: []*computepb.AccessConfig{
						{
							Name: proto.String("default"),
							Type: proto.String(computepb.AccessConfig_ONE_TO_ONE_NAT.String()),
						},
					},
				},
			},
		},
	}
	insertOp, err := instanceTemplatesClient.Insert(
		ctx,
		&computepb.InsertInstanceTemplateRequest{
			Project:                  projectId,
			InstanceTemplateResource: templateResource,
		})
	if err != nil {
		t.Fatal(err)
	}
	err = insertOp.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, err = instanceTemplatesClient.Delete(
			ctx,
			&computepb.DeleteInstanceTemplateRequest{
				Project:          projectId,
				InstanceTemplate: templateName,
			})
		if err != nil {
			t.Error(err)
		}
	}()

	igmResource := &computepb.InstanceGroupManager{
		BaseInstanceName: proto.String("gogapic"),
		TargetSize:       proto.Int32(1),
		InstanceTemplate: proto.String(insertOp.Proto().GetTargetLink()),
		Name:             proto.String(managerName),
	}

	insertOp, err = instanceGroupManagersClient.Insert(
		ctx,
		&computepb.InsertInstanceGroupManagerRequest{
			Project:                      projectId,
			Zone:                         defaultZone,
			InstanceGroupManagerResource: igmResource,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = insertOp.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		deleteOp, err := instanceGroupManagersClient.Delete(
			timeoutCtx,
			&computepb.DeleteInstanceGroupManagerRequest{
				Zone:                 defaultZone,
				Project:              projectId,
				InstanceGroupManager: managerName,
			})
		if err != nil {
			t.Error(err)
		}
		if err := deleteOp.Wait(ctx); err != nil {
			t.Error(err)
		}
	}()
	fetched, err := instanceGroupManagersClient.Get(
		ctx,
		&computepb.GetInstanceGroupManagerRequest{
			Project:              projectId,
			Zone:                 defaultZone,
			InstanceGroupManager: managerName,
		})
	if err != nil {
		t.Fatal(err)
	}
	if fetched.GetTargetSize() != 1 {
		t.Fatalf("expected target size: %d, got: %d", 1, fetched.GetTargetSize())
	}
	resizeOperation, err := instanceGroupManagersClient.Resize(
		ctx,
		&computepb.ResizeInstanceGroupManagerRequest{
			Project:              projectId,
			Size:                 0,
			Zone:                 defaultZone,
			InstanceGroupManager: managerName,
		})
	if err != nil {
		t.Fatal(err)
	}
	err = resizeOperation.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fetched, err = instanceGroupManagersClient.Get(
		ctx,
		&computepb.GetInstanceGroupManagerRequest{
			Project:              projectId,
			Zone:                 defaultZone,
			InstanceGroupManager: managerName,
		})
	if err != nil {
		t.Fatal(err)
	}
	if fetched.GetTargetSize() != 0 {
		t.Fatalf("expected target size: %d, got: %d", 0, fetched.GetTargetSize())
	}
}
