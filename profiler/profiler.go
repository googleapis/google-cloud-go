// Copyright 2017 Google Inc. All Rights Reserved.
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

// Package profiler is a client for the Google Cloud Profiler service.
//
// This package is still experimental and subject to change.
//
// Calling Start will start a goroutine to collect profiles and
// upload to Cloud Profiler server, at the rhythm specified by
// the server.
//
// The caller should provide the target string in the config so Cloud
// Profiler knows how to group the profile data. Otherwise the target
// string is set to "unknown".
//
// Optionally DebugLogging can be set in the config to enable detailed
// logging from profiler.
//
// Start should only be called once. The first call will start
// the profiling goroutine. Any additional calls will be ignored.
package profiler

import (
	"bytes"
	"errors"
	"log"
	"runtime/pprof"
	"sync"
	"time"

	gcemd "cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/internal/version"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	gax "github.com/googleapis/gax-go"
	"golang.org/x/net/context"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	pb "google.golang.org/genproto/googleapis/devtools/cloudprofiler/v2"
	edpb "google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcmd "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	config    Config
	startOnce sync.Once
	// getProjectID, getInstanceName, getZone, startCPUProfile, stopCPUProfile,
	// writeHeapProfile and sleep are overrideable for testing.
	getProjectID     = gcemd.ProjectID
	getInstanceName  = gcemd.InstanceName
	getZone          = gcemd.Zone
	startCPUProfile  = pprof.StartCPUProfile
	stopCPUProfile   = pprof.StopCPUProfile
	writeHeapProfile = pprof.WriteHeapProfile
	sleep            = gax.Sleep
)

const (
	apiAddress       = "cloudprofiler.googleapis.com:443"
	xGoogAPIMetadata = "x-goog-api-client"
	zoneNameLabel    = "zone"
	instanceLabel    = "instance"
	scope            = "https://www.googleapis.com/auth/monitoring.write"

	initialBackoff = time.Second
	// Ensure the agent will recover within 1 hour.
	maxBackoff        = time.Hour
	backoffMultiplier = 1.3 // Backoff envelope increases by this factor on each retry.
	retryInfoMetadata = "google.rpc.retryinfo-bin"
)

// Config is the profiler configuration.
type Config struct {
	// Target groups related deployments together, defaults to "unknown".
	Target string

	// DebugLogging enables detailed debug logging from profiler.
	DebugLogging bool

	// ProjectID is the Cloud Console project ID to use instead of
	// the one read from the VM metadata server.
	//
	// Set this if you are running the agent in your local environment
	// or anywhere else outside of Google Cloud Platform.
	ProjectID string

	// InstanceName is the name of the VM instance to use instead of
	// the one read from the VM metadata server.
	//
	// Set this if you are running the agent in your local environment
	// or anywhere else outside of Google Cloud Platform.
	InstanceName string

	// ZoneName is the name of the zone to use instead of
	// the one read from the VM metadata server.
	//
	// Set this if you are running the agent in your local environment
	// or anywhere else outside of Google Cloud Platform.
	ZoneName string

	// APIAddr is the HTTP endpoint to use to connect to the profiler
	// agent API. Defaults to the production environment, overridable
	// for testing.
	APIAddr string
}

// startError represents the error occured during the
// initializating and starting of the agent.
var startError error

// Start starts a goroutine to collect and upload profiles.
// See package level documentation for details.
func Start(cfg Config, options ...option.ClientOption) error {
	startOnce.Do(func() {
		startError = start(cfg, options...)
	})
	return startError
}

func start(cfg Config, options ...option.ClientOption) error {
	initializeConfig(cfg)

	ctx := context.Background()

	opts := []option.ClientOption{
		option.WithEndpoint(config.APIAddr),
		option.WithScopes(scope),
	}
	opts = append(opts, options...)

	conn, err := gtransport.Dial(ctx, opts...)
	if err != nil {
		debugLog("failed to dial GRPC: %v", err)
		return err
	}

	d, err := initializeDeployment()
	if err != nil {
		debugLog("failed to initialize deployment: %v", err)
		return err
	}

	l := initializeProfileLabels()

	a, ctx := initializeResources(ctx, conn, d, l)
	go pollProfilerService(ctx, a)
	return nil
}

func debugLog(format string, e ...interface{}) {
	if config.DebugLogging {
		log.Printf(format, e...)
	}
}

// agent polls Cloud Profiler server for instructions on behalf of
// a task, and collects and uploads profiles as requested.
type agent struct {
	client        *client
	deployment    *pb.Deployment
	profileLabels map[string]string
}

// abortedBackoffDuration retrieves the retry duration from gRPC trailing
// metadata, which is set by Cloud Profiler server.
func abortedBackoffDuration(md grpcmd.MD) (time.Duration, error) {
	elem := md[retryInfoMetadata]
	if len(elem) <= 0 {
		return 0, errors.New("no retry info")
	}

	var retryInfo edpb.RetryInfo
	if err := proto.Unmarshal([]byte(elem[0]), &retryInfo); err != nil {
		return 0, err
	} else if time, err := ptypes.Duration(retryInfo.RetryDelay); err != nil {
		return 0, err
	} else {
		if time < 0 {
			return 0, errors.New("negative retry duration")
		}
		return time, nil
	}
}

type retryer struct {
	backoff gax.Backoff
	md      grpcmd.MD
}

