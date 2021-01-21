// Copyright 2020 Google LLC
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

// +build go1.15

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/datastore"
)

// timeSaver gets and puts a time.Time value.
type timeSaver interface {
	get(context.Context) (time.Time, error)
	put(context.Context, time.Time) error
}

type dsTimeSaver struct {
	projectID string
	client    *datastore.Client
	k         *datastore.Key
}

var _ timeSaver = &dsTimeSaver{}

// indexTimestamp is the time we've processed until, stored in Datastore.
type indexTimestamp struct {
	T time.Time
}

func (ts *dsTimeSaver) get(ctx context.Context) (time.Time, error) {
	if ts.client == nil {
		var err error
		ts.client, err = datastore.NewClient(ctx, ts.projectID)
		if err != nil {
			return time.Time{}, err
		}
		ts.k = datastore.NameKey("godocfx-index", "latest", nil)
	}
	prevLatest := indexTimestamp{}
	if err := ts.client.Get(ctx, ts.k, &prevLatest); err != nil {
		if err != datastore.ErrNoSuchEntity {
			return time.Time{}, fmt.Errorf("Get: %v", err)
		}
		// Default to 10 days ago.
		prevLatest.T = time.Now().Add(-10 * 24 * time.Hour).UTC()
		log.Println("Default to", prevLatest.T)
	}
	return prevLatest.T.UTC(), nil
}

func (ts *dsTimeSaver) put(ctx context.Context, t time.Time) error {
	if _, err := ts.client.Put(ctx, ts.k, &indexTimestamp{T: t}); err != nil {
		return fmt.Errorf("Put: %v", err)
	}
	return nil
}
