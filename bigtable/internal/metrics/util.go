/*
Copyright 2024 Google LLC

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

package internal

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	defaultCluster = "<unspecified>"
	defaultZone    = "global"
	defaultTable   = "<unspecified>"
	PeerInfoMDKey  = "bigtable-peer-info"
)

// get GFE latency in ms from response metadata
func ExtractServerLatency(headerMD metadata.MD, trailerMD metadata.MD) (float64, error) {
	serverTimingStr := ""

	// Check whether server latency available in response header metadata
	if headerMD != nil {
		headerMDValues := headerMD.Get(ServerTimingMDKey)
		if len(headerMDValues) != 0 {
			serverTimingStr = headerMDValues[0]
		}
	}

	if len(serverTimingStr) == 0 {
		// Check whether server latency available in response trailer metadata
		if trailerMD != nil {
			trailerMDValues := trailerMD.Get(ServerTimingMDKey)
			if len(trailerMDValues) != 0 {
				serverTimingStr = trailerMDValues[0]
			}
		}
	}

	serverLatencyMillisStr := strings.TrimPrefix(serverTimingStr, serverTimingValPrefix)
	serverLatencyMillis, err := strconv.ParseFloat(strings.TrimSpace(serverLatencyMillisStr), 64)
	if !strings.HasPrefix(serverTimingStr, serverTimingValPrefix) || err != nil {
		return serverLatencyMillis, err
	}

	return serverLatencyMillis, nil
}

// Obtain cluster and zone from response metadata
// Check both headers and trailers because in different environments the metadata could
// be returned in headers or trailers
func ExtractLocation(headerMD metadata.MD, trailerMD metadata.MD) (string, string, error) {
	var locationMetadata []string

	// Check whether location metadata available in response header metadata
	if headerMD != nil {
		locationMetadata = headerMD.Get(LocationMDKey)
	}

	if locationMetadata == nil {
		// Check whether location metadata available in response trailer metadata
		// if none found in response header metadata
		if trailerMD != nil {
			locationMetadata = trailerMD.Get(LocationMDKey)
		}
	}

	if len(locationMetadata) < 1 {
		return defaultCluster, defaultZone, errors.New("failed to get location metadata")
	}

	// Unmarshal binary location metadata
	responseParams := &btpb.ResponseParams{}
	err := proto.Unmarshal([]byte(locationMetadata[0]), responseParams)
	if err != nil {
		return defaultCluster, defaultZone, err
	}

	return responseParams.GetClusterId(), responseParams.GetZoneId(), nil
}

// ExtractPeerInfo decodes the bigtable-peer-info sideband metadata (populated
// by the server when the PeerInfo feature flag is negotiated on) and returns
// the parsed PeerInfo. Returns (nil, nil) when the header is absent — the
// caller records the attempt without transport labels in that case. Server
// emits URL-safe base64; any '=' padding is stripped so a single
// RawURLEncoding decoder handles both padded and unpadded shapes (matches
// java-bigtable's Base64.getUrlDecoder()).
func ExtractPeerInfo(headerMD metadata.MD, trailerMD metadata.MD) (*btpb.PeerInfo, error) {
	var peerInfoData []string
	if headerMD != nil {
		peerInfoData = headerMD.Get(PeerInfoMDKey)
	}
	if len(peerInfoData) == 0 && trailerMD != nil {
		peerInfoData = trailerMD.Get(PeerInfoMDKey)
	}
	if len(peerInfoData) == 0 || peerInfoData[0] == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(peerInfoData[0], "="))
	if err != nil {
		return nil, fmt.Errorf("failed to decode %s from header: %w", PeerInfoMDKey, err)
	}
	var peerInfo btpb.PeerInfo
	if err := proto.Unmarshal(decoded, &peerInfo); err != nil {
		return nil, fmt.Errorf("failed to parse %s protobuf: %w", PeerInfoMDKey, err)
	}
	return &peerInfo, nil
}

func ConvertToMs(d time.Duration) float64 {
	return float64(d.Nanoseconds()) / 1000000
}

// GrpcCodeOf extracts the gRPC status code from an error. Maps a nil
// error to codes.OK, a status.Error to its embedded code, a context
// deadline/canceled error to its canonical code, and anything else to
// codes.Unknown. Shared helper so tracer/session paths that only need
// the code (not the wrapped error) don't reimplement the walk.
func GrpcCodeOf(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	if s, ok := status.FromError(err); ok {
		return s.Code()
	}
	if s := status.FromContextError(err); s.Code() != codes.Unknown {
		return s.Code()
	}
	return codes.Unknown
}
