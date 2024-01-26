package main

import (
	"context"
	"crypto/rand"
	"io"
	"log"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
)

const (
	projectID  = "brenna-test-320521"
	objectName = "testfile"
)

func main() {
	ctx := context.Background()
	client, err := storage.NewGRPCClient(ctx)
	if err != nil {
		log.Fatalf("storage.NewGRPCClient: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	bucketName := "golang-grpc-test-" + uuid.New().String()

	// test out creating bucket (unary RPC)
	err = client.Bucket(bucketName).Create(ctx, projectID, nil)
	if err != nil {
		log.Fatalf("creating bucket: %v", err)
	}
	defer client.Bucket(bucketName).Delete(ctx)

	// test out uploading, downloading, and deleting file from bucket
	o := client.Bucket(bucketName).Object(objectName)
	wc := o.NewWriter(ctx)
	buf := generateRandomBytes(3 * 1024 * 1024)
	if _, err := wc.Write(buf); err != nil {
		log.Fatalf("writing file: %v", err)
	}
	if err := wc.Close(); err != nil {
		log.Fatalf("closing object: %v", err)
	}

	rc, err := client.Bucket(bucketName).Object(objectName).NewReader(ctx)
	if err != nil {
		log.Fatalf("Object(%q).NewReader: %v", objectName, err)
	}
	defer rc.Close()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		log.Fatalf("reading file: %v", err)
	}

}

// Generates size random bytes.
func generateRandomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
