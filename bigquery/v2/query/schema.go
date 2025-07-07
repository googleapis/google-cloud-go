// Copyright 2025 Google LLC
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

package query

import (
	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
)

// FieldType is the type of field.
type FieldType string

const (
	// StringFieldType is a string field type.
	StringFieldType FieldType = "STRING"
	// BytesFieldType is a bytes field type.
	BytesFieldType FieldType = "BYTES"
	// IntegerFieldType is a integer field type.
	IntegerFieldType FieldType = "INTEGER"
	// FloatFieldType is a float field type.
	FloatFieldType FieldType = "FLOAT"
	// BooleanFieldType is a boolean field type.
	BooleanFieldType FieldType = "BOOLEAN"
	// TimestampFieldType is a timestamp field type.
	TimestampFieldType FieldType = "TIMESTAMP"
	// RecordFieldType is a record field type. It is typically used to create columns with repeated or nested data.
	RecordFieldType FieldType = "RECORD"
	// DateFieldType is a date field type.
	DateFieldType FieldType = "DATE"
	// TimeFieldType is a time field type.
	TimeFieldType FieldType = "TIME"
	// DateTimeFieldType is a datetime field type.
	DateTimeFieldType FieldType = "DATETIME"
	// NumericFieldType is a numeric field type. Numeric types include integer types, floating point types and the
	// NUMERIC data type.
	NumericFieldType FieldType = "NUMERIC"
	// GeographyFieldType is a string field type.  Geography types represent a set of points
	// on the Earth's surface, represented in Well Known Text (WKT) format.
	GeographyFieldType FieldType = "GEOGRAPHY"
	// BigNumericFieldType is a numeric field type that supports values of larger precision
	// and scale than the NumericFieldType.
	BigNumericFieldType FieldType = "BIGNUMERIC"
	// IntervalFieldType is a representation of a duration or an amount of time.
	IntervalFieldType FieldType = "INTERVAL"
	// JSONFieldType is a representation of a json object.
	JSONFieldType FieldType = "JSON"
	// RangeFieldType represents a continuous range of values.
	RangeFieldType FieldType = "RANGE"
)

// FieldMode is the mode of field.
type FieldMode string

const (
	// ModeNullable marks the field as nullable.
	ModeNullable FieldMode = "NULLABLE"
	// ModeRequired marks the field as required.
	ModeRequired FieldMode = "REQUIRED"
	// ModeRepeated marks the field as an array.
	ModeRepeated FieldMode = "REPEATED"
)

// internal schema struct with some optimization to parse row data
// we should steers users on using bigquerypb.Schema externally
type schema struct {
	pb *bigquerypb.TableSchema
}

func newSchema(pb *bigquerypb.TableSchema) *schema {
	return &schema{pb: pb}
}

func newSchemaFromField(field *bigquerypb.TableFieldSchema) *schema {
	s := &bigquerypb.TableSchema{
		Fields: field.Fields,
	}
	return newSchema(s)
}

func (s *schema) len() int {
	return len(s.pb.Fields)
}
