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
)

// Downloader manages a set of parallelized downloads.
type Downloader struct {
	client    *storage.Client
	config    *transferManagerConfig
	inputs    []DownloadObjectInput
	results   []DownloadOutput
	errors    []error
	inputsMu  *sync.Mutex
	resultsMu *sync.Mutex
	errorsMu  *sync.Mutex
	work      chan *DownloadObjectInput // Piece of work to be executed.
	done      chan bool                 // Indicates to finish up work; expecting no more inputs.
	workers   *sync.WaitGroup           // Keeps track of the workers that are currently running.
}

// DownloadObject queues the download of a single object. This will initiate the
// download but is non-blocking; call Downloader.Results to process the result.
func (d *Downloader) DownloadObject(ctx context.Context, input *DownloadObjectInput) {
	input.ctx = ctx
	d.addInput(input)
}

// DownloadObject queues the download of a single object. This will initiate the
// download but is non-blocking. The result will not be added to the results obtained
// from Downloader.Results; use the callback to process the result.
func (d *Downloader) DownloadObjectWithCallback(ctx context.Context, input *DownloadObjectInput, callback func(*DownloadOutput)) {
	input.ctx = ctx
	input.callback = &callback
	d.addInput(input)
}

// WaitAndClose waits for all outstanding downloads to complete. The Downloader
// must not be used for any more downloads after this has been called.
// WaitAndClose returns an error if any of the downloads fail.
func (d *Downloader) WaitAndClose() error {
	d.done <- true
	d.workers.Wait()

	if len(d.errors) > 0 {
		// TODO: return a multierror instead with go 1.20+ Join
		return errors.New("transfermanager: at least one error encountered downloading objects")
	}
	return nil
}

// Results returns all the results of the downloads completed since the last
// time it was called. Call WaitAndClose before calling Results to wait for all
// downloads to complete.
// Results will not return results for downloads initiated with a callback.
func (d *Downloader) Results() []DownloadOutput {
	d.resultsMu.Lock()
	r := make([]DownloadOutput, len(d.results))
	copy(r, d.results)
	d.results = []DownloadOutput{}
	d.resultsMu.Unlock()

	return r
}

// sendInputsToWorkChan listens continuously to the inputs slice until d.done.
// It will send all items in inputs to the d.work chan.
// Once it receives from d.done, it drains the remaining items in the inputs
// (sending them to d.work) and then closes the d.work chan.
func (d *Downloader) sendInputsToWorkChan() {
	for {
		select {
		case <-d.done:
			d.drainInput()
			close(d.work)
			return
		default:
			d.drainInput()
		}
	}
}

// drainInput consumes everything in the inputs slice and sends it to the work chan.
// It will block if there are not enough workers to consume every input, until all
// inputs are received on the work chan(ie. they're dispatched to an available worker).
func (d *Downloader) drainInput() {
	for {
		d.inputsMu.Lock()
		if len(d.inputs) < 1 {
			d.inputsMu.Unlock()
			return
		}
		input := d.inputs[0]
		d.inputs = d.inputs[1:]
		d.inputsMu.Unlock()
		d.work <- &input
	}
}

func (d *Downloader) addInput(input *DownloadObjectInput) {
	d.inputsMu.Lock()
	d.inputs = append(d.inputs, *input)
	d.inputsMu.Unlock()
}

func (d *Downloader) addResult(result *DownloadOutput) {
	d.resultsMu.Lock()
	d.results = append(d.results, *result)
	d.resultsMu.Unlock()
}

func (d *Downloader) error(err error) {
	d.errorsMu.Lock()
	d.errors = append(d.errors, err)
	d.errorsMu.Unlock()
}

// downloadWorker continuously processes downloads until the work channel is closed.
func (d *Downloader) downloadWorker() {
	for {
		input, ok := <-d.work
		if !ok {
			break // no more work; exit
		}

		// TODO: break down the input into smaller pieces if necessary; maybe as follows:
		// Only request partSize data to begin with. If no error and we haven't finished
		// reading the object, enqueue the remaining pieces of work and mark in the
		// out var the amount of shards to wait for.
		out := input.downloadShard(d.client, d.config.perOperationTimeout)

		// Keep track of any error that occurred.
		if out.Err != nil {
			d.error(out.Err)
		}

		// Either execute the callback, or append to results.
		if input.callback != nil {
			(*input.callback)(out)
		} else {
			d.addResult(out)
		}
	}
	d.workers.Done()
}

// NewDownloader creates a new Downloader to add operations to.
// Choice of transport, etc is configured on the client that's passed in.
func NewDownloader(c *storage.Client, opts ...TransferManagerOption) (*Downloader, error) {
	d := &Downloader{
		client:    c,
		config:    initTransferManagerConfig(opts...),
		inputs:    []DownloadObjectInput{},
		results:   []DownloadOutput{},
		errors:    []error{},
		inputsMu:  &sync.Mutex{},
		resultsMu: &sync.Mutex{},
		errorsMu:  &sync.Mutex{},
		work:      make(chan *DownloadObjectInput),
		done:      make(chan bool),
		workers:   &sync.WaitGroup{},
	}

	// Start a listener to send work through.
	go d.sendInputsToWorkChan()

	// Start workers.
	for i := 0; i < d.config.numWorkers; i++ {
		d.workers.Add(1)
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

	ctx      context.Context
	callback *func(*DownloadOutput)
}

// downloadShard will read a specific object into in.Destination.
// If timeout is less than 0, no timeout is set.
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
		o = o.If(*in.Conditions)
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

// DownloadOutput provides output for a single object download, including all
// errors received while downloading object parts. If the download was successful,
// Attrs will be populated.
type DownloadOutput struct {
	Bucket string
	Object string
	Err    error                      // error occurring during download
	Attrs  *storage.ReaderObjectAttrs // attributes of downloaded object, if successful
}
