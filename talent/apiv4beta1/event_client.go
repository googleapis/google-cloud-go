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

package talent

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"time"

	longrunningpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	talentpb "cloud.google.com/go/talent/apiv4beta1/talentpb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	gtransport "google.golang.org/api/transport/grpc"
	httptransport "google.golang.org/api/transport/http"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
)

var newEventClientHook clientHook

// EventCallOptions contains the retry settings for each method of EventClient.
type EventCallOptions struct {
	CreateClientEvent []gax.CallOption
	GetOperation      []gax.CallOption
}

func defaultEventGRPCClientOptions() []option.ClientOption {
	return []option.ClientOption{
		internaloption.WithDefaultEndpoint("jobs.googleapis.com:443"),
		internaloption.WithDefaultEndpointTemplate("jobs.UNIVERSE_DOMAIN:443"),
		internaloption.WithDefaultMTLSEndpoint("jobs.mtls.googleapis.com:443"),
		internaloption.WithDefaultUniverseDomain("googleapis.com"),
		internaloption.WithDefaultAudience("https://jobs.googleapis.com/"),
		internaloption.WithDefaultScopes(DefaultAuthScopes()...),
		internaloption.EnableJwtWithScope(),
		internaloption.EnableNewAuthLibrary(),
		option.WithGRPCDialOption(grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(math.MaxInt32))),
	}
}

func defaultEventCallOptions() *EventCallOptions {
	return &EventCallOptions{
		CreateClientEvent: []gax.CallOption{
			gax.WithTimeout(30000 * time.Millisecond),
		},
		GetOperation: []gax.CallOption{},
	}
}

func defaultEventRESTCallOptions() *EventCallOptions {
	return &EventCallOptions{
		CreateClientEvent: []gax.CallOption{
			gax.WithTimeout(30000 * time.Millisecond),
		},
		GetOperation: []gax.CallOption{},
	}
}

// internalEventClient is an interface that defines the methods available from Cloud Talent Solution API.
type internalEventClient interface {
	Close() error
	setGoogleClientInfo(...string)
	Connection() *grpc.ClientConn
	CreateClientEvent(context.Context, *talentpb.CreateClientEventRequest, ...gax.CallOption) (*talentpb.ClientEvent, error)
	GetOperation(context.Context, *longrunningpb.GetOperationRequest, ...gax.CallOption) (*longrunningpb.Operation, error)
}

// EventClient is a client for interacting with Cloud Talent Solution API.
// Methods, except Close, may be called concurrently. However, fields must not be modified concurrently with method calls.
//
// A service handles client event report.
type EventClient struct {
	// The internal transport-dependent client.
	internalClient internalEventClient

	// The call options for this service.
	CallOptions *EventCallOptions
}

// Wrapper methods routed to the internal client.

// Close closes the connection to the API service. The user should invoke this when
// the client is no longer required.
func (c *EventClient) Close() error {
	return c.internalClient.Close()
}

// setGoogleClientInfo sets the name and version of the application in
// the `x-goog-api-client` header passed on each request. Intended for
// use by Google-written clients.
func (c *EventClient) setGoogleClientInfo(keyval ...string) {
	c.internalClient.setGoogleClientInfo(keyval...)
}

// Connection returns a connection to the API service.
//
// Deprecated: Connections are now pooled so this method does not always
// return the same resource.
func (c *EventClient) Connection() *grpc.ClientConn {
	return c.internalClient.Connection()
}

// CreateClientEvent report events issued when end user interacts with customer’s application
// that uses Cloud Talent Solution. You may inspect the created events in
// self service
// tools (at https://console.cloud.google.com/talent-solution/overview).
// Learn
// more (at https://cloud.google.com/talent-solution/docs/management-tools)
// about self service tools.
func (c *EventClient) CreateClientEvent(ctx context.Context, req *talentpb.CreateClientEventRequest, opts ...gax.CallOption) (*talentpb.ClientEvent, error) {
	return c.internalClient.CreateClientEvent(ctx, req, opts...)
}

// GetOperation is a utility method from google.longrunning.Operations.
func (c *EventClient) GetOperation(ctx context.Context, req *longrunningpb.GetOperationRequest, opts ...gax.CallOption) (*longrunningpb.Operation, error) {
	return c.internalClient.GetOperation(ctx, req, opts...)
}

// eventGRPCClient is a client for interacting with Cloud Talent Solution API over gRPC transport.
//
// Methods, except Close, may be called concurrently. However, fields must not be modified concurrently with method calls.
type eventGRPCClient struct {
	// Connection pool of gRPC connections to the service.
	connPool gtransport.ConnPool

	// Points back to the CallOptions field of the containing EventClient
	CallOptions **EventCallOptions

	// The gRPC API client.
	eventClient talentpb.EventServiceClient

	operationsClient longrunningpb.OperationsClient

	// The x-goog-* metadata to be sent with each request.
	xGoogHeaders []string

	logger *slog.Logger
}

