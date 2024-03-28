/*
Copyright 2021 Google LLC

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

/*
This file provides interceptors to hook into grpc streaming calls
and inject latency and errors into cbtemulator for testing
*/
package bttest

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	validStreamMethodSuffixes = []string{"ReadRows", "MutateRows"}
)

/*
   Creates interceptors inject latency or errors into cbtemulator
*/
type EmulatorInterceptor struct {
	LatencyTargets       latencyTargets
	GrpcErrorCodeTargets grpcErrorCodeTargets
}

func (esib *EmulatorInterceptor) StreamInterceptor() grpc.ServerOption {
	log.Println("Building Stream Server Interceptor:")
	if len(esib.LatencyTargets) > 0 {
		log.Printf(" - Latency Targets: %s\n", esib.LatencyTargets.String())
	}
	if len(esib.GrpcErrorCodeTargets) > 0 {
		log.Printf(" - Error Targets: %s\n", esib.GrpcErrorCodeTargets.String())
	}

	return grpc.StreamInterceptor(esib.interceptorFunc())
}

func (esib *EmulatorInterceptor) interceptorFunc() grpc.StreamServerInterceptor {
	return func(srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler) error {

		startTime := time.Now()

		// Latency injection
		pVal := rand.Float64() * 100
		for _, lt := range esib.LatencyTargets {
			if strings.HasSuffix(info.FullMethod, lt.methodSuffix) && pVal >= lt.percentile {
				addedLatency := time.Until(startTime.Add(lt.expectedDuration))
				if addedLatency > time.Duration(0) {
					time.Sleep(addedLatency)
				}
			}
		}

		// Return any actual errors from handler
		err := handler(srv, ss)
		if err != nil {
			return err
		}

		// If no actual errors, run error injection
		eRand := rand.Float64() * 100
		for _, gt := range esib.GrpcErrorCodeTargets {
			if strings.HasSuffix(info.FullMethod, gt.methodSuffix) && eRand <= gt.stackedErrorRate {
				return status.Error(gt.grpcErrorCode, "Injected Emulator Error")
			}
		}

		return nil
	}
}

/*
   latencyTargets define how long it will take for a method
   to return at different percentiles. e.g. ReadRows:p50:100ms
*/

type latencyTargets []latencyTarget

// Set() and String() used by Flags lib
func (lts *latencyTargets) Set(s string) error {
	lt, err := newLatencyTarget(s)
	if err != nil {
		return err
	}
	*lts = append(*lts, *lt)
	return nil
}

func (lts *latencyTargets) String() string {
	var s []string
	for _, v := range *lts {
		s = append(s, v.String())
	}
	return strings.Join(s, ", ")
}

type latencyTarget struct {
	methodSuffix     string
	percentile       float64
	expectedDuration time.Duration
}

// Create new latencyTarget from string like "ReadRows:p50:100ms"
func newLatencyTarget(s string) (*latencyTarget, error) {
	var lt latencyTarget
	var err error

	pieces := strings.Split(s, ":")
	if len(pieces) != 3 {
		return nil, fmt.Errorf("Expected Latency Target in form of: <method>:<percentile>:<duration>")
	}
	err = lt.setMethodSuffix(pieces[0])
	if err != nil {
		return nil, err
	}
	err = lt.setPercentile(pieces[1])
	if err != nil {
		return nil, err
	}
	if err = lt.setExpectedDuration(pieces[2]); err != nil {
		return nil, err
	}

	return &lt, nil
}

func (lt *latencyTarget) setMethodSuffix(s string) error {
	if isValidStreamMethodSuffix(s) {
		lt.methodSuffix = s
		return nil
	}
	return fmt.Errorf("Invalid method \"%s\". Expected one of: %s", s, validStreamMethodSuffixes)
}

func isValidStreamMethodSuffix(s string) bool {
	for _, v := range validStreamMethodSuffixes {
		if s == v {
			return true
		}
	}
	return false
}

func (lt *latencyTarget) setPercentile(s string) error {
	sf := strings.TrimPrefix(s, "p")
	p, err := strconv.ParseFloat(sf, 64)
	if err != nil || (p < 0 || p >= 100) {
		return fmt.Errorf("Invalid percentile \"%s\". Expected float in range [0, 100)", s)
	}
	lt.percentile = p
	return nil
}

