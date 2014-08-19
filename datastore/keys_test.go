// Copyright 2014 Google Inc. All Rights Reserved.
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

package datastore

import "testing"

func TestKeyEqual(t *testing.T) {
	tests := []struct {
		k0, k1 *Key
		eq     bool
	}{
		{nil, nil, true},
		{&Key{}, nil, false},
		{&Key{}, &Key{}, true},
		{&Key{namespace: "ns1"}, &Key{namespace: "ns2"}, false},
	}
	for _, test := range tests {
		assertKeyEqual(t, test.k0, test.k1, test.eq)
	}

	k := &Key{
		fullPath: []*Path{
			&Path{Kind: "Parent", Name: "name"},
			&Path{Kind: "Child", ID: 123},
		},
	}
	o := &Key{
		fullPath: []*Path{
			&Path{Kind: "Parent", Name: "name"},
		},
	}
	assertKeyEqual(t, k, o, false)

	k = &Key{
		fullPath: []*Path{
			&Path{Kind: "Parent", Name: "name"},
			&Path{Kind: "Child", ID: 123},
		},
	}
	o = &Key{
		fullPath: []*Path{
			&Path{Kind: "Parent", Name: "name"},
			&Path{Kind: "Child", ID: 456},
		},
	}
	assertKeyEqual(t, k, o, false)

	k = &Key{
		fullPath: []*Path{
			&Path{Kind: "Parent", Name: "name1"},
		},
	}
	o = &Key{
		fullPath: []*Path{
			&Path{Kind: "Parent", Name: "name2"},
		},
	}
	assertKeyEqual(t, k, o, false)

	k = &Key{
		namespace: "ns1",
		fullPath: []*Path{
			&Path{Kind: "Parent", Name: "name"},
			&Path{Kind: "Child", ID: 123},
		},
	}
	o = &Key{
		namespace: "ns1",
		fullPath: []*Path{
			&Path{Kind: "Parent", Name: "name"},
			&Path{Kind: "Child", ID: 123},
		},
	}
	assertKeyEqual(t, k, o, true)

}

func assertKeyEqual(t *testing.T, k, o *Key, expected bool) {
	if k.Equal(o) != expected {
		t.Errorf("%#v == %#v\n\tgot: %v; want: %v\n", k, o, !expected, expected)
	} else if o.Equal(k) != expected {
		t.Errorf("%#v == %#v\n\tgot: %v; want: %v\n", o, k, !expected, expected)
	}
}
