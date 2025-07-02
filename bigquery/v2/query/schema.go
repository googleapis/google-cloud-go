package query

import (
	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"github.com/apache/arrow/go/v15/arrow"
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
	// ModeNullable
	ModeNullable FieldMode = "NULLABLE"
	// ModeRequired
	ModeRequired FieldMode = "REQUIRED"
	// IntegerFieldType is a integer field type.
	ModeRepeated FieldMode = "REPEATED"
)

// internal schema struct with some optimization to parse row data
// we should steers users on using bigquerypb.Schema externally
type schema struct {
	pb          *bigquerypb.TableSchema
	arrowSchema *arrow.Schema
}

func newSchema(pb *bigquerypb.TableSchema) *schema {
	return &schema{pb: pb, arrowSchema: arrowSchemaFromBigQuerySchema(pb)}
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

func arrowSchemaFromBigQuerySchema(pb *bigquerypb.TableSchema) *arrow.Schema {
	if pb == nil {
		return nil
	}
	fields := arrowFieldsFromFields(pb.Fields)
	schema := arrow.NewSchema(fields, nil)
	return schema
}

func arrowFieldsFromFields(fields []*bigquerypb.TableFieldSchema) []arrow.Field {
	arfields := []arrow.Field{}
	for _, f := range fields {
		arfields = append(arfields, arrow.Field{
			Name:     f.Name,
			Type:     arrowTypeFromField(f),
			Nullable: f.Mode == string(ModeNullable),
		})
	}
	return arfields
}

// based on BigQuery Storage API conversion
// https://cloud.google.com/bigquery/docs/reference/storage#arrow_schema_details
func arrowTypeFromField(f *bigquerypb.TableFieldSchema) arrow.DataType {
	var baseType arrow.DataType
	switch FieldType(f.Type) {
	case StringFieldType:
		baseType = arrow.BinaryTypes.String
	case BytesFieldType:
		baseType = arrow.BinaryTypes.Binary
	case IntegerFieldType:
		baseType = arrow.PrimitiveTypes.Int64
	case FloatFieldType:
		baseType = arrow.PrimitiveTypes.Float64
	case BooleanFieldType:
		baseType = arrow.FixedWidthTypes.Boolean
	case TimestampFieldType:
		baseType = &arrow.TimestampType{
			Unit:     arrow.Microsecond,
			TimeZone: "UTC",
		}
	case DateFieldType:
		baseType = arrow.FixedWidthTypes.Date32
	case TimeFieldType:
		baseType = arrow.FixedWidthTypes.Time64us
	case DateTimeFieldType:
		baseType = &arrow.TimestampType{
			Unit:     arrow.Microsecond,
			TimeZone: "",
		}
	case NumericFieldType:
		baseType = &arrow.Decimal128Type{
			Precision: int32(f.Precision),
			Scale:     int32(f.Scale),
		}
	case BigNumericFieldType:
		baseType = &arrow.Decimal256Type{
			Precision: int32(f.Precision),
			Scale:     int32(f.Scale),
		}
	case RecordFieldType:
		fields := arrowFieldsFromFields(f.Fields)
		baseType = arrow.StructOf(fields...)
	case GeographyFieldType:
		baseType = arrow.BinaryTypes.String
	case JSONFieldType:
		baseType = arrow.BinaryTypes.String
	case RangeFieldType:
		panic("range not supported yet")
	case IntervalFieldType:
		panic("internal not supported yet")
	}

	if f.Mode == string(ModeRepeated) {
		return arrow.ListOf(baseType)
	}
	return baseType
}
