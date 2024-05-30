// Copyright 2023 Google LLC
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

package genai

import (
	"context"
	"errors"
	"fmt"
	"time"

	aiplatform "cloud.google.com/go/aiplatform/apiv1beta1"
	pb "cloud.google.com/go/aiplatform/apiv1beta1/aiplatformpb"
	"cloud.google.com/go/vertexai/internal/support"
	"google.golang.org/api/iterator"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	fieldmaskpb "google.golang.org/protobuf/types/known/fieldmaskpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

type cacheClient = aiplatform.GenAiCacheClient

var (
	newCacheClient     = aiplatform.NewGenAiCacheClient
	newCacheRESTClient = aiplatform.NewGenAiCacheRESTClient
)

// GenerativeModelFromCachedContent returns a [GenerativeModel] that uses the given [CachedContent].
// The argument should come from a call to [Client.CreateCachedContent] or [Client.GetCachedContent].
func (c *Client) GenerativeModelFromCachedContent(cc *CachedContent) *GenerativeModel {
	return &GenerativeModel{
		c:                 c,
		name:              cc.Model,
		fullName:          inferFullModelName(c.projectID, c.location, cc.Model),
		CachedContentName: cc.Name,
	}
}

// CreateCachedContent creates a new CachedContent.
// You can use the return value to create a model with [CachedContent.GenerativeModel].
func (c *Client) CreateCachedContent(ctx context.Context, cc *CachedContent) (*CachedContent, error) {
	return c.cachedContentFromProto(c.cc.CreateCachedContent(ctx, &pb.CreateCachedContentRequest{
		Parent:        c.parent(),
		CachedContent: cc.toProto(),
	}))
}

// GetCachedContent retrieves the CachedContent with the given name.
func (c *Client) GetCachedContent(ctx context.Context, name string) (*CachedContent, error) {
	return c.cachedContentFromProto(c.cc.GetCachedContent(ctx, &pb.GetCachedContentRequest{Name: name}))
}

// DeleteCachedContent deletes the CachedContent with the given name.
func (c *Client) DeleteCachedContent(ctx context.Context, name string) error {
	return c.cc.DeleteCachedContent(ctx, &pb.DeleteCachedContentRequest{Name: name})
}

// CachedContentToUpdate specifies which fields of a CachedContent to modify in a call to
// [Client.UpdateCachedContent].
type CachedContentToUpdate struct {
	// If non-nil, update the expire time or TTL.
	Expiration *ExpireTimeOrTTL
}

// UpdateCachedContent modifies the [CachedContent] with the given name according to the values
// of the [CachedContentToUpdate] struct.
// It returns the modified CachedContent.
func (c *Client) UpdateCachedContent(ctx context.Context, name string, ccu *CachedContentToUpdate) (*CachedContent, error) {
	if ccu == nil || ccu.Expiration == nil {
		return nil, errors.New("cloud.google.com/go/vertexai/genai: no update specified")
	}
	cc := &CachedContent{
		Name:       name,
		Expiration: *ccu.Expiration,
	}
	return c.cachedContentFromProto(c.cc.UpdateCachedContent(ctx, &pb.UpdateCachedContentRequest{
		CachedContent: cc.toProto(),
		UpdateMask:    &fieldmaskpb.FieldMask{Paths: []string{"expiration"}},
	}))
}

// ListCachedContents lists all the CachedContents associated with the project and location.
func (c *Client) ListCachedContents(ctx context.Context) *CachedContentIterator {
	return &CachedContentIterator{
		it: c.cc.ListCachedContents(ctx, &pb.ListCachedContentsRequest{Parent: c.parent()}),
	}
}

// A CachedContentIterator iterates over CachedContents.
type CachedContentIterator struct {
	it *aiplatform.CachedContentIterator
}

// Next returns the next result. Its second return value is iterator.Done if there are no more
// results. Once Next returns Done, all subsequent calls will return Done.
func (it *CachedContentIterator) Next() (*CachedContent, error) {
	m, err := it.it.Next()
	if err != nil {
		return nil, err
	}
	return (CachedContent{}).fromProto(m), nil
}

// PageInfo supports pagination. See the google.golang.org/api/iterator package for details.
func (it *CachedContentIterator) PageInfo() *iterator.PageInfo {
	return it.it.PageInfo()
}

func (c *Client) cachedContentFromProto(pcc *pb.CachedContent, err error) (*CachedContent, error) {
	if err != nil {
		return nil, err
	}
	cc := (CachedContent{}).fromProto(pcc)
	cc.client = c
	return cc, nil
}

// ExpireTimeOrTTL describes the time when a resource expires.
// If ExpireTime is non-zero, it is the expiration time.
// Otherwise, the expiration time is the value of TTL ("time to live") added
// to the current time.
type ExpireTimeOrTTL struct {
	ExpireTime time.Time
	TTL        time.Duration
}

// populateCachedContentTo populates some fields of p from v.
func populateCachedContentTo(p *pb.CachedContent, v *CachedContent) {
	exp := v.Expiration
	if !exp.ExpireTime.IsZero() {
		p.Expiration = &pb.CachedContent_ExpireTime{
			ExpireTime: timestamppb.New(exp.ExpireTime),
		}
	} else if exp.TTL != 0 {
		p.Expiration = &pb.CachedContent_Ttl{
			Ttl: durationpb.New(exp.TTL),
		}
	}
	// If both fields of v.Expiration are zero, leave p.Expiration unset.
}

// populateCachedContentFrom populates some fields of v from p.
func populateCachedContentFrom(v *CachedContent, p *pb.CachedContent) {
	if p.Expiration == nil {
		return
	}
	switch e := p.Expiration.(type) {
	case *pb.CachedContent_ExpireTime:
		v.Expiration.ExpireTime = support.TimeFromProto(e.ExpireTime)
	case *pb.CachedContent_Ttl:
		v.Expiration.TTL = e.Ttl.AsDuration()
	default:
		panic(fmt.Sprintf("unknown type of CachedContent.Expiration: %T", p.Expiration))
	}
}
