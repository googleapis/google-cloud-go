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

// Code generated by protoc-gen-go_gapic. DO NOT EDIT.

//go:build go1.23

package networkservices

import (
	"iter"

	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	networkservicespb "cloud.google.com/go/networkservices/apiv1/networkservicespb"
	"github.com/googleapis/gax-go/v2/iterator"
	locationpb "google.golang.org/genproto/googleapis/cloud/location"
)

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *AuthzExtensionIterator) All() iter.Seq2[*networkservicespb.AuthzExtension, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *EndpointPolicyIterator) All() iter.Seq2[*networkservicespb.EndpointPolicy, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *GatewayIterator) All() iter.Seq2[*networkservicespb.Gateway, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *GatewayRouteViewIterator) All() iter.Seq2[*networkservicespb.GatewayRouteView, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *GrpcRouteIterator) All() iter.Seq2[*networkservicespb.GrpcRoute, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *HttpRouteIterator) All() iter.Seq2[*networkservicespb.HttpRoute, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *LbRouteExtensionIterator) All() iter.Seq2[*networkservicespb.LbRouteExtension, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *LbTrafficExtensionIterator) All() iter.Seq2[*networkservicespb.LbTrafficExtension, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *LocationIterator) All() iter.Seq2[*locationpb.Location, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *MeshIterator) All() iter.Seq2[*networkservicespb.Mesh, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *MeshRouteViewIterator) All() iter.Seq2[*networkservicespb.MeshRouteView, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *OperationIterator) All() iter.Seq2[*longrunningpb.Operation, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *ServiceBindingIterator) All() iter.Seq2[*networkservicespb.ServiceBinding, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *ServiceLbPolicyIterator) All() iter.Seq2[*networkservicespb.ServiceLbPolicy, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *TcpRouteIterator) All() iter.Seq2[*networkservicespb.TcpRoute, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *TlsRouteIterator) All() iter.Seq2[*networkservicespb.TlsRoute, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *WasmPluginIterator) All() iter.Seq2[*networkservicespb.WasmPlugin, error] {
	return iterator.RangeAdapter(it.Next)
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *WasmPluginVersionIterator) All() iter.Seq2[*networkservicespb.WasmPluginVersion, error] {
	return iterator.RangeAdapter(it.Next)
}
