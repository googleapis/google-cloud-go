// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package metadata provides access to metadata server adoported for needs
// of the resource detection algorithm. The package partially duplicates
// implementation of "cloud.google.com/go/compute/metadata" package.
//
// This package is a wrapper around the GCE metadata service,
// as documented at https://developers.google.com/compute/docs/metadata.
package metadata // import "cloud.google.com/go/logging/metadata"

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// metadataIP is the documented metadata server IP address.
	metadataIP = "169.254.169.254"

	// metadataHostEnv is the environment variable specifying the
	// GCE metadata hostname.  If empty, the default value of
	// metadataIP ("169.254.169.254") is used instead.
	// This variable name should be used when emulating the
	// metadata server. Note that it is used by
	// "cloud.google.com/go/compute/metadata" package.
	metadataHostEnv = "GCE_METADATA_HOST"

	userAgent = "gcloud-golang/0.1"

	instancePathPrefix = "instance/attributes/"
	projectPathPrefix  = "project/attributes/"
)

// NotDefinedError is returned when requested metadata is not defined.
type NotDefinedError string

func (suffix NotDefinedError) Error() string {
	return fmt.Sprintf("metadata: attribute %q not defined", string(suffix))
}

// Error contains an error response from the server.
type Error struct {
	// Code is the HTTP response status code.
	Code int
	// Message is the server response message.
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("metadata: received %d `%s`", e.Code, e.Message)
}

// client provides access to a predefined set of metadata attributes.
type client struct {
	hc *http.Client
}

var defaultClient = &client{hc: newHTTPClient()}
var host = metadataHost()

func metadataHost() string {
	// Using a fixed IP makes it very difficult to spoof the metadata service in
	// a container, which is an important use-case for local testing of cloud
	// deployments. To enable spoofing of the metadata service, the environment
	// variable GCE_METADATA_HOST is first inspected to decide where metadata
	// requests shall go.
	host := os.Getenv(metadataHostEnv)
	if host == "" {
		// Using 169.254.169.254 instead of "metadata" here because Go binaries built with the "netgo" tag and without cgo won't
		// know the search suffix for "metadata" is ".google.internal", and this IP address is documented as being stable anyway.
		host = metadataIP
	}
	return host
}

// newDefaultHTTPClient creates a client to retrieve metadata with minimal timeout settings.
func newHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   500 * time.Millisecond,
				KeepAlive: 30 * time.Second,
			}).Dial,
		},
	}
}

// getETag returns a value from the metadata service as well as the associated ETag.
// This func is otherwise equivalent to Get.
func (c *client) getETag(suffix string) (value, etag string, err error) {
	suffix = strings.TrimLeft(suffix, "/")
	u := "http://" + host + "/computeMetadata/v1/" + suffix
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	req.Header.Set("User-Agent", userAgent)
	res, reqErr := c.hc.Do(req)
	if reqErr != nil {
		return "", "", reqErr
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return "", "", NotDefinedError(suffix)
	}
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", "", err
	}
	if res.StatusCode != 200 {
		return "", "", &Error{Code: res.StatusCode, Message: string(all)}
	}
	return string(all), res.Header.Get("Etag"), nil
}

// Get returns a value from the metadata server.
// The suffix is appended to "http://${GCE_METADATA_HOST}/computeMetadata/v1/".
//
// If the GCE_METADATA_HOST environment variable is not defined, a default of
// 169.254.169.254 will be used instead.
//
// If the requested metadata is not defined, the returned error will
// be of type NotDefinedError.
func (c *client) get(suffix string) (string, error) {
	val, _, err := c.getETag(suffix)
	return val, err
}

// IsMetadataActive returns activity status of the local metadata server.
// true is returned if the metadata server active or false otherwise.
func IsMetadataActive() bool {
	_, err := defaultClient.get("")
	return err == nil
}

// InstanceAttributeValue returns the value of the provided VM
// instance attribute.
//
// If the requested attribute is not defined, the returned error will
// be of type NotDefinedError.
//
// InstanceAttributeValue may return ("", nil) if the attribute was
// defined to be the empty string.
func InstanceAttributeValue(attr string) (string, error) {
	return defaultClient.get(instancePathPrefix + attr)
}

// ProjectAttributeValue returns the value of the provided
// project attribute.
//
// If the requested attribute is not defined, the returned error will
// be of type NotDefinedError.
//
// ProjectAttributeValue may return ("", nil) if the attribute was
// defined to be the empty string.
func ProjectAttributeValue(attr string) (string, error) {
	return defaultClient.get(projectPathPrefix + attr)
}

func pathLastValue(val string, err error) (string, error) {
	if err == nil {
		return val[strings.LastIndex(val, "/")+1:], nil
	}
	return val, err
}

func ProjectID() (string, error) {
	return defaultClient.get("project/project-id")
}

func InstanceID() (string, error) {
	return defaultClient.get("instance/id")
}

func InstanceRegion() (string, error) {
	return pathLastValue(defaultClient.get("instance/region"))
}

func InstanceZone() (string, error) {
	return pathLastValue(defaultClient.get("instance/zone"))
}

func InstancePreempted() (string, error) {
	return pathLastValue(defaultClient.get("instance/preempted"))
}

func InstanceCPUPlatform() (string, error) {
	return pathLastValue(defaultClient.get("instance/cpu-platform"))
}