func (lt *latencyTarget) setExpectedDuration(s string) error {
	d, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("Invalid duration \"%s\". ParseDuration error: %s", s, err)
	}
	lt.expectedDuration = d
	return nil
}

func (lt *latencyTarget) String() string {
	return fmt.Sprintf("%s:p%.2f:%s", lt.methodSuffix, lt.percentile, lt.expectedDuration)
}

/*
   grpcErrorCodeTargets define how often each method should throw a given GRPC Error Code
   e.g. "ReadRows:10%:14" will have a 10% chance to throw Error Code 14 (Unavailable) on each ReadRows request
*/

type grpcErrorCodeTargets []grpcErrorCodeTarget

/*
   Error targets are stacked so that arguments like:
     - [ReadRows:50%:12, ReadRows:50%:14, MutateRows:10%:9]
   transform to:
     - [ReadRows:50%:12, ReadRows:100%:14, MutateRows:10%:9]
   and when we check against randint(100):
    - ReadRows will always throw an error (50% probability for each error)
    - MutateRows throw an error 10% of the time
*/
func (ets *grpcErrorCodeTargets) Set(s string) error {
	et, err := newErrorTarget(s)
	if err != nil {
		return err
	}
	totalErrorRate := ets.getTotalErrorRateForMethodSuffix(et.methodSuffix) + et.errorRate
	if totalErrorRate > 100 {
		return fmt.Errorf("Total error rate for method \"%s\" should not be >100", et.methodSuffix)
	}
	et.stackedErrorRate = totalErrorRate
	*ets = append(*ets, *et)
	return nil
}

func (ets *grpcErrorCodeTargets) String() string {
	var s []string
	for _, v := range *ets {
		s = append(s, v.String())
	}
	return strings.Join(s, ", ")
}

func (ets *grpcErrorCodeTargets) getTotalErrorRateForMethodSuffix(methodSuffix string) float64 {
	totalErrorRate := float64(0)
	for _, gt := range *ets {
		if gt.methodSuffix == methodSuffix {
			totalErrorRate = totalErrorRate + gt.errorRate
		}
	}
	return totalErrorRate
}

type grpcErrorCodeTarget struct {
	methodSuffix     string
	errorRate        float64
	stackedErrorRate float64
	grpcErrorCode    codes.Code
}

// Create new grpc error code target from string like "MutateRows:10%:14"
func newErrorTarget(s string) (*grpcErrorCodeTarget, error) {
	var gt grpcErrorCodeTarget
	var err error

	// Split 's' and use each value to build error target
	pieces := strings.Split(s, ":")
	if len(pieces) != 3 {
		return nil, fmt.Errorf("Expected GRPC Error Target in form of: <method>:<error_rate>:<grpc_error_code>")
	}

	// Method
	err = gt.setMethodSuffix(pieces[0])
	if err != nil {
		return nil, err
	}
	// Error Rate
	err = gt.setErrorRate(pieces[1])
	if err != nil {
		return nil, err
	}
	// Error Code
	if err = gt.setGrpcErrorCode(pieces[2]); err != nil {
		return nil, err
	}

	return &gt, nil
}

func (gt *grpcErrorCodeTarget) setMethodSuffix(s string) error {
	if isValidStreamMethodSuffix(s) {
		gt.methodSuffix = s
		return nil
	}
	return fmt.Errorf("Invalid method \"%s\". Expected one of: %s", s, validStreamMethodSuffixes)
}

func (gt *grpcErrorCodeTarget) setErrorRate(s string) error {
	sf := strings.TrimSuffix(s, "%")
	e, err := strconv.ParseFloat(sf, 64)
	if err != nil || (e < 0 || e > 100) {
		return fmt.Errorf("Invalid error rate \"%s\". Expected float in range [0, 100]", s)
	}
	gt.errorRate = e
	return nil
}

func (gt *grpcErrorCodeTarget) setGrpcErrorCode(s string) error {
	var c codes.Code
	// Use UnmarshalJSON() to check against valid codes in google.golang.org/grpc/codes
	err := c.UnmarshalJSON([]byte(s))
	if err != nil {
		return fmt.Errorf("Invalid GRPC Error Code: \"%s\". %v", s, err)
	}
	gt.grpcErrorCode = c
	return nil
}

func (gt *grpcErrorCodeTarget) String() string {
	return fmt.Sprintf("%s:%.2f%%:%v", gt.methodSuffix, gt.errorRate, gt.grpcErrorCode)
}
