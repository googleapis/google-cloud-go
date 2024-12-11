package main

import (
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	// Read the file content
	storageControlGRPCFile := "../controlpb/storage_control.pb.go"
	input, err := os.ReadFile(storageControlGRPCFile)
	if err != nil {
		log.Fatalln(err)
	}
	mapping := map[string]string{
		"/google.storage.control.v2.StorageControl/DeleteBucket":              "/google.storage.v2.Storage/DeleteBucket",
		"/google.storage.control.v2.StorageControl/GetBucket":                 "/google.storage.v2.Storage/GetBucket",
		"/google.storage.control.v2.StorageControl/CreateBucket":              "/google.storage.v2.Storage/CreateBucket",
		"/google.storage.control.v2.StorageControl/ListBuckets":               "/google.storage.v2.Storage/ListBuckets",
		"/google.storage.control.v2.StorageControl/LockBucketRetentionPolicy": "/google.storage.v2.Storage/LockBucketRetentionPolicy",
		"/google.storage.control.v2.StorageControl/GetIamPolicy":              "/google.storage.v2.Storage/GetIamPolicy",
		"/google.storage.control.v2.StorageControl/SetIamPolicy":              "/google.storage.v2.Storage/SetIamPolicy",
		"/google.storage.control.v2.StorageControl/TestIamPermissions":        "/google.storage.v2.Storage/TestIamPermissions",
		"/google.storage.control.v2.StorageControl/UpdateBucket":              "/google.storage.v2.Storage/UpdateBucket",
		"/google.storage.control.v2.StorageControl/ComposeObject":             "/google.storage.v2.Storage/ComposeObject",
		"/google.storage.control.v2.StorageControl/DeleteObject":              "/google.storage.v2.Storage/DeleteObject",
		"/google.storage.control.v2.StorageControl/RestoreObject":             "/google.storage.v2.Storage/RestoreObject",
		"/google.storage.control.v2.StorageControl/GetObject":                 "/google.storage.v2.Storage/GetObject",
		"/google.storage.control.v2.StorageControl/UpdateObject":              "/google.storage.v2.Storage/UpdateObject",
		"/google.storage.control.v2.StorageControl/ListObjects":               "/google.storage.v2.Storage/ListObjects",
		"/google.storage.control.v2.StorageControl/RewriteObject":             "/google.storage.v2.Storage/RewriteObject",
		"/google.storage.control.v2.StorageControl/MoveObject":                "/google.storage.v2.Storage/MoveObject",
	}
	// Replace the strings
	output := string(input)
	for key, value := range mapping {
		output = strings.Replace(output, key, value, -1)
	}
	// Write the updated content back to the file
	err = os.WriteFile(storageControlGRPCFile+".mod", []byte(output), 0644)
	if err != nil {
		log.Fatalf("failed: %v", err)
	}

	fmt.Println("Strings replaced successfully!")
}
