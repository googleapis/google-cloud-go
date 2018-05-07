// Copyright 2018 Google Inc. All Rights Reserved.
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

// This program is called from run.sh to test the httpr binary.

package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

var mode = flag.String("mode", "", "'record' or 'replay'")

func main() {
	flag.Parse()
	if *mode != "record" && *mode != "replay" {
		log.Fatal("need '-mode record' or '-mode replay'")
	}
	if testutil.ProjID() == "" {
		log.Fatal("set GCLOUD_TESTS_GOLANG_PROJECT_ID and GCLOUD_TESTS_GOLANG_KEY")
	}
	ctx := context.Background()
	tr, err := proxyTransport()
	if err != nil {
		log.Fatal(err)
	}
	var hc *http.Client
	if *mode == "record" {
		ts := testutil.TokenSource(context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Transport: tr}),
			storage.ScopeFullControl)
		hc = &http.Client{
			Transport: &oauth2.Transport{
				Base:   tr,
				Source: ts,
			},
		}
	} else {
		hc = &http.Client{Transport: tr}
	}
	client, err := storage.NewClient(ctx, option.WithHTTPClient(hc))
	if err != nil {
		log.Fatal(err)
	}
	b := client.Bucket(testutil.ProjID())
	attrs, err := b.Attrs(ctx)
	if err != nil {
		log.Fatal(err)
	}
	client.Close()
	fmt.Printf("name:%s reqpays:%v location:%s sclass:%s",
		attrs.Name, attrs.RequesterPays, attrs.Location, attrs.StorageClass)
}

func proxyTransport() (*http.Transport, error) {
	caCert, err := getBody("http://localhost:8181/authority.cer")
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM([]byte(caCert)) {
		return nil, errors.New("bad CA Cert")
	}
	return &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{RootCAs: caCertPool},
	}, nil
}

func getBody(url string) ([]byte, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return ioutil.ReadAll(res.Body)
}
