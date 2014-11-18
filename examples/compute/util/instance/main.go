// Copyright 2014 Google Inc. All Rights Reserved.
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

// instance is a sample application using  the computeutil package.
package main

import (
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/golang/oauth2"
	"github.com/golang/oauth2/google"
	"google.golang.org/cloud"
	computeutil "google.golang.org/cloud/compute/util"
)

var (
	jsonFile          = flag.String("j", "", "A path to your JSON key file for your service account downloaded from Google Developer Console, not needed if you run it on Compute Engine instances.")
	projID            = flag.String("p", "", "The ID of your Google Cloud project.")
	name              = flag.String("n", "gcloud-computeutil-instance", "The name of the instance to create.")
	image             = flag.String("i", "projects/google-containers/global/images/container-vm-v20140929", "The image to use for the instance.")
	zone              = flag.String("z", "us-central1-f", "The zone for the instance.")
	machineType       = flag.String("m", "f1-micro", "The machine type for the instance.")
	startupScriptPath = flag.String("s", "", "The path to the startup script for the instance")
)

func main() {
	flag.Parse()
	if *jsonFile == "" || *projID == "" {
		flag.PrintDefaults()
		log.Fatalf("Please specify JSON file and Project ID.")
	}
	var metadata map[string]string
	if *startupScriptPath != "" {
		startupScript, err := ioutil.ReadFile(*startupScriptPath)
		if err != nil {
			log.Fatalf("Error reading startup script %q: %v", startupScriptPath, err)
		}
		metadata = map[string]string{
			"startup-script": string(startupScript),
		}
		log.Println(metadata)
	}
	flow, err := oauth2.New(
		google.ServiceAccountJSONKey(*jsonFile),
		oauth2.Scope(computeutil.ScopeCompute),
	)
	if err != nil {
		log.Fatalf("oauth2 flow creation failed, %v", err)
	}
	client := &http.Client{Transport: flow.NewTransport()}
	ctx := cloud.WithZone(cloud.NewContext(*projID, client), *zone)
	var instance *computeutil.Instance
	instance, err = computeutil.GetInstance(ctx, *name)
	if err != nil { // not found
		instance, err = computeutil.NewInstance(ctx, &computeutil.Instance{
			Name:        *name,
			Image:       *image,
			MachineType: *machineType,
			Metadata:    metadata,
		})
		if err != nil {
			log.Fatalf("failed to create instance %q: %v", *name, err)
		}
	}
	log.Printf("instance %q ready: %#v", *name, instance)
	io.Copy(os.Stdout, instance.SerialPortOutput(ctx))
}
