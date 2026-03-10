/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"database/sql"
	"math/big"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/google/uuid"
)

func TestNullTypesScan_TypedNil(t *testing.T) {
	// Helper to reduce boilerplate
	testScan := func(name string, scanner sql.Scanner, src interface{}) {
		t.Helper()
		// recovering from panic to verify we catch it if it happens (though we want to fix it so it doesn't panic)
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("%s: Scan panicked with %v", name, r)
			}
		}()

		if err := scanner.Scan(src); err != nil {
			t.Errorf("%s: Scan returned error: %v", name, err)
		}
		if v, ok := scanner.(interface{ IsValid() bool }); ok && v.IsValid() {
			t.Errorf("%s: Expected Valid to be false for nil input, got true", name)
		}
	}

	// NullUUID
	var nullUUID NullUUID
	testScan("NullUUID with (*uuid.UUID)(nil)", &nullUUID, (*uuid.UUID)(nil))
	testScan("NullUUID with (*string)(nil)", &nullUUID, (*string)(nil))

	// NullInt64
	var nullInt64 NullInt64
	testScan("NullInt64 with (*int64)(nil)", &nullInt64, (*int64)(nil))
	testScan("NullInt64 with (*string)(nil)", &nullInt64, (*string)(nil))

	// NullString
	var nullString NullString
	testScan("NullString with (*string)(nil)", &nullString, (*string)(nil))

	// NullFloat64
	var nullFloat64 NullFloat64
	testScan("NullFloat64 with (*float64)(nil)", &nullFloat64, (*float64)(nil))
	testScan("NullFloat64 with (*string)(nil)", &nullFloat64, (*string)(nil))

	// NullFloat32
	var nullFloat32 NullFloat32
	testScan("NullFloat32 with (*float32)(nil)", &nullFloat32, (*float32)(nil))
	testScan("NullFloat32 with (*string)(nil)", &nullFloat32, (*string)(nil))

	// NullBool
	var nullBool NullBool
	testScan("NullBool with (*bool)(nil)", &nullBool, (*bool)(nil))
	testScan("NullBool with (*string)(nil)", &nullBool, (*string)(nil))

	// NullTime
	var nullTime NullTime
	testScan("NullTime with (*time.Time)(nil)", &nullTime, (*time.Time)(nil))
	testScan("NullTime with (*string)(nil)", &nullTime, (*string)(nil))

	// NullDate
	var nullDate NullDate
	testScan("NullDate with (*civil.Date)(nil)", &nullDate, (*civil.Date)(nil))
	testScan("NullDate with (*string)(nil)", &nullDate, (*string)(nil))

	// NullNumeric
	var nullNumeric NullNumeric
	testScan("NullNumeric with (*big.Rat)(nil)", &nullNumeric, (*big.Rat)(nil))
	testScan("NullNumeric with (*string)(nil)", &nullNumeric, (*string)(nil))

	// PGNumeric
	var pgNumeric PGNumeric
	testScan("PGNumeric with (*big.Rat)(nil)", &pgNumeric, (*big.Rat)(nil))
	testScan("PGNumeric with (*string)(nil)", &pgNumeric, (*string)(nil))
	testScan("PGNumeric with (*float64)(nil)", &pgNumeric, (*float64)(nil))
}
