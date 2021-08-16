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

package testdata

import "cloud.google.com/go/bigquery"

var (
	SimpleMessageSchema bigquery.Schema = bigquery.Schema{
		{Name: "name", Type: bigquery.StringFieldType, Required: true},
		{Name: "value", Type: bigquery.IntegerFieldType},
	}

	GithubArchiveSchema bigquery.Schema = bigquery.Schema{
		{Name: "type", Type: bigquery.StringFieldType},
		{Name: "public", Type: bigquery.BooleanFieldType},
		{Name: "payload", Type: bigquery.StringFieldType},
		{
			Name: "repo",
			Type: bigquery.RecordFieldType,
			Schema: bigquery.Schema{
				{Name: "id", Type: bigquery.IntegerFieldType},
				{Name: "name", Type: bigquery.StringFieldType},
				{Name: "url", Type: bigquery.StringFieldType},
			},
		},
		{
			Name: "actor",
			Type: bigquery.RecordFieldType,
			Schema: bigquery.Schema{
				{Name: "id", Type: bigquery.IntegerFieldType},
				{Name: "login", Type: bigquery.StringFieldType},
				{Name: "gravatar_id", Type: bigquery.StringFieldType},
				{Name: "avatar_url", Type: bigquery.StringFieldType},
				{Name: "url", Type: bigquery.StringFieldType},
			},
		},
		{
			Name: "org",
			Type: bigquery.RecordFieldType,
			Schema: bigquery.Schema{
				{Name: "id", Type: bigquery.IntegerFieldType},
				{Name: "login", Type: bigquery.StringFieldType},
				{Name: "gravatar_id", Type: bigquery.StringFieldType},
				{Name: "avatar_url", Type: bigquery.StringFieldType},
				{Name: "url", Type: bigquery.StringFieldType},
			},
		},
		{Name: "created_at", Type: bigquery.TimestampFieldType},
		{Name: "id", Type: bigquery.StringFieldType},
		{Name: "other", Type: bigquery.StringFieldType},
	}
)
