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

	"golang.org/x/net/context"
	raw "google.golang.org/api/storage/v1"
)

// ACLPermission is the level of access to grant.
type ACLPermission string

const (
	RoleOwner  ACLPermission = "OWNER"
	RoleReader ACLPermission = "READER"
)

// ACLScope is used to determine the permission level for a user, group or team.
// Scopes are sometimes referred to as grantees.
//
// It could be in the form of:
// "user-<userId>", "user-<email>", "group-<groupId>", "group-<email>",
// "domain-<domain>" and "project-team-<projectId>".
//
// Or one of the predefined constants: AllUsers, AllAuthenticatedUsers.
type ACLScope string

const (
	AllUsers              ACLScope = "allUsers"
	AllAuthenticatedUsers ACLScope = "allAuthenticatedUsers"
)

// ACLEntry represents a grant for a permission to a scope (user, group or team) for a Google Cloud Storage object or bucket.
type ACLEntry struct {
	Scope      ACLScope
	Permission ACLPermission
}

// ACL provides operations on an access control list for a Google Cloud Storage bucket or object.
type ACL struct {
	c         *Client
	bucket    string
	object    string
	isDefault bool
}

// Delete permanently deletes the ACL entry for the given scope.
func (a *ACL) Delete(ctx context.Context, scope ACLScope) error {
	if a.object != "" {
		return a.objectDelete(ctx, scope)
	}
	if a.isDefault {
		return a.bucketDefaultDelete(ctx, scope)
	}
	return a.bucketDelete(ctx, scope)
}

// Set sets the permission level for the given scope.
func (a *ACL) Set(ctx context.Context, scope ACLScope, perm ACLPermission) error {
	if a.object != "" {
		return a.objectSet(ctx, scope, perm)
	}
	if a.isDefault {
		return a.bucketDefaultSet(ctx, scope, perm)
	}
	return a.bucketSet(ctx, scope, perm)
}

// List retrieves ACL entries.
func (a *ACL) List(ctx context.Context) ([]ACLEntry, error) {
	if a.object != "" {
		return a.objectList(ctx)
	}
	if a.isDefault {
		return a.bucketDefaultList(ctx)
	}
	return a.bucketList(ctx)
}

func (a *ACL) bucketDefaultList(ctx context.Context) ([]ACLEntry, error) {
	acls, err := a.c.raw.DefaultObjectAccessControls.List(a.bucket).Do()
	if err != nil {
		return nil, fmt.Errorf("storage: error listing default object ACL for bucket %q: %v", a.bucket, err)
	}
	r := make([]ACLEntry, 0, len(acls.Items))
	for _, v := range acls.Items {
		if m, ok := v.(map[string]interface{}); ok {
			entity, ok1 := m["entity"].(string)
			role, ok2 := m["role"].(string)
			if ok1 && ok2 {
				r = append(r, ACLEntry{Scope: ACLScope(entity), Permission: ACLPermission(role)})
			}
		}
	}
	return r, nil
}

func (a *ACL) bucketDefaultSet(ctx context.Context, scope ACLScope, role ACLPermission) error {
	acl := &raw.ObjectAccessControl{
		Bucket: a.bucket,
		Entity: string(scope),
		Role:   string(role),
	}
	_, err := a.c.raw.DefaultObjectAccessControls.Update(a.bucket, string(scope), acl).Do()
	if err != nil {
		return fmt.Errorf("storage: error updating default ACL entry for bucket %q, scope %q: %v", a.bucket, scope, err)
	}
	return nil
}

func (a *ACL) bucketDefaultDelete(ctx context.Context, scope ACLScope) error {
	err := a.c.raw.DefaultObjectAccessControls.Delete(a.bucket, string(scope)).Do()
	if err != nil {
		return fmt.Errorf("storage: error deleting default ACL entry for bucket %q, scope %q: %v", a.bucket, scope, err)
	}
	return nil
}

func (a *ACL) bucketList(ctx context.Context) ([]ACLEntry, error) {
	acls, err := a.c.raw.BucketAccessControls.List(a.bucket).Do()
	if err != nil {
		return nil, fmt.Errorf("storage: error listing bucket ACL for bucket %q: %v", a.bucket, err)
	}
	r := make([]ACLEntry, len(acls.Items))
	for i, v := range acls.Items {
		r[i].Scope = ACLScope(v.Entity)
		r[i].Permission = ACLPermission(v.Role)
	}
	return r, nil
}

func (a *ACL) bucketSet(ctx context.Context, scope ACLScope, role ACLPermission) error {
	acl := &raw.BucketAccessControl{
		Bucket: a.bucket,
		Entity: string(scope),
		Role:   string(role),
	}
	_, err := a.c.raw.BucketAccessControls.Update(a.bucket, string(scope), acl).Do()
	if err != nil {
		return fmt.Errorf("storage: error updating bucket ACL entry for bucket %q, scope %q: %v", a.bucket, scope, err)
	}
	return nil
}

func (a *ACL) bucketDelete(ctx context.Context, scope ACLScope) error {
	err := a.c.raw.BucketAccessControls.Delete(a.bucket, string(scope)).Do()
	if err != nil {
		return fmt.Errorf("storage: error deleting bucket ACL entry for bucket %q, scope %q: %v", a.bucket, scope, err)
	}
	return nil
}

func (a *ACL) objectList(ctx context.Context) ([]ACLEntry, error) {
	acls, err := a.c.raw.ObjectAccessControls.List(a.bucket, a.object).Do()
	if err != nil {
		return nil, fmt.Errorf("storage: error listing object ACL for bucket %q, file %q: %v", a.bucket, a.object, err)
	}
	r := make([]ACLEntry, 0, len(acls.Items))
	for _, v := range acls.Items {
		if m, ok := v.(map[string]interface{}); ok {
			entity, ok1 := m["entity"].(string)
			role, ok2 := m["role"].(string)
			if ok1 && ok2 {
				r = append(r, ACLEntry{Scope: ACLScope(entity), Permission: ACLPermission(role)})
			}
		}
	}
	return r, nil
}

func (a *ACL) objectSet(ctx context.Context, scope ACLScope, role ACLPermission) error {
	acl := &raw.ObjectAccessControl{
		Bucket: a.bucket,
		Entity: string(scope),
		Role:   string(role),
	}
	_, err := a.c.raw.ObjectAccessControls.Update(a.bucket, a.object, string(scope), acl).Do()
	if err != nil {
		return fmt.Errorf("storage: error updating object ACL entry for bucket %q, file %q, scope %q: %v", a.bucket, a.object, scope, err)
	}
	return nil
}

func (a *ACL) objectDelete(ctx context.Context, scope ACLScope) error {
	err := a.c.raw.ObjectAccessControls.Delete(a.bucket, a.object, string(scope)).Do()
	if err != nil {
		return fmt.Errorf("storage: error deleting object ACL entry for bucket %q, file %q, scope %q: %v", a.bucket, a.object, scope, err)
	}
	return nil
}
