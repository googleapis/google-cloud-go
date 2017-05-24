/*
Copyright 2017 Google Inc. All Rights Reserved.

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
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
)

// A KeySet defines a collection of Cloud Spanner keys and/or key ranges. All the
// keys are expected to be in the same table or index. The keys need not be sorted in
// any particular way.
//
// An individual Key can act as a KeySet, as can a KeyRange. Use the KeySets function
// to create a KeySet consisting of multiple Keys and KeyRanges. To obtain an empty
// KeySet, call KeySets with no arguments.
//
// If the same key is specified multiple times in the set (for example if two
// ranges, two keys, or a key and a range overlap), the Cloud Spanner backend behaves
// as if the key were only specified once.
type KeySet interface {
	keySetProto() (*sppb.KeySet, error)
}

// AllKeys returns a KeySet that represents all Keys of a table or a index.
func AllKeys() KeySet {
	return all{}
}

type all struct{}

func (all) keySetProto() (*sppb.KeySet, error) {
	return &sppb.KeySet{All: true}, nil
}

// KeySets returns the union of the KeySets. If any of the KeySets is AllKeys, then
// the resulting KeySet will be equivalent to AllKeys.
func KeySets(keySets ...KeySet) KeySet {
	u := make(union, len(keySets))
	copy(u, keySets)
	return u
}

type union []KeySet

func (u union) keySetProto() (*sppb.KeySet, error) {
	upb := &sppb.KeySet{}
	for _, ks := range u {
		pb, err := ks.keySetProto()
		if err != nil {
			return nil, err
		}
		if pb.All {
			return pb, nil
		}
		upb.Keys = append(upb.Keys, pb.Keys...)
		upb.Ranges = append(upb.Ranges, pb.Ranges...)
	}
	return upb, nil
}
