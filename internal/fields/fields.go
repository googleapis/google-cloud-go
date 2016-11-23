// Copyright 2016 Google Inc. All Rights Reserved.
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

// Package fields provides a view of the fields of a struct that follows the Go
// rules, amended to consider tags and case insensitivity.
//
// Usage
//
// First define a function that interprets tags:
//
//   func parseTag(st reflect.StructTag) (name string, keep bool, other interface{}) { ... }
//
// The function's return values describe whether to ignore the field
// completely or provide an alternate name, as well as other data from the
// parse that is stored to avoid re-parsing.
//
// Next, construct a Cache, passing your function. As its name suggests, a
// Cache remembers field information for a type, so subsequent calls with the
// same type are very fast.
//
//    cache := fields.NewCache(parseTag)
//
// To get the fields of a struct type as determined by the above rules, call
// the Fields method:
//
//    fields := cache.Fields(reflect.TypeOf(MyStruct{}))
//
// The return value can be treated as a slice of Fields.
//
// Given a string, such as a key or column name obtained during unmarshalling,
// call Match on the list of fields to find a field whose name is the best
// match:
//
//   field := fields.Match(name)
//
// Match looks for an exact match first, then falls back to a case-insensitive
// comparison.
package fields

import (
	"bytes"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
)

// A Field records information about a struct field.
type Field struct {
	Name        string       // effective field name
	NameFromTag bool         // did Name come from a tag?
	Type        reflect.Type // field type
	Index       []int        // index sequence, for reflect.Value.FieldByIndex
	ParsedTag   interface{}  // third return value of the parseTag function
}

// A Cache records information about the fields of struct types.
//
// A Cache is safe for use by multiple goroutines.
type Cache struct {
	parseTag func(reflect.StructTag) (name string, keep bool, other interface{})
	cache    atomic.Value // map[reflect.Type][]Field
	mu       sync.Mutex   // used only by writers of cache
}

// NewCache constructs a Cache. Its argument should be a function that accepts
// a struct tag and returns three values: an alternative name for the field
// extracted from the tag, a boolean saying whether to keep the field or ignore
// it, and additional data that is stored with the field information to avoid
// having to parse the tag again.
func NewCache(parseTag func(reflect.StructTag) (name string, keep bool, other interface{})) *Cache {
	if parseTag == nil {
		parseTag = func(reflect.StructTag) (string, bool, interface{}) {
			return "", true, nil
		}
	}
	return &Cache{parseTag: parseTag}
}

// A fieldScan represents an item on the fieldByNameFunc scan work list.
type fieldScan struct {
	typ   reflect.Type
	index []int
}

// Fields returns all the exported fields of t, which must be a struct type. It
// follows the standard Go rules for embedded fields, modified by the presence
// of tags. The result is sorted by field name.
// If not nil, the given parseTag function should extract and return a name
// from the struct tag. This name is used instead of the field's declared name.
//
// These rules apply in the absence of tags:
// Anonymous struct fields are treated as if their inner exported fields were
// fields in the outer struct (embedding). The result includes all fields that
// aren't shadowed by fields at higher level of embedding. If more than one
// field with the same name exists at the same level of embedding, it is
// excluded. An anonymous field that is not of struct type is treated as having
// its type as its name.
//
// Tags modify these rules as follows:
// A field's tag is used as its name.
// An anonymous struct field with a name given in its tag is treated as
// a field having that name, rather than an embedded struct (the struct's
// fields will not be returned).
// If more than one field with the same name exists at the same level of embedding,
// but exactly one of them is tagged, then the tagged field is reported and the others
// are ignored.
func (c *Cache) Fields(t reflect.Type) List {
	if t.Kind() != reflect.Struct {
		panic("fields: Fields of non-struct type")
	}
	return List(c.cachedTypeFields(t))
}

// A List is a list of Fields.
type List []Field