// NewEventClient creates a new event service client based on gRPC.
// The returned client must be Closed when it is done being used to clean up its underlying connections.
//
// A service handles client event report.
func NewEventClient(ctx context.Context, opts ...option.ClientOption) (*EventClient, error) {
	clientOpts := defaultEventGRPCClientOptions()
	if newEventClientHook != nil {
		hookOpts, err := newEventClientHook(ctx, clientHookParams{})
		if err != nil {
			return nil, err
		}
		clientOpts = append(clientOpts, hookOpts...)
	}

	connPool, err := gtransport.DialPool(ctx, append(clientOpts, opts...)...)
	if err != nil {
		return nil, err
	}
	client := EventClient{CallOptions: defaultEventCallOptions()}

	c := &eventGRPCClient{
		connPool:         connPool,
		eventClient:      talentpb.NewEventServiceClient(connPool),
		CallOptions:      &client.CallOptions,
		logger:           internaloption.GetLogger(opts),
		operationsClient: longrunningpb.NewOperationsClient(connPool),
	}
	c.setGoogleClientInfo()

	client.internalClient = c

	return &client, nil
}

// Connection returns a connection to the API service.
//
// Deprecated: Connections are now pooled so this method does not always
// return the same resource.
func (c *eventGRPCClient) Connection() *grpc.ClientConn {
	return c.connPool.Conn()
}

// setGoogleClientInfo sets the name and version of the application in
// the `x-goog-api-client` header passed on each request. Intended for
// use by Google-written clients.
func (c *eventGRPCClient) setGoogleClientInfo(keyval ...string) {
	kv := append([]string{"gl-go", gax.GoVersion}, keyval...)
	kv = append(kv, "gapic", getVersionClient(), "gax", gax.Version, "grpc", grpc.Version, "pb", protoVersion)
	c.xGoogHeaders = []string{
		"x-goog-api-client", gax.XGoogHeader(kv...),
	}
}

// Close closes the connection to the API service. The user should invoke this when
// the client is no longer required.
func (c *eventGRPCClient) Close() error {
	return c.connPool.Close()
}

// Methods, except Close, may be called concurrently. However, fields must not be modified concurrently with method calls.
type eventRESTClient struct {
	// The http endpoint to connect to.
	endpoint string

	// The http client.
	httpClient *http.Client

	// The x-goog-* headers to be sent with each request.
	xGoogHeaders []string

	// Points back to the CallOptions field of the containing EventClient
	CallOptions **EventCallOptions

	logger *slog.Logger
}

// NewEventRESTClient creates a new event service rest client.
//
// A service handles client event report.
func NewEventRESTClient(ctx context.Context, opts ...option.ClientOption) (*EventClient, error) {
	clientOpts := append(defaultEventRESTClientOptions(), opts...)
	httpClient, endpoint, err := httptransport.NewClient(ctx, clientOpts...)
	if err != nil {
		return nil, err
	}

	callOpts := defaultEventRESTCallOptions()
	c := &eventRESTClient{
		endpoint:    endpoint,
		httpClient:  httpClient,
		CallOptions: &callOpts,
		logger:      internaloption.GetLogger(opts),
	}
	c.setGoogleClientInfo()

	return &EventClient{internalClient: c, CallOptions: callOpts}, nil
}

func defaultEventRESTClientOptions() []option.ClientOption {
	return []option.ClientOption{
		internaloption.WithDefaultEndpoint("https://jobs.googleapis.com"),
		internaloption.WithDefaultEndpointTemplate("https://jobs.UNIVERSE_DOMAIN"),
		internaloption.WithDefaultMTLSEndpoint("https://jobs.mtls.googleapis.com"),
		internaloption.WithDefaultUniverseDomain("googleapis.com"),
		internaloption.WithDefaultAudience("https://jobs.googleapis.com/"),
		internaloption.WithDefaultScopes(DefaultAuthScopes()...),
		internaloption.EnableNewAuthLibrary(),
	}
}

// setGoogleClientInfo sets the name and version of the application in
// the `x-goog-api-client` header passed on each request. Intended for
// use by Google-written clients.
func (c *eventRESTClient) setGoogleClientInfo(keyval ...string) {
	kv := append([]string{"gl-go", gax.GoVersion}, keyval...)
	kv = append(kv, "gapic", getVersionClient(), "gax", gax.Version, "rest", "UNKNOWN", "pb", protoVersion)
	c.xGoogHeaders = []string{
		"x-goog-api-client", gax.XGoogHeader(kv...),
	}
}

