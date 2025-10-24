package internal

import (
	"fmt"
	"log"

	"cloud.google.com/go/compute/metadata"
)

// IsRunningOnGCP checks that the code is running on GCP by checking the metadata server.
// Basically, reads /sys/class/dmi/id/product_name
func IsRunningOnGCP() error {
	if metadata.OnGCE() {
		log.Println("Detected running on GCE via metadata.OnGCE()")
		return nil
	}
	return fmt.Errorf("not running on GCE according to metadata.OnGCE()")
}
