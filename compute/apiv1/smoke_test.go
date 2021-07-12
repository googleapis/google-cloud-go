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
	"cloud.google.com/go/internal/testutil"
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
	"google.golang.org/protobuf/proto"
	"math/rand"
	"testing"
	"time"
)

var projectId = testutil.ProjID()
var defaultZone = "us-central1-a"

func TestCreateGetListInstance(t *testing.T){
	rand.Seed(time.Now().UTC().UnixNano())
	name := fmt.Sprintf("gotest%d", rand.Int())
	description := "тест"
	machineType := fmt.Sprintf(
		"https://www.googleapis.com/compute/v1/projects/%s/zones/%s/machineTypes/n1-standard-1",
		projectId, defaultZone)
	ctx := context.Background()
	c, err := NewInstancesRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	zonesClient, err := NewZoneOperationsRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	configName := "default"
	accessConfig := computepb.AccessConfig{
		Name: &configName,
	}
	configs := []*computepb.AccessConfig {
		&accessConfig,
	}
	networkInterface := computepb.NetworkInterface{
		AccessConfigs: configs,
	}
	interfaces := []*computepb.NetworkInterface{
		&networkInterface,
	}
	sourceImage := "projects/debian-cloud/global/images/family/debian-10"
	initializeParams := &computepb.AttachedDiskInitializeParams{
		SourceImage: &sourceImage,
	}
	diskType := computepb.AttachedDisk_PERSISTENT
	disk := computepb.AttachedDisk{
		AutoDelete: proto.Bool(true),
		Boot: proto.Bool(true),
		Type: &diskType,
		InitializeParams: initializeParams,
	}
	disks := []*computepb.AttachedDisk {
		&disk,
	}
	instance := &computepb.Instance{
		Name: &name,
		Description: &description,
		MachineType: &machineType,
		Disks: disks,
		NetworkInterfaces: interfaces,
	}

	createRequest := &computepb.InsertInstanceRequest{
		Project: projectId,
		Zone: defaultZone,
		InstanceResource: instance,
	}

	insert, err := c.Insert(ctx, createRequest)
	if err != nil {
		fmt.Println(err)
		t.Error(err)
	}

	waitZonalRequest := &computepb.WaitZoneOperationRequest{
		Project: projectId,
		Zone: defaultZone,
		Operation: insert.GetName(),
	}
	_, err = zonesClient.Wait(ctx, waitZonalRequest)
	if err != nil {
		return
	}
	fmt.Printf("Inserted instance named %s\n", name)
	defer ForceDeleteInstance(name, ctx, c)

	getRequest := &computepb.GetInstanceRequest{
		Project: projectId,
		Zone: defaultZone,
		Instance: name,
	}
	get, err := c.Get(ctx, getRequest)
	if err != nil {
		fmt.Println(err)
		t.Error(err)
	}
	assert.Equal(t, name, get.GetName())
	assert.Equal(t, "тест", get.GetDescription())

	listRequest := &computepb.ListInstancesRequest{
		Project: projectId,
		Zone: defaultZone,
	}

	list, err := c.List(ctx, listRequest)
	if err != nil {
		fmt.Println(err)
		t.Error(err)
	}
	items := list.GetItems()
	found := false
	for _, element := range items {
		if element.GetName() == name {
			found = true
		}
	}
	if found == false{
		t.Error("Couldn't find the instance in list response")
	}

	deleteInstanceRequest := &computepb.DeleteInstanceRequest{
		Project: projectId,
		Zone: defaultZone,
		Instance: name,
	}
	_, err = c.Delete(ctx, deleteInstanceRequest)
	if err != nil {
		fmt.Println(err)
		t.Error(err)
	}
}

func ForceDeleteInstance(name string, ctx context.Context, client *InstancesClient){
	deleteInstanceRequest := &computepb.DeleteInstanceRequest{
		Project: projectId,
		Zone: defaultZone,
		Instance: name,
	}
	_, err := client.Delete(ctx, deleteInstanceRequest)
	if err != nil {}

}

