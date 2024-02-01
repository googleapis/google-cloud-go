// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fakepb

// FooVersion_State is an int type.
type FooVersion_State int32

const (
	Foo_STATE_UNSPECIFIED  FooVersion_State = 0
	FooVersion_ENABLED     FooVersion_State = 1
	SecretVersion_DISABLED FooVersion_State = 2
)

var (
	FooVersion_State_name = map[int32]string{
		0: "STATE_UNSPECIFIED",
		1: "ENABLED",
		2: "DISABLED",
	}
	FooVersion_State_value = map[string]int32{
		"STATE_UNSPECIFIED": 0,
		"ENABLED":           1,
		"DISABLED":          2,
	}
)

// CreateFooRequest is a struct.
type CreateFooRequest struct{}

// CreateFooResponse is a struct.
type CreateFooResponse struct{}

// ListFoosRequest is a struct.
type ListFoosRequest struct{}

// ListFoosResponse is a struct.
type ListFoosResponse struct{}

// Foo is a struct.
type Foo struct {
	State FooVersion_State
}

const (
	dontAliasThisConst = 0
)

var (
	dontAliasThisVar = 0
)

type dontAliasThisInterface interface {
	Bar()
}
type dontAliasThisStruct struct {
	Bar string
}
