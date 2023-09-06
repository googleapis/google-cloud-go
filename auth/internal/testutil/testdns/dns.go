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

// Package testgcs is a light DNS client used for testings to avoid pulling in
// dependencies.
package testdns

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
	"cloud.google.com/go/auth/internal/testutil"
)

type Client struct {
	client *http.Client
}

func NewClient(tp auth.TokenProvider) *Client {
	client := internal.CloneDefaultClient()
	testutil.AddAuthorizationMiddleware(client, tp)
	return &Client{
		client: client,
	}
}

func (c *Client) GetProject(ctx context.Context, projectID string) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://dns.googleapis.com/dns/v1/projects/%s", projectID), nil)
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		errBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("%s", errBody)
	}
	return nil
}
