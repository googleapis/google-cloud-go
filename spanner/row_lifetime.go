/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

type rowLifetime struct {
	release func()
	rawVals [][]byte
}

func rowLifetimeFor(row *Row) *rowLifetime {
	if row.release == nil {
		row.release = new(rowLifetime)
	}
	return row.release
}

func setRowRelease(row *Row, release func()) {
	if row != nil {
		rowLifetimeFor(row).release = release
	}
}

func setRawValsForRow(row *Row, values [][]byte) {
	if row != nil {
		if values == nil && row.release == nil {
			return
		}
		rowLifetimeFor(row).rawVals = values
	}
}

func rawValsForRow(row *Row) [][]byte {
	if row == nil || row.release == nil {
		return nil
	}
	return row.release.rawVals
}

func releaseRawRow(row *Row) {
	if row == nil || row.release == nil {
		return
	}
	release := row.release.release
	row.release.release = nil
	row.release.rawVals = nil
	if release != nil {
		release()
	}
}