// Match returns the field in the list whose name best matches the supplied name, nor nil
// if no field does. If there is a field with the exact name, it is returned.
// Otherwise the first field whose name matches case-insensitively is returned.
func (l List) Match(name string) *Field {
	return l.MatchBytes([]byte(name))
}

// MatchBytes is identical to Match, except that the argument is a byte slice.
func (l List) MatchBytes(name []byte) *Field {
	var f *Field
	for i := range l {
		ff := &l[i]
		nameBytes := []byte(ff.Name)
		if bytes.Equal(nameBytes, name) {
			return ff
		}
		if f == nil && bytes.EqualFold(nameBytes, name) {
			f = ff
		}
	}
	return f
}

// cachedTypeFields is like typeFields but uses a cache to avoid repeated work.
// This code has been copied and modified from
// https://go.googlesource.com/go/+/go1.7.3/src/encoding/json/encode.go.
func (c *Cache) cachedTypeFields(t reflect.Type) []Field {
	mp, _ := c.cache.Load().(map[reflect.Type][]Field)
	f := mp[t]
	if f != nil {
		return f
	}

	// Compute fields without lock.
	// Might duplicate effort but won't hold other computations back.
	f = c.typeFields(t)
	if f == nil {
		f = []Field{}
	}

	c.mu.Lock()
	mp, _ = c.cache.Load().(map[reflect.Type][]Field)
	newM := make(map[reflect.Type][]Field, len(mp)+1)
	for k, v := range mp {
		newM[k] = v
	}
	newM[t] = f
	c.cache.Store(newM)
	c.mu.Unlock()
	return f
}

func (c *Cache) typeFields(t reflect.Type) []Field {
	fields := c.listFields(t)
	sort.Sort(byName(fields))
	// Delete all fields that are hidden by the Go rules for embedded fields.

	// The fields are sorted in primary order of name, secondary order of field
	// index length. So the first field with a given name is the dominant one.
	var out []Field
	for advance, i := 0, 0; i < len(fields); i += advance {
		// One iteration per name.
		// Find the sequence of fields with the name of this first field.
		fi := fields[i]
		name := fi.Name
		for advance = 1; i+advance < len(fields); advance++ {
			fj := fields[i+advance]
			if fj.Name != name {
				break
			}
		}
		// Find the dominant field, if any, out of all fields that have the same name.
		dominant, ok := dominantField(fields[i : i+advance])
		if ok {
			out = append(out, dominant)
		}
	}
	return out
}