// Close closes the connection to the API service. The user should invoke this when
// the client is no longer required.
func (c *eventRESTClient) Close() error {
	// Replace httpClient with nil to force cleanup.
	c.httpClient = nil
	return nil
}

// Connection returns a connection to the API service.
//
// Deprecated: This method always returns nil.
func (c *eventRESTClient) Connection() *grpc.ClientConn {
	return nil
}
func (c *eventGRPCClient) CreateClientEvent(ctx context.Context, req *talentpb.CreateClientEventRequest, opts ...gax.CallOption) (*talentpb.ClientEvent, error) {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "parent", url.QueryEscape(req.GetParent()))}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).CreateClientEvent[0:len((*c.CallOptions).CreateClientEvent):len((*c.CallOptions).CreateClientEvent)], opts...)
	var resp *talentpb.ClientEvent
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = executeRPC(ctx, c.eventClient.CreateClientEvent, req, settings.GRPC, c.logger, "CreateClientEvent")
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *eventGRPCClient) GetOperation(ctx context.Context, req *longrunningpb.GetOperationRequest, opts ...gax.CallOption) (*longrunningpb.Operation, error) {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "name", url.QueryEscape(req.GetName()))}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).GetOperation[0:len((*c.CallOptions).GetOperation):len((*c.CallOptions).GetOperation)], opts...)
	var resp *longrunningpb.Operation
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = executeRPC(ctx, c.operationsClient.GetOperation, req, settings.GRPC, c.logger, "GetOperation")
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateClientEvent report events issued when end user interacts with customer’s application
// that uses Cloud Talent Solution. You may inspect the created events in
// self service
// tools (at https://console.cloud.google.com/talent-solution/overview).
// Learn
// more (at https://cloud.google.com/talent-solution/docs/management-tools)
// about self service tools.
func (c *eventRESTClient) CreateClientEvent(ctx context.Context, req *talentpb.CreateClientEventRequest, opts ...gax.CallOption) (*talentpb.ClientEvent, error) {
	m := protojson.MarshalOptions{AllowPartial: true, UseEnumNumbers: true}
	jsonReq, err := m.Marshal(req)
	if err != nil {
		return nil, err
	}

	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("/v4beta1/%v/clientEvents", req.GetParent())

	params := url.Values{}
	params.Add("$alt", "json;enum-encoding=int")

	baseUrl.RawQuery = params.Encode()

	// Build HTTP headers from client and context metadata.
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "parent", url.QueryEscape(req.GetParent()))}

	hds = append(c.xGoogHeaders, hds...)
	hds = append(hds, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).CreateClientEvent[0:len((*c.CallOptions).CreateClientEvent):len((*c.CallOptions).CreateClientEvent)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &talentpb.ClientEvent{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("POST", baseUrl.String(), bytes.NewReader(jsonReq))
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		buf, err := executeHTTPRequest(ctx, c.httpClient, httpReq, c.logger, jsonReq, "CreateClientEvent")
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}

// GetOperation is a utility method from google.longrunning.Operations.
func (c *eventRESTClient) GetOperation(ctx context.Context, req *longrunningpb.GetOperationRequest, opts ...gax.CallOption) (*longrunningpb.Operation, error) {
	baseUrl, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	baseUrl.Path += fmt.Sprintf("/v4beta1/%v", req.GetName())

	params := url.Values{}
	params.Add("$alt", "json;enum-encoding=int")

	baseUrl.RawQuery = params.Encode()

	// Build HTTP headers from client and context metadata.
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "name", url.QueryEscape(req.GetName()))}

	hds = append(c.xGoogHeaders, hds...)
	hds = append(hds, "Content-Type", "application/json")
	headers := gax.BuildHeaders(ctx, hds...)
	opts = append((*c.CallOptions).GetOperation[0:len((*c.CallOptions).GetOperation):len((*c.CallOptions).GetOperation)], opts...)
	unm := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	resp := &longrunningpb.Operation{}
	e := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		if settings.Path != "" {
			baseUrl.Path = settings.Path
		}
		httpReq, err := http.NewRequest("GET", baseUrl.String(), nil)
		if err != nil {
			return err
		}
		httpReq = httpReq.WithContext(ctx)
		httpReq.Header = headers

		buf, err := executeHTTPRequest(ctx, c.httpClient, httpReq, c.logger, nil, "GetOperation")
		if err != nil {
			return err
		}

		if err := unm.Unmarshal(buf, resp); err != nil {
			return err
		}

		return nil
	}, opts...)
	if e != nil {
		return nil, e
	}
	return resp, nil
}
