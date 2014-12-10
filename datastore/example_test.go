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

package datastore_test

import (
	"log"
	"net/http"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/cloud"
	"google.golang.org/cloud/datastore"
)

// TODO(jbd): Remove after Go 1.4.
// Related to https://codereview.appspot.com/107320046
func TestA(t *testing.T) {}

func Example_auth() {
	// Initialize an authorized transport with Google Developers Console
	// JSON key. Read the google package examples to learn more about
	// different authorization flows you can use.
	// http://godoc.org/golang.org/x/oauth2/google
	opts, err := oauth2.New(
		google.ServiceAccountJSONKey("/path/to/json/keyfile.json"),
		oauth2.Scope(datastore.ScopeDatastore),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := cloud.NewContext("project-id", &http.Client{Transport: opts.NewTransport()})
	_ = ctx // Use the context (see other examples)
}

func Example_getPutDelete() {
	// see the auth example how to initiate a context.
	ctx := cloud.NewContext("project-id", &http.Client{Transport: nil})

	type Article struct {
		Title       string
		Description string
		Body        string `datastore:",noindex"`
		Author      *datastore.Key
		PublishedAt time.Time
	}

	// Create a new article entity.
	newKey := datastore.NewIncompleteKey(ctx, "Article", nil)
	key, err := datastore.Put(ctx, newKey, &Article{
		Title:       "The title of the article",
		Description: "The description of the article...",
		Body:        "...",
		Author:      datastore.NewKey(ctx, "Author", "jbd", 0, nil),
		PublishedAt: time.Now(),
	})
	if err != nil {
		log.Println(err)
		return
	}
	// Retrieve the newly created entiy.
	article := &Article{}
	if err := datastore.Get(ctx, key, article); err != nil {
		log.Println(err)
	}
	// Delete the entity.
	if err := datastore.Delete(ctx, key); err != nil {
		log.Println(err)
	}
}

type Post struct {
	Title       string
	PublishedAt time.Time
	Comments    int
}

func Example_getMulti() {
	// see the auth example how to initiate a context.
	ctx := cloud.NewContext("project-id", &http.Client{Transport: nil})

	keys := []*datastore.Key{
		datastore.NewKey(ctx, "Post", "post1", 0, nil),
		datastore.NewKey(ctx, "Post", "post2", 0, nil),
		datastore.NewKey(ctx, "Post", "post3", 0, nil),
	}
	posts := make([]Post, 3)
	if err := datastore.GetMulti(ctx, keys, posts); err != nil {
		log.Println(err)
	}
}

func Example_putMulti() {
	// see the auth example how to initiate a context.
	ctx := cloud.NewContext("project-id", &http.Client{Transport: nil})

	keys := []*datastore.Key{
		datastore.NewKey(ctx, "Post", "post1", 0, nil),
		datastore.NewKey(ctx, "Post", "post2", 0, nil),
	}

	// PutMulti with a Post slice.
	posts := []*Post{
		{Title: "Post 1", PublishedAt: time.Now()},
		{Title: "Post 2", PublishedAt: time.Now()},
	}
	if _, err := datastore.PutMulti(ctx, keys, posts); err != nil {
		log.Println(err)
	}

	// PutMulti with an empty interface slice.
	morePosts := []interface{}{
		&Post{Title: "Post 1", PublishedAt: time.Now()},
		&Post{Title: "Post 2", PublishedAt: time.Now()},
	}
	if _, err := datastore.PutMulti(ctx, keys, morePosts); err != nil {
		log.Println(err)
	}
}

func Example_basicQueries() {
	// see the auth example how to initiate a context.
	ctx := cloud.NewContext("project-id", &http.Client{Transport: nil})

	// Count the number of the post entities.
	n, err := datastore.NewQuery("Post").Count(ctx)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("There are %d posts.", n)

	// List the posts published since yesterday.
	yesterday := time.Now().Add(-24 * time.Hour)
	it := datastore.NewQuery("Post").Filter("PublishedAt >", yesterday).Run(ctx)
	// Use the iterator.
	_ = it

	// Order the posts by the number of comments they have recieved.
	datastore.NewQuery("Post").Order("-Comments")

	// Start listing from an offset and limit the results.
	datastore.NewQuery("Post").Offset(20).Limit(10)
}
