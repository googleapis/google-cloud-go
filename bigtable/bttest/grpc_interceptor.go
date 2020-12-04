/*
Copyright 2020 Google LLC

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

package bttest

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
)

var (
	validMethodNames = []string{"MutateRow", "CheckAndMutate", "ReadModifyWrite", "ReadRows", "MutateRows"}
)

// StreamServerInterceptorConfig builds an interceptor to be passed into bttest.NewServer.
// Currently supports interceptors to simulate latency, and could be expanded to other use-cases
type StreamServerInterceptorConfig struct {
	latencyTargets []LatencyTarget
}

// Constructor using only latency
func NewLatencyStreamServerInterceptorConfig(latencyTargets LatencyTargets) (*StreamServerInterceptorConfig, error) {
	sic := new(StreamServerInterceptorConfig)
	sic.latencyTargets = latencyTargets
	return sic, nil
}

// Generate an Interceptor func to be passed into grpc opts
func (ssic *StreamServerInterceptorConfig) CreateInterceptor() grpc.StreamServerInterceptor {
	fmt.Printf("Creating GRPC StreamInterceptor:\n")
	fmt.Printf(" --> Latency targets: %s\n", ssic.latencyTargets)
	return func(srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler) error {

		start := time.Now()
		err := handler(srv, ss)

		percentile := rand.Int31n(100)

		// Loop through latency targets and sleep if percentile > target percentile
		for _, lt := range ssic.latencyTargets {
			if strings.HasSuffix(info.FullMethod, lt.methodSuffix) {
				if percentile >= lt.percentile {
					time.Sleep(time.Until(start.Add(lt.expectedDuration)))
				}
			}
		}

		return err
	}
}

// LatencyTargets implements flags interface
type LatencyTargets []LatencyTarget

func (lts *LatencyTargets) Set(s string) error {
	lt, err := NewLatencyTargetFromFlag(s)
	if err != nil {
		return err
	}
	*lts = append(*lts, *lt)
	return nil
}

func (lts *LatencyTargets) String() string {
	var s []string
	for _, lt := range *lts {
		s = append(s, lt.String())
	}
	return fmt.Sprintf("%q\n", s)
}

// For a specific method + percentile, define the expected duration
type LatencyTarget struct {
	methodSuffix     string
	percentile       int32
	expectedDuration time.Duration
	repr             string
}

func NewLatencyTargetFromFlag(s string) (*LatencyTarget, error) {
	lt := new(LatencyTarget)

	vals := strings.Split(s, ":")
	if len(vals) != 3 {
		return nil, fmt.Errorf("Expected Latency Target in form of: <method>:<percentile>:<duration>")
	}

	var err error
	if err = lt.setMethod(vals[0]); err != nil {
		return nil, err
	}
	if err = lt.setPercentile(vals[1]); err != nil {
		return nil, err
	}
	if err = lt.setExpectedDuration(vals[2]); err != nil {
		return nil, err
	}

	// Remember format string to make printing easier
	lt.repr = s
	return lt, nil
}

func (lt *LatencyTarget) setMethod(s string) error {
	for _, v := range validMethodNames {
		if s == v {
			lt.methodSuffix = s
			return nil
		}
	}
	return fmt.Errorf("Invalid latency method. Expected one of: [%s]", strings.Join(validMethodNames, ", "))
}

func (lt *LatencyTarget) setPercentile(s string) error {
	p := strings.TrimPrefix(s, "p")
	i, err := strconv.Atoi(p)
	if err != nil || (i < 0 || i > 99) {
		return fmt.Errorf("Invalid latency percentile: %s. Expected integer between 0 and 99", s)
	}
	lt.percentile = int32(i)
	return nil
}

func (lt *LatencyTarget) setExpectedDuration(s string) error {
	d, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("Invalid latency duration: %s. %s", s, err)
	}
	lt.expectedDuration = d
	return nil
}

func (lt LatencyTarget) String() string {
	return lt.repr
}
