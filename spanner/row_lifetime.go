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

import "sync"

type rawRowData struct {
	vals    [][]byte
	release func()
}

var rawRows sync.Map // map[*Row]rawRowData

func rawValsForRow(row *Row) ([][]byte, bool) {
	if row == nil {
		return nil, false
	}
	v, ok := rawRows.Load(row)
	if !ok {
		return nil, false
	}
	return v.(rawRowData).vals, true
}

func setRawRow(row *Row, vals [][]byte, release func()) {
	rawRows.Store(row, rawRowData{vals: vals, release: release})
}

func releaseRawRow(row *Row) {
	if row == nil {
		return
	}
	v, ok := rawRows.LoadAndDelete(row)
	if !ok {
		return
	}
	if release := v.(rawRowData).release; release != nil {
		release()
	}
}
