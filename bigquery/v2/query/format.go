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
	"fmt"
	"math/big"
	"time"

	"cloud.google.com/go/civil"
)

const (
	timestampFormat = "2006-01-02 15:04:05.999999-07:00"
)

func parseCivilDate(s string) (civil.Date, error) {
	cdt, err := civil.ParseDate(s)
	if err != nil {
		t, err := time.Parse("2006-1-2", s)
		if err != nil {
			return civil.Date{}, err
		}
		return civil.DateOf(t), err
	}
	return cdt, nil
}

func bigNumericString(r *big.Rat) string {
	return r.FloatString(38)
}

func civilTimeString(t civil.Time) string {
	if t.Nanosecond == 0 {
		return t.String()
	}
	micro := (t.Nanosecond + 500) / 1000 // round to nearest microsecond
	t.Nanosecond = 0
	return t.String() + fmt.Sprintf(".%06d", micro)
}

func civilDateString(d civil.Date) string {
	return d.String()
}

func civilDateTimeString(dt civil.DateTime) string {
	return dt.Date.String() + " " + civilTimeString(dt.Time)
}

func timestampString(ts time.Time) string {
	return ts.Format(timestampFormat)
}
