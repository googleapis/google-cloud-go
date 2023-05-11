// Copyright 2022 Google LLC
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

package gensnippets

import (
	"fmt"
	"sort"
	"strings"

	"cloud.google.com/go/internal/gensnippets/metadata"
)

type apiInfo struct {
	// protoPkg is the proto namespace for the API package.
	protoPkg string
	// libPkg is the gapic import path.
	libPkg string
	// protoServices is a map of gapic client short names to service structs.
	protoServices map[string]*service
	// version is the Go module version for the gapic client.
	version string
	// shortName for the service.
	shortName string
}

// RegionTags gets the region tags keyed by client name and method name.
func (ai *apiInfo) RegionTags() map[string]map[string]string {
	regionTags := map[string]map[string]string{}
	for svcName, svc := range ai.protoServices {
		regionTags[svcName] = map[string]string{}
		for mName, m := range svc.methods {
			regionTags[svcName][mName] = m.regionTag
		}
	}
	return regionTags
}

// RegionTags gets the region tags keyed by client name and method name.
func (ai *apiInfo) ToSnippetMetadata() *metadata.Index {
	index := &metadata.Index{
		ClientLibrary: &metadata.ClientLibrary{
			Name:     ai.libPkg,
			Version:  ai.version,
			Language: metadata.Language_GO,
			Apis: []*metadata.Api{
				{
					Id:      ai.protoPkg,
					Version: ai.protoVersion(),
				},
			},
		},
	}

	// Sorting keys to stabilize output
	var svcKeys []string
	for k := range ai.protoServices {
		svcKeys = append(svcKeys, k)
	}
	sort.StringSlice(svcKeys).Sort()
	for _, clientShortName := range svcKeys {
		service := ai.protoServices[clientShortName]
		var methodKeys []string
		for k := range service.methods {
			methodKeys = append(methodKeys, k)
		}
		sort.StringSlice(methodKeys).Sort()
		for _, methodShortName := range methodKeys {
			method := service.methods[methodShortName]
			snip := &metadata.Snippet{
				RegionTag:   method.regionTag,
				Title:       fmt.Sprintf("%s %s Sample", ai.shortName, methodShortName),
				Description: strings.TrimSpace(method.doc),
				File:        fmt.Sprintf("%s/%s/main.go", clientShortName, methodShortName),
				Language:    metadata.Language_GO,
				Canonical:   false,
				Origin:      *metadata.Snippet_API_DEFINITION.Enum(),
				ClientMethod: &metadata.ClientMethod{
					ShortName:  methodShortName,
					FullName:   fmt.Sprintf("%s.%s.%s", ai.protoPkg, clientShortName, methodShortName),
					Async:      false,
					ResultType: method.result,
					Client: &metadata.ServiceClient{
						ShortName: clientShortName,
						FullName:  fmt.Sprintf("%s.%s", ai.protoPkg, clientShortName),
					},
					Method: &metadata.Method{
						ShortName: methodShortName,
						FullName:  fmt.Sprintf("%s.%s.%s", ai.protoPkg, service.protoName, methodShortName),
						Service: &metadata.Service{
							ShortName: service.protoName,
							FullName:  fmt.Sprintf("%s.%s", ai.protoPkg, service.protoName),
						},
					},
				},
			}
			segment := &metadata.Snippet_Segment{
				Start: int32(method.regionTagStart + 1),
				End:   int32(method.regionTagEnd - 1),
				Type:  metadata.Snippet_Segment_FULL,
			}
			snip.Segments = append(snip.Segments, segment)
			for _, param := range method.params {
				methParam := &metadata.ClientMethod_Parameter{
					Type: param.pType,
					Name: param.name,
				}
				snip.ClientMethod.Parameters = append(snip.ClientMethod.Parameters, methParam)
			}
			index.Snippets = append(index.Snippets, snip)
		}
	}
	return index
}

func (ai *apiInfo) protoVersion() string {
	ss := strings.Split(ai.protoPkg, ".")
	return ss[len(ss)-1]
}

// service associates a proto service from gapic metadata with gapic client and its methods
type service struct {
	// protoName is the name of the proto service.
	protoName string
	// methods is a map of gapic method short names to method structs.
	methods map[string]*method
}

// method associates elements of gapic client methods (docs, params and return types)
// with snippet file details such as the region tag string and line numbers.
type method struct {
	// doc is the documention for the methods.
	doc string
	// regionTag is the region tag that will be used for the generated snippet.
	regionTag string
	// regionTagStart is the line number of the START region tag in the snippet file.
	regionTagStart int
	// regionTagEnd is the line number of the END region tag in the snippet file.
	regionTagEnd int
	// params are the input parameters for the gapic method.
	params []*param
	// result is the return value for the method.
	result string
}

type param struct {
	// name of the parameter.
	name string
	// pType is the Go type for the parameter.
	pType string
}
