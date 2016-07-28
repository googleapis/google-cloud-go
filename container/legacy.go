// Copyright 2016 Google Inc. All Rights Reserved.
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

package container

import (
	"errors"
	"net/http"

	"golang.org/x/net/context"
	raw "google.golang.org/api/container/v1"
	"google.golang.org/cloud/internal"
)

// Cluster returns metadata about the specified cluster.
//
// Deprecated: please use Client.Cluster instead.
func Cluster(ctx context.Context, zone, name string) (*Resource, error) {
	s := rawService(ctx)
	resp, err := s.Projects.Zones.Clusters.Get(internal.ProjID(ctx), zone, name).Do()
	if err != nil {
		return nil, err
	}
	return resourceFromRaw(resp), nil
}

// CreateCluster creates a new cluster with the provided metadata
// in the specified zone.
//
// Deprecated: please use Client.CreateCluster instead.
func CreateCluster(ctx context.Context, zone string, resource *Resource) (*Resource, error) {
	panic("not implemented")
}

// DeleteCluster deletes a cluster.
//
// Deprecated: please use Client.DeleteCluster instead.
func DeleteCluster(ctx context.Context, zone, name string) error {
	s := rawService(ctx)
	_, err := s.Projects.Zones.Clusters.Delete(internal.ProjID(ctx), zone, name).Do()
	return err
}

// Operations returns a list of operations from the specified zone.
// If no zone is specified, it looks up for all of the operations
// that are running under the user's project.
//
// Deprecated: please use Client.Operations instead.
func Operations(ctx context.Context, zone string) ([]*Op, error) {
	s := rawService(ctx)
	if zone == "" {
		resp, err := s.Projects.Zones.Operations.List(internal.ProjID(ctx), "-").Do()
		if err != nil {
			return nil, err
		}
		return opsFromRaw(resp.Operations), nil
	}
	resp, err := s.Projects.Zones.Operations.List(internal.ProjID(ctx), zone).Do()
	if err != nil {
		return nil, err
	}
	return opsFromRaw(resp.Operations), nil
}

// Operation returns an operation.
//
// Deprecated: please use Client.Operation instead.
func Operation(ctx context.Context, zone, name string) (*Op, error) {
	s := rawService(ctx)
	resp, err := s.Projects.Zones.Operations.Get(internal.ProjID(ctx), zone, name).Do()
	if err != nil {
		return nil, err
	}
	if resp.StatusMessage != "" {
		return nil, errors.New(resp.StatusMessage)
	}
	return opFromRaw(resp), nil
}

func rawService(ctx context.Context) *raw.Service {
	return internal.Service(ctx, "container", func(hc *http.Client) interface{} {
		svc, _ := raw.New(hc)
		return svc
	}).(*raw.Service)
}