func (c *Cache) listFields(t reflect.Type) []Field {
	// This uses the same condition that the Go language does: there must be a unique instance
	// of the match at a given depth level. If there are multiple instances of a match at the
	// same depth, they annihilate each other and inhibit any possible match at a lower level.
	// The algorithm is breadth first search, one depth level at a time.

	// The current and next slices are work queues:
	// current lists the fields to visit on this depth level,
	// and next lists the fields on the next lower level.
	current := []fieldScan{}
	next := []fieldScan{{typ: t}}

	// nextCount records the number of times an embedded type has been
	// encountered and considered for queueing in the 'next' slice.
	// We only queue the first one, but we increment the count on each.
	// If a struct type T can be reached more than once at a given depth level,
	// then it annihilates itself and need not be considered at all when we
	// process that next depth level.
	var nextCount map[reflect.Type]int

	// visited records the structs that have been considered already.
	// Embedded pointer fields can create cycles in the graph of
	// reachable embedded types; visited avoids following those cycles.
	// It also avoids duplicated effort: if we didn't find the field in an
	// embedded type T at level 2, we won't find it in one at level 4 either.
	visited := map[reflect.Type]bool{}

	var fields []Field // Fields found.

	for len(next) > 0 {
		current, next = next, current[:0]
		count := nextCount
		nextCount = nil

		// Process all the fields at this depth, now listed in 'current'.
		// The loop queues embedded fields found in 'next', for processing during the next
		// iteration. The multiplicity of the 'current' field counts is recorded
		// in 'count'; the multiplicity of the 'next' field counts is recorded in 'nextCount'.
		for _, scan := range current {
			t := scan.typ
			if visited[t] {
				// We've looked through this type before, at a higher level.
				// That higher level would shadow the lower level we're now at,
				// so this one can't be useful to us. Ignore it.
				continue
			}
			visited[t] = true
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				exported := (f.PkgPath == "")

				// If a named field is unexported, ignore it. An anonymous
				// unexported field is processed, because it may contain
				// exported fields, which are visible.
				if !exported && !f.Anonymous {
					continue
				}

				// Examine the tag.
				tagName, keep, other := c.parseTag(f.Tag)
				if !keep {
					continue
				}

				var ntyp reflect.Type
				if f.Anonymous {
					// Anonymous field of type T or *T.
					ntyp = f.Type
					if ntyp.Kind() == reflect.Ptr {
						ntyp = ntyp.Elem()
					}
				}

				// Record fields with a tag name, non-anonymous fields, or
				// anonymous non-struct fields.
				if tagName != "" || ntyp == nil || ntyp.Kind() != reflect.Struct {
					if !exported {
						continue
					}
					name := tagName
					if name == "" {
						name = f.Name
					}
					sf := Field{
						Name:        name,
						NameFromTag: tagName != "",
						Type:        f.Type,
						ParsedTag:   other,
					}
					sf.Index = append(sf.Index, scan.index...)
					sf.Index = append(sf.Index, i)
					fields = append(fields, sf)
					if count[t] > 1 {
						// If there were multiple instances, add a second,
						// so that the annihilation code will see a duplicate.
						fields = append(fields, fields[len(fields)-1])
					}
					continue
				}

				// Queue embedded struct fields for processing with next level,
				// but only if the embedded types haven't already been queued.
				if nextCount[ntyp] > 0 {
					nextCount[ntyp] = 2 // exact multiple doesn't matter
					continue
				}
				if nextCount == nil {
					nextCount = map[reflect.Type]int{}
				}
				nextCount[ntyp] = 1
				if count[t] > 1 {
					nextCount[ntyp] = 2 // exact multiple doesn't matter
				}
				var index []int
				index = append(index, scan.index...)
				index = append(index, i)
				next = append(next, fieldScan{ntyp, index})
			}
		}
	}
	return fields
}

// byName sorts field by name, breaking ties with depth, then breaking ties
// with index sequence.
type byName []Field

func (x byName) Len() int { return len(x) }

func (x byName) Swap(i, j int) { x[i], x[j] = x[j], x[i] }

func (x byName) Less(i, j int) bool {
	if x[i].Name != x[j].Name {
		return x[i].Name < x[j].Name
	}
	if len(x[i].Index) != len(x[j].Index) {
		return len(x[i].Index) < len(x[j].Index)
	}
	for k, xik := range x[i].Index {
		if xik != x[j].Index[k] {
			return xik < x[j].Index[k]
		}
	}
	return false
}

// dominantField looks through the fields, all of which are known to have the
// same name, to find the single field that dominates the others using Go's
// embedding rules, modified by the presence of tags. If there are multiple
// top-level fields, the boolean will be false: This condition is an error in
// Go and we skip all the fields.
func dominantField(fields []Field) (Field, bool) {
	// The fields are sorted in increasing index-length order. The winner
	// must therefore be one with the shortest index length. Drop all
	// longer entries, which is easy: just truncate the slice.
	length := len(fields[0].Index)
	tagged := -1 // Index of first tagged field.
	for i, f := range fields {
		if len(f.Index) > length {
			fields = fields[:i]
			break
		}
		if f.NameFromTag {
			if tagged >= 0 {
				// Multiple tagged fields at the same level: conflict.
				// Return no field.
				return Field{}, false
			}
			tagged = i
		}
	}
	if tagged >= 0 {
		return fields[tagged], true
	}
	// All remaining fields have the same length. If there's more than one,
	// we have a conflict (two fields named "X" at the same level) and we
	// return no field.
	if len(fields) > 1 {
		return Field{}, false
	}
	return fields[0], true
}