func (r *retryer) Retry(err error) (time.Duration, bool) {
	st, _ := status.FromError(err)
	if st != nil && st.Code() == codes.Aborted {
		dur, err := abortedBackoffDuration(r.md)
		if err == nil {
			return dur, true
		}
		debugLog("failed to get backoff duration: %v", err)
	}
	return r.backoff.Pause(), true
}

// createProfile talks to Cloud Profiler server to create profile. In
// case of error, the goroutine will sleep and retry. Sleep duration may
// be specified by the server. Otherwise it will be an exponentially
// increasing value, bounded by maxBackoff.
func (a *agent) createProfile(ctx context.Context) *pb.Profile {
	req := pb.CreateProfileRequest{
		Deployment:  a.deployment,
		ProfileType: []pb.ProfileType{pb.ProfileType_CPU, pb.ProfileType_HEAP},
	}

	var p *pb.Profile
	md := grpcmd.New(map[string]string{})

	gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		p, err = a.client.client.CreateProfile(ctx, &req, grpc.Trailer(&md))
		return err
	}, gax.WithRetry(func() gax.Retryer {
		return &retryer{
			backoff: gax.Backoff{
				Initial:    initialBackoff,
				Max:        maxBackoff,
				Multiplier: backoffMultiplier,
			},
			md: md,
		}
	}))

	debugLog("successfully created profile %v", p.GetProfileType())
	return p
}

func (a *agent) profileAndUpload(ctx context.Context, p *pb.Profile) {
	var prof bytes.Buffer
	pt := p.GetProfileType()

	switch pt {
	case pb.ProfileType_CPU:
		duration, err := ptypes.Duration(p.Duration)
		if err != nil {
			debugLog("failed to get profile duration: %v", err)
			return
		}
		if err := startCPUProfile(&prof); err != nil {
			debugLog("failed to start CPU profile: %v", err)
			return
		}
		sleep(ctx, duration)
		stopCPUProfile()
	case pb.ProfileType_HEAP:
		if err := writeHeapProfile(&prof); err != nil {
			debugLog("failed to write heap profile: %v", err)
			return
		}
	default:
		debugLog("unexpected profile type: %v", pt)
		return
	}

	// Starting Go 1.9 the profiles are symbolized by runtime/pprof.
	// TODO(jianqiaoli): Remove the symbolization code when we decide to
	// stop supporting Go 1.8.
	if !shouldAssumeSymbolized {
		if err := parseAndSymbolize(&prof); err != nil {
			debugLog("failed to symbolize profile: %v", err)
		}
	}

	p.ProfileBytes = prof.Bytes()
	p.Labels = a.profileLabels
	req := pb.UpdateProfileRequest{Profile: p}

	// Upload profile, discard profile in case of error.
	debugLog("start uploading profile")
	if _, err := a.client.client.UpdateProfile(ctx, &req); err != nil {
		debugLog("failed to upload profile: %v", err)
	}
}

// client is a client for interacting with Cloud Profiler API.
type client struct {
	// gRPC API client.
	client pb.ProfilerServiceClient

	// Metadata for google API to be sent with each request.
	xGoogHeader []string
}

// setXGoogHeader sets the name and version of the application in
// the `x-goog-api-client` header passed on each request. Intended for
// use by Google-written clients.
func (c *client) setXGoogHeader(keyval ...string) {
	kv := append([]string{"gl-go", version.Go(), "gccl", version.Repo}, keyval...)
	kv = append(kv, "gax", gax.Version, "grpc", grpc.Version)
	c.xGoogHeader = []string{gax.XGoogHeader(kv...)}
}

func (c *client) insertMetadata(ctx context.Context) context.Context {
	md, _ := grpcmd.FromOutgoingContext(ctx)
	md = md.Copy()
	md[xGoogAPIMetadata] = c.xGoogHeader
	return grpcmd.NewOutgoingContext(ctx, md)
}

func initializeDeployment() (*pb.Deployment, error) {
	var err error

	projectID := config.ProjectID
	if projectID == "" {
		projectID, err = getProjectID()
		if err != nil {
			return nil, err
		}
	}

	zone := config.ZoneName
	if zone == "" {
		zone, err = getZone()
		if err != nil {
			return nil, err
		}
	}

	return &pb.Deployment{
		ProjectId: projectID,
		Target:    config.Target,
		Labels: map[string]string{
			zoneNameLabel: zone,
		},
	}, nil
}

func initializeProfileLabels() map[string]string {
	instance := config.InstanceName
	if instance == "" {
		var err error
		if instance, err = getInstanceName(); err != nil {
			instance = "unknown"
			debugLog("failed to get instance name: %v", err)
		}
	}

	return map[string]string{instanceLabel: instance}
}

func initializeResources(ctx context.Context, conn *grpc.ClientConn, d *pb.Deployment, l map[string]string) (*agent, context.Context) {
	c := &client{
		client: pb.NewProfilerServiceClient(conn),
	}
	c.setXGoogHeader()

	ctx = c.insertMetadata(ctx)
	return &agent{
		client:        c,
		deployment:    d,
		profileLabels: l,
	}, ctx
}

func initializeConfig(cfg Config) {
	config = cfg

	if config.Target == "" {
		config.Target = "unknown"
	}
	if config.APIAddr == "" {
		config.APIAddr = apiAddress
	}
}

// pollProfilerService starts an endless loop to poll Cloud Profiler
// server for instructions, and collects and uploads profiles as
// requested.
func pollProfilerService(ctx context.Context, a *agent) {
	for {
		p := a.createProfile(ctx)
		a.profileAndUpload(ctx, p)
	}
}
