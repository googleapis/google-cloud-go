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

	SimpleMessageEvolvedSchema bigquery.Schema = bigquery.Schema{
		{Name: "name", Type: bigquery.StringFieldType, Required: true},
		{Name: "value", Type: bigquery.IntegerFieldType},
		{Name: "other", Type: bigquery.StringFieldType},
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

	ComplexTypeSchema bigquery.Schema = bigquery.Schema{
		{
			Name:     "nested_repeated_type",
			Type:     bigquery.RecordFieldType,
			Repeated: true,
			Schema: bigquery.Schema{
				{
					Name:     "inner_type",
					Type:     bigquery.RecordFieldType,
					Repeated: true,
					Schema: bigquery.Schema{
						{Name: "value", Type: bigquery.StringFieldType, Repeated: true},
					},
				},
			},
		},
		{
			Name: "inner_type",
			Type: bigquery.RecordFieldType,
			Schema: bigquery.Schema{
				{Name: "value", Type: bigquery.StringFieldType, Repeated: true},
			},
		},
	}

	// We currently follow proto2 rules here, hence the well known types getting treated as records.
	WithWellKnownTypesSchema bigquery.Schema = bigquery.Schema{
		{Name: "int64_value", Type: bigquery.IntegerFieldType},
		{
			Name: "wrapped_int64",
			Type: bigquery.RecordFieldType,
			Schema: bigquery.Schema{
				{Name: "value", Type: bigquery.IntegerFieldType},
			},
		},
		{Name: "string_value", Type: bigquery.StringFieldType, Repeated: true},
		{
			Name:     "wrapped_string",
			Type:     bigquery.RecordFieldType,
			Repeated: true,
			Schema: bigquery.Schema{
				{Name: "value", Type: bigquery.StringFieldType},
			},
		},
	}

	ExternalEnumMessageSchema bigquery.Schema = bigquery.Schema{
		{
			Name: "msg_a",
			Type: bigquery.RecordFieldType,
			Schema: bigquery.Schema{
				{Name: "foo", Type: bigquery.StringFieldType},
				{Name: "bar", Type: bigquery.IntegerFieldType},
			},
		},
		{
			Name: "msg_b",
			Type: bigquery.RecordFieldType,
			Schema: bigquery.Schema{
				{Name: "baz", Type: bigquery.IntegerFieldType},
			},
		},
	}

	ValidationBaseSchema bigquery.Schema = bigquery.Schema{
		{
			Name: "double_field",
			Type: bigquery.FloatFieldType,
		},
		{
			Name: "float_field",
			Type: bigquery.FloatFieldType,
		},
		{
			Name: "int32_field",
			Type: bigquery.IntegerFieldType,
		},
		{
			Name: "int64_field",
			Type: bigquery.IntegerFieldType,
		},
		{
			Name: "uint32_field",
			Type: bigquery.IntegerFieldType,
		},
		{
			Name: "sint32_field",
			Type: bigquery.IntegerFieldType,
		},
		{
			Name: "sint64_field",
			Type: bigquery.IntegerFieldType,
		},
		{
			Name: "fixed32_field",
			Type: bigquery.IntegerFieldType,
		},
		{
			Name: "sfixed32_field",
			Type: bigquery.IntegerFieldType,
		},
		{
			Name: "sfixed64_field",
			Type: bigquery.IntegerFieldType,
		},
		{
			Name: "bool_field",
			Type: bigquery.BooleanFieldType,
		},
		{
			Name: "string_field",
			Type: bigquery.StringFieldType,
		},
		{
			Name: "bytes_field",
			Type: bigquery.BytesFieldType,
		},
		{
			Name: "enum_field",
			Type: bigquery.IntegerFieldType,
		},
	}

	ValidationRepeatedSchema bigquery.Schema = bigquery.Schema{
		{
			Name: "id",
			Type: bigquery.IntegerFieldType,
		},
		{
			Name:     "double_repeated",
			Type:     bigquery.FloatFieldType,
			Repeated: true,
		},
		{
			Name:     "float_repeated",
			Type:     bigquery.FloatFieldType,
			Repeated: true,
		},
		{
			Name:     "int32_repeated",
			Type:     bigquery.IntegerFieldType,
			Repeated: true,
		},
		{
			Name:     "int64_repeated",
			Type:     bigquery.IntegerFieldType,
			Repeated: true,
		},
		{
			Name:     "uint32_repeated",
			Type:     bigquery.IntegerFieldType,
			Repeated: true,
		},
		{
			Name:     "sint32_repeated",
			Type:     bigquery.IntegerFieldType,
			Repeated: true,
		},
		{
			Name:     "sint64_repeated",
			Type:     bigquery.IntegerFieldType,
			Repeated: true,
		},
		{
			Name:     "fixed32_repeated",
			Type:     bigquery.IntegerFieldType,
			Repeated: true,
		},
		{
			Name:     "sfixed32_repeated",
			Type:     bigquery.IntegerFieldType,
			Repeated: true,
		},
		{
			Name:     "sfixed64_repeated",
			Type:     bigquery.IntegerFieldType,
			Repeated: true,
		},
		{
			Name:     "bool_repeated",
			Type:     bigquery.BooleanFieldType,
			Repeated: true,
		},
		{
			Name:     "enum_repeated",
			Type:     bigquery.IntegerFieldType,
			Repeated: true,
		},
	}

	ValidationColumnAnnotations bigquery.Schema = bigquery.Schema{
		{
			Name: "first",
			Type: bigquery.StringFieldType,
		},
		{
			Name: "second",
			Type: bigquery.StringFieldType,
		},
		{
			Name: "特別コラム",
			Type: bigquery.StringFieldType,
		},
	}

	ExampleEmployeeSchema bigquery.Schema = bigquery.Schema{
		{
			Name: "id",
			Type: bigquery.IntegerFieldType,
		},
		{
			Name: "username",
			Type: bigquery.StringFieldType,
		},
		{
			Name: "given_name",
			Type: bigquery.StringFieldType,
		},
		{
			Name:     "departments",
			Type:     bigquery.StringFieldType,
			Repeated: true,
		},
		{
			Name: "salary",
			Type: bigquery.IntegerFieldType,
		},
	}

	DefaultValueSchema bigquery.Schema = bigquery.Schema{
		{
			Name: "id",
			Type: bigquery.StringFieldType,
		},
		{
			Name: "strcol",
			Type: bigquery.StringFieldType,
		},
		{
			Name:                   "strcol_withdef",
			Type:                   bigquery.StringFieldType,
			DefaultValueExpression: "\"defaultvalue\"",
		},
		{
			Name: "intcol",
			Type: bigquery.IntegerFieldType,
		},
		{
			Name:                   "intcol_withdef",
			Type:                   bigquery.IntegerFieldType,
			DefaultValueExpression: "-99",
		},
		{
			Name: "otherstr",
			Type: bigquery.StringFieldType,
		},
		{
			Name:                   "otherstr_withdef",
			Type:                   bigquery.StringFieldType,
			DefaultValueExpression: "\"otherval\"",
		},
	}
)
