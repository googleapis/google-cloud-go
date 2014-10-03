// Copyright 2014 Google Inc. All Rights Reserved.
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

	raw "code.google.com/p/google-api-go-client/storage/v1"
)

// ACLRole is the the access permission for the entity.
type ACLRole string

const (
	RoleOwner  ACLRole = "OWNER"
	RoleReader ACLRole = "READER"
)

// ACLRule represents an access control list rule entry for a Google Cloud Storage object or bucket.
// A bucket is a Google Cloud Storage container whose name is globally unique and contains zero or
// more objects.  An object is a blob of data that is stored in a bucket.
type ACLRule struct {
	// Entity identifies the entity holding the current
	// rule's permissions. It could be in the form of:
	// - "user-<userId>"
	// - "user-<email>"
	// - "group-<groupId>"
	// - "group-<email>"
	// - "domain-<domain>"
	// - "project-team-<projectId>"
	// - "allUsers"
	// - "allAuthenticatedUsers"
	Entity string `json:"entity,omitempty"`

	// Role is the the access permission for the entity.
	Role ACLRole `json:"role,omitempty"`
}

// DefaultACL returns the default object ACL entries for the named bucket.
func (c *Client) DefaultACL(bucket string) ([]ACLRule, error) {
	acls, err := c.conn.s.DefaultObjectAccessControls.List(bucket).Do()
	if err != nil {
		return nil, fmt.Errorf("storage: error listing default object ACL for bucket %q: %v", bucket, err)
	}
	r := make([]ACLRule, 0, len(acls.Items))
	for _, v := range acls.Items {
		if m, ok := v.(map[string]interface{}); ok {
			entity, ok1 := m["entity"].(string)
			role, ok2 := m["role"].(string)
			if ok1 && ok2 {
				r = append(r, ACLRule{Entity: entity, Role: ACLRole(role)})
			}
		}
	}
	return r, nil
}

// PutDefaultACLRule saves the named default object ACL entity with the provided role for the named bucket.
func (c *Client) PutDefaultACLRule(bucket, entity string, role ACLRole) error {
	acl := &raw.ObjectAccessControl{
		Bucket: bucket,
		Entity: entity,
		Role:   string(role),
	}
	_, err := c.conn.s.DefaultObjectAccessControls.Update(bucket, entity, acl).Do()
	if err != nil {
		return fmt.Errorf("storage: error updating default ACL rule for bucket %q, entity %q: %v", bucket, entity, err)
	}
	return nil
}

// DeleteDefaultACLRule deletes the named default ACL entity for the named bucket.
func (c *Client) DeleteDefaultACLRule(bucket, entity string) error {
	err := c.conn.s.DefaultObjectAccessControls.Delete(bucket, entity).Do()
	if err != nil {
		return fmt.Errorf("storage: error deleting default ACL rule for bucket %q, entity %q: %v", bucket, entity, err)
	}
	return nil
}

// BucketACL returns the ACL entries for the named bucket.
func (c *Client) BucketACL(bucket string) ([]ACLRule, error) {
	acls, err := c.conn.s.BucketAccessControls.List(bucket).Do()
	if err != nil {
		return nil, fmt.Errorf("storage: error listing bucket ACL for bucket %q: %v", bucket, err)
	}
	r := make([]ACLRule, len(acls.Items))
	for i, v := range acls.Items {
		r[i].Entity = v.Entity
		r[i].Role = ACLRole(v.Role)
	}
	return r, nil
}

// PutBucketACLRule saves the named ACL entity with the provided role for the named bucket.
func (c *Client) PutBucketACLRule(bucket, entity string, role ACLRole) error {
	acl := &raw.BucketAccessControl{
		Bucket: bucket,
		Entity: entity,
		Role:   string(role),
	}
	_, err := c.conn.s.BucketAccessControls.Update(bucket, entity, acl).Do()
	if err != nil {
		return fmt.Errorf("storage: error updating bucket ACL rule for bucket %q, entity %q: %v", bucket, entity, err)
	}
	return nil
}

// DeleteBucketACLRule deletes the named ACL entity for the named bucket.
func (c *Client) DeleteBucketACLRule(bucket, entity string) error {
	err := c.conn.s.BucketAccessControls.Delete(bucket, entity).Do()
	if err != nil {
		return fmt.Errorf("storage: error deleting bucket ACL rule for bucket %q, entity %q: %v", bucket, entity, err)
	}
	return nil
}

// ACL returns the ACL entries for the named object.
func (b *BucketClient) ACL(object string) ([]ACLRule, error) {
	acls, err := b.conn.s.ObjectAccessControls.List(b.name, object).Do()
	if err != nil {
		return nil, fmt.Errorf("storage: error listing object ACL for bucket %q, file %q: %v", b.name, object, err)
	}
	r := make([]ACLRule, 0, len(acls.Items))
	for _, v := range acls.Items {
		if m, ok := v.(map[string]interface{}); ok {
			entity, ok1 := m["entity"].(string)
			role, ok2 := m["role"].(string)
			if ok1 && ok2 {
				r = append(r, ACLRule{Entity: entity, Role: ACLRole(role)})
			}
		}
	}
	return r, nil
}

// PutACLRule saves the named ACL entity with the provided role for the named object.
func (b *BucketClient) PutACLRule(object, entity string, role ACLRole) error {
	acl := &raw.ObjectAccessControl{
		Bucket: b.name,
		Entity: entity,
		Role:   string(role),
	}
	_, err := b.conn.s.ObjectAccessControls.Update(b.name, object, entity, acl).Do()
	if err != nil {
		return fmt.Errorf("storage: error updating object ACL rule for bucket %q, file %q, entity %q: %v", b.name, object, entity, err)
	}
	return nil
}

// DeleteACLRule deletes the named ACL entity for the named object.
func (b *BucketClient) DeleteACLRule(object, entity string) error {
	err := b.conn.s.ObjectAccessControls.Delete(b.name, object, entity).Do()
	if err != nil {
		return fmt.Errorf("storage: error deleting object ACL rule for bucket %q, file %q, entity %q: %v", b.name, object, entity, err)
	}
	return nil
}