func TestCreateGetRemoveSecurityPolicies(t *testing.T){
	rand.Seed(time.Now().UTC().UnixNano())
	name := fmt.Sprintf("gotest%d", rand.Int())
	ctx := context.Background()
	c, err := NewSecurityPoliciesRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	globalCLient, err := NewGlobalOperationsRESTClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defaultDescription := "default rule"
	description := "test rule"
	defaultPriority  := int32(2147483647)
	priority := int32(0)
	action := "allow"

	srcIpRanges := []string{
		"*",
	}
	config := &computepb.SecurityPolicyRuleMatcherConfig{
		SrcIpRanges: srcIpRanges,
	}
	versionExpr := computepb.SecurityPolicyRuleMatcher_SRC_IPS_V1
	matcher := &computepb.SecurityPolicyRuleMatcher{
		Config: config,
		VersionedExpr: &versionExpr,
	}
	securityPolicyRule := &computepb.SecurityPolicyRule{
		Action: &action,
		Priority: &priority,
		Description: &description,
		Match: matcher,
	}
	securityPolicyRuleDefault := &computepb.SecurityPolicyRule{
		Action: &action,
		Priority: &defaultPriority,
		Description: &defaultDescription,
		Match: matcher,
	}

	rules := []*computepb.SecurityPolicyRule{
		securityPolicyRule,
		securityPolicyRuleDefault,
	}

	securityPolicy := &computepb.SecurityPolicy{
		Name: &name,
		Rules: rules,
	}

	insertRequest := &computepb.InsertSecurityPolicyRequest{
		Project: projectId,
		SecurityPolicyResource: securityPolicy,
	}
	insert, err := c.Insert(ctx, insertRequest)
	if err != nil {
		fmt.Println(err)
		t.Error(err)
	}

	waitGlobalRequest := &computepb.WaitGlobalOperationRequest{
		Project: projectId,
		Operation: insert.GetName(),
	}
	_, err = globalCLient.Wait(ctx, waitGlobalRequest)
	if err != nil {
		fmt.Println(err)
		t.Error(err)
	}
	fmt.Printf("Inserted security policy named %s\n", name)
	defer ForceDeleteSecurityPolicy(name, ctx, c)

	removeRuleRequest := &computepb.RemoveRuleSecurityPolicyRequest{
		Priority: &priority,
		Project: projectId,
		SecurityPolicy: name,
	}

	rule, err := c.RemoveRule(ctx, removeRuleRequest)
	if err != nil {
		fmt.Println(err)
		t.Error(err)
	}
	waitGlobalRequestRemove := &computepb.WaitGlobalOperationRequest{
		Project: projectId,
		Operation: rule.GetName(),
	}
	_, err = globalCLient.Wait(ctx, waitGlobalRequestRemove)
	if err != nil {
		fmt.Println(err)
		t.Error(err)
	}

	getRequest := &computepb.GetSecurityPolicyRequest{
		Project: projectId,
		SecurityPolicy: name,
	}
	get, err := c.Get(ctx, getRequest)
	if err != nil {
		fmt.Println(err)
		t.Error(err)
	}
	assert.Equal(t, 1, len(get.GetRules()))

	deleteRequest := &computepb.DeleteSecurityPolicyRequest{
		Project: projectId,
		SecurityPolicy: name,
	}
	_, err = c.Delete(ctx, deleteRequest)
	if err != nil {
		fmt.Println(err)
		t.Error(err)
	}
}

func ForceDeleteSecurityPolicy(name string, ctx context.Context, client *SecurityPoliciesClient){
	deleteRequest := &computepb.DeleteSecurityPolicyRequest{
		Project: projectId,
		SecurityPolicy: name,
	}
	_, err := client.Delete(ctx, deleteRequest)
	if err != nil {}
}
