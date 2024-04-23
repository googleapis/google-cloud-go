package main

import (
	"context"
	"log"
	"os"
	"time"

	"cloud.google.com/go/storage"
	"cloud.google.com/go/storage/transfermanager"
	"google.golang.org/api/iterator"
)

func main() {
	ctx := context.Background()

	// Pass in any client opts or set retry policy here
	client, err := storage.NewClient(ctx) // can also use NewGRPCClient
	if err != nil {
		// handle error
	}

	// Create Downloader with desired options, including number of workers,
	// part size, per operation timeout, etc.
	d, err := transfermanager.NewDownloader(client, transfermanager.WithWorkers(16))
	if err != nil {
		// handle error
	}

	// Sharded and/or parallelized download
	// Create local file writer for output
	f, err := os.Create("/path/to/localfile")
	if err != nil {
		// handle error
	}

	// Create download input
	in := &transfermanager.DownloadObjectInput{
		Bucket:      "mybucket",
		Source:      "myblob",
		Destination: f,
		// Optionally specify params to apply to download.
		EncryptionKey: []byte("mykey"),
	}

	// Can set timeout on this download using context.
	ctx, cancel = context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	// Add to Downloader
	d.DownloadObject(ctx, in)

	// Repeat if desired

	// Download many files to path
	// Create query, using any GCS list objects options
	dirIn := &transfermanager.DownloadDirectoryInput{
		Bucket:         "mybucket",
		LocalDirectory: "/path/to/dir",

		// Optional filtering within bucket.
		Prefix:    "objectprefix/",
		MatchGlob: "objectprefix/**abc**",
	}

	d.DownloadDirectory(ctx, dirIn)

	// Wait for all downloads to complete.
	d.WaitAndClose()

	// Iterate through completed downloads and process results. This can
	// also happen async in a go routine as the downloads run.
	it := d.Results()
	for {
		out, err := it.Next() // if async, blocks until next result is available.
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("error getting next result: %v")
		}
		if out.Err != nil {
			log.Printf("download of %v failed with error %v", out.Name, out.Err)
		} else {
			log.Printf("download of %v succeeded", out.Name)
		}
	}

}
