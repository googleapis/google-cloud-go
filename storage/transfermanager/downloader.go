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

package transfermanager

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// Downloader manages parallel download operations from a Cloud Storage bucket.
type Downloader struct {
	client  *storage.Client
	config  *transferManagerConfig
	work    chan *DownloadObjectInput // Piece of work to be executed.
	output  chan *DownloadOutput      // Channel for completed downloads; used to feed results iterator.
	workers *sync.WaitGroup           // Keeps track of the workers that are currently running.
}

// DownloadObject queues the download of a single object. If it's larger than
// the specified part size, the download will automatically be broken up into
// multiple range reads. This will initiate the download but is non-blocking;
// call Downloader.Results to process the result.
func (d *Downloader) DownloadObject(ctx context.Context, input *DownloadObjectInput) {
	input.ctx = ctx
	d.work <- input
}

// Download a set of objects to a local path. Downloader will create the needed
// directory structure locally as the operations progress.
// This will initiate the download but is non-blocking; wait on Downloader.Results
// to process the result. Results will be split into individual objects.
// NOTE: Do not use, DownloadDirectory is not implemented.
func (d *Downloader) DownloadDirectory(ctx context.Context, input *DownloadDirectoryInput) {
	d.output <- &DownloadOutput{Bucket: input.Bucket, Err: errors.New("DownloadDirectory is not implemented")}
	// This does obj listing/os.File calls and then converts to object inputs.
}

// Waits for all outstanding downloads to complete. The Downloader must not be
// used to download more objects or directories after this has been called.
func (d *Downloader) WaitAndClose() error {
	close(d.work)
	d.workers.Wait()
	close(d.output)
	return nil
}

// Results returns the iterator for download outputs.
func (d *Downloader) Results() *DownloadOutputIterator {
	return &DownloadOutputIterator{
		output: d.output,
	}
}

// downloadWorker continuously processes downloads until the work channel is closed.
func (d *Downloader) downloadWorker() {
	d.workers.Add(1)
	for {
		input, ok := <-d.work
		if !ok {
			break // no more work; exit
		}

		// TODO: break down the input into smaller pieces if necessary; maybe as follows:
		// Only request partSize data to begin with. If no error and we haven't finished
		// reading the object, enqueue the remaining pieces of work and mark in the
		// out var the amount of shards to wait for.
		d.output <- input.downloadShard(d.client, d.config.perOperationTimeout)
	}
	d.workers.Done()
}

// NewDownloader creates a new Downloader to add operations to.
// Choice of transport, etc is configured on the client that's passed in.
func NewDownloader(c *storage.Client, opts ...TransferManagerOption) (*Downloader, error) {
	const (
		chanBufferSize = 1000 // how big is it reasonable to make this?
		// We should probably expose this as max concurrent ops, because programs can deadlock if calling d.Waitclose before processing it.Next
	)

	d := &Downloader{
		client:  c,
		config:  initTransferManagerConfig(opts...),
		output:  make(chan *DownloadOutput, chanBufferSize),
		work:    make(chan *DownloadObjectInput, chanBufferSize),
		workers: &sync.WaitGroup{},
	}

	// Start workers in background.
	for i := 0; i < d.config.numWorkers; i++ {
		go d.downloadWorker()
	}

	return d, nil
}

// DownloadRange specifies the object range.
type DownloadRange struct {
	// Offset is the starting offset (inclusive) from with the object is read.
	// If offset is negative, the object is read abs(offset) bytes from the end,
	// and length must also be negative to indicate all remaining bytes will be read.
	Offset int64
	// Length is the number of bytes to read.
	// If length is negative or larger than the object size, the object is read
	// until the end.
	Length int64
}

// DownloadObjectInput is the input for a single object to download.
type DownloadObjectInput struct {
	// Required fields
	Bucket      string
	Object      string
	Destination io.WriterAt

	// Optional fields
	Generation    *int64
	Conditions    *storage.Conditions
	EncryptionKey []byte
	Range         *DownloadRange // if specified, reads only a range

	ctx context.Context
}

// downloadShard will read a specific object into in.Destination.
// TODO: download a single shard instead of the entire object.
func (in *DownloadObjectInput) downloadShard(client *storage.Client, timeout time.Duration) (out *DownloadOutput) {
	out = &DownloadOutput{Bucket: in.Bucket, Object: in.Object}

	// Set timeout.
	ctx := in.ctx
	if timeout > 0 {
		c, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		ctx = c
	}

	// Set options on the object.
	o := client.Bucket(in.Bucket).Object(in.Object)

	if in.Conditions != nil {
		o.If(*in.Conditions)
	}
	if in.Generation != nil {
		o = o.Generation(*in.Generation)
	}
	if len(in.EncryptionKey) > 0 {
		o = o.Key(in.EncryptionKey)
	}

	var offset, length int64 = 0, -1 // get the entire object by default

	if in.Range != nil {
		offset, length = in.Range.Offset, in.Range.Length
	}

	// Read.
	r, err := o.NewRangeReader(ctx, offset, length)
	if err != nil {
		out.Err = err
		return
	}

	// TODO: write at a specific offset.
	off := io.NewOffsetWriter(in.Destination, 0)
	_, err = io.Copy(off, r)
	if err != nil {
		out.Err = err
		r.Close()
		return
	}

	if err = r.Close(); err != nil {
		out.Err = err
		return
	}

	out.Attrs = &r.Attrs
	return
}

type DownloadDirectoryInput struct {
	// Required fields
	Bucket         string
	LocalDirectory string

	// Optional fields to filter objects in bucket. Selection from storage.Query.
	Prefix                   string
	StartOffset              string
	EndOffset                string
	IncludeTrailingDelimiter bool // maybe unnecessary?
	MatchGlob                string

	// Maybe some others about local file naming.
}

// DownloadOutput provides output for a single object download, including all
// errors received while downloading object parts. If the download was successful,
// Attrs will be populated.
type DownloadOutput struct {
	Bucket string
	Object string
	Err    error                      // error occurring during download. Can use multi-error in Go 1.20+ if multiple failures.
	Attrs  *storage.ReaderObjectAttrs // Attributes of downloaded object, if successful.
}

// DownloadOutputIterator allows the end user to iterate through completed
// object downloads.
type DownloadOutputIterator struct {
	output <-chan *DownloadOutput
}

// Next iterates through results. When complete, will return the iterator.Done
// error. It is considered complete once WaitAndClose() has been called on the
// Downloader.
// DownloadOutputs will be available as the downloads complete; they can
// be iterated through asynchronously or at the end of the job.
// Next will block if there are no more completed downloads (and the Downloader
// is not closed).
func (it *DownloadOutputIterator) Next() (*DownloadOutput, error) {
	out, ok := <-it.output
	if !ok {
		return nil, iterator.Done
	}
	return out, nil
}
