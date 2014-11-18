package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/golang/oauth2"
	"github.com/golang/oauth2/google"
	"google.golang.org/cloud"
	"google.golang.org/cloud/compute"
)

var (
	jsonFile    = flag.String("j", "", "A path to your JSON key file for your service account downloaded from Google Developer Console, not needed if you run it on Compute Engine instances.")
	projID      = flag.String("p", "", "The ID of your Google Cloud project.")
	name        = flag.String("n", "", "The name of the instance to create.")
	image       = flag.String("i", "projects/google-containers/global/images/container-vm-v20140929", "The image to use for the instance.")
	zone        = flag.String("z", "us-central1-f", "The compute zone for the intsance.")
	machineType = flag.String("m", "f1-micro", "The compute zone for the intsance.")
)

func main() {
	flag.Parse()
	if *jsonFile == "" || *projID == "" {
		flag.PrintDefaults()
		log.Fatalf("Please specify JSON file and Project ID.")
	}
	flow, err := oauth2.New(
		google.ServiceAccountJSONKey(*jsonFile),
		oauth2.Scope(compute.ScopeCompute),
	)
	if err != nil {
		log.Fatalf("clientAndId failed, %v", err)
	}
	client := &http.Client{Transport: flow.NewTransport()}
	if *name == "" {
		*name = fmt.Sprintf("gcloud-compute-cli-%d", time.Now().Unix())
	}
	ctx := cloud.WithZone(cloud.NewContext(*projID, client), *zone)
	instance, err := compute.NewInstance(ctx, &compute.Instance{
		Name:        *name,
		Image:       *image,
		MachineType: *machineType,
	})
	if err != nil {
		log.Fatalf("failed to create instance %q: %v", *name, err)
	}
	log.Printf("instance %q created: %#v", *name, instance)
}
