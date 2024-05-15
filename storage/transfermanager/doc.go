// Copyright 2024 Google LLC
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

/*
Package transfermanager provides an easy way to parallelize downloads in Google
Cloud Storage.

More information about Google Cloud Storage is available at
https://cloud.google.com/storage/docs.

See https://pkg.go.dev/cloud.google.com/go for authentication, timeouts,
connection pooling and similar aspects of this package.

NOTE: This package is in alpha. It is not stable, and is likely to change.

# Example usage

	// Pass in any client opts or set retry policy here.
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

	// Create local file writer for output.
	f, err := os.Create("/path/to/localfile")
	if err != nil {
		// handle error
	}

	// Create download input
	in := &transfermanager.DownloadObjectInput{
		Bucket:      "mybucket",
		Object:      "myblob",
		Destination: f,
		// Optionally specify params to apply to download.
		EncryptionKey: []byte("mykey"),
	}

	// Can set timeout on this download using context. Note that this download
	// may not start immediately if all workers are busy, so this may time out
	// before the download even starts. To set a timeout that starts with the
	// download, use transfermanager.WithPerOpTimeout(time.Duration).
	ctx, cancel = context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	// Add to Downloader.
	d.DownloadObject(ctx, in)

	// Repeat if desired.

	// Wait for all downloads to complete.
	d.WaitAndClose()

	// Iterate through completed downloads and process results. This can
	// also happen async in a go routine as the downloads run.
	results := d.Results()
	for _, out := range results {
		if out.Err != nil {
			log.Printf("download of %v failed with error %v", out.Name, out.Err)
		} else {
			log.Printf("download of %v succeeded", out.Object)
		}
	}
*/
package transfermanager // import "cloud.google.com/go/storage/transfermanager"
