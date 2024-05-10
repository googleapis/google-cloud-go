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
	d.inputsMu.Lock()
	d.inputs = append(d.inputs, *input)
	d.inputsMu.Unlock()
}

// DownloadObject queues the download of a single object. This will initiate the
// download but is non-blocking. The result will not be added to the results obtained
// from Downloader.Results; use the callback to process the result.
func (d *Downloader) DownloadObjectWithCallback(ctx context.Context, input *DownloadObjectInput, callback func(*DownloadOutput)) {
	input.ctx = ctx
	input.callback = &callback
	d.inputsMu.Lock()
	d.inputs = append(d.inputs, *input)
	d.inputsMu.Unlock()
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

// Results returns the iterator for download outputs.
func (d *Downloader) Results() []DownloadOutput {
	return d.results
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
			d.errorsMu.Lock()
			d.errors = append(d.errors, out.Err)
			d.errorsMu.Unlock()
		}

		// Either execute the callback, or append to results.
		if input.callback != nil {
			(*input.callback)(out)
		} else {
			d.resultsMu.Lock()
			d.results = append(d.results, *out)
			d.resultsMu.Unlock()
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

// DownloadOutput provides output for a single object download, including all
// errors received while downloading object parts. If the download was successful,
// Attrs will be populated.
type DownloadOutput struct {
	Bucket string
	Object string
	Err    error                      // error occurring during download
	Attrs  *storage.ReaderObjectAttrs // attributes of downloaded object, if successful
}

// // DownloadOutputIterator allows the end user to iterate through completed
// // object downloads.
// type DownloadOutputIterator struct {
// 	output <-chan *DownloadOutput
// }

// // Next iterates through results. When complete, will return the iterator.Done
// // error. It is considered complete once WaitAndClose() has been called on the
// // Downloader.
// // Note that if there was an error reading an object, it will not be returned
// // by Next - check DownloadOutput.Err instead.
// // DownloadOutputs will be available as the downloads complete; they can
// // be iterated through asynchronously or at the end of the job.
// // Next will block if there are no more completed downloads (and the Downloader
// // is not closed).
// func (it *DownloadOutputIterator) Next() (*DownloadOutput, error) {
// 	out, ok := <-it.output
// 	if !ok {
// 		return nil, iterator.Done
// 	}
// 	fmt.Println(out)
// 	return out, nil
// }

// // listenForAndDelegateWork will receive from the work chan, and start goroutines
// // to execute that work without blocking.
// // It should be called only once.
// func (d *Downloader) listenForAndDelegateWork1() {
// 	for {
// 		// Dequeue the work. Can block.
// 		input, ok := <-d.work
// 		if !ok {
// 			break // no more work; exit
// 		}

// 		// Start a worker. This may block.
// 		d.workerGroup.Go(func() error {
// 			// Do the download.
// 			// TODO: break down the input into smaller pieces if necessary; maybe as follows:
// 			// Only request partSize data to begin with. If no error and we haven't finished
// 			// reading the object, enqueue the remaining pieces of work (by sending them to d.work)
// 			// and mark in the out var the amount of shards to wait for.
// 			out := input.downloadShard(d.client, d.config.perOperationTimeout)

// 			// Send the output to be received by Next. This could block until received
// 			// (ie. the user calls Next), but it's okay; our worker has already returned.
// 			// Alternatively, we could feed these to a slice that we grab from in Next.
// 			// This would not block then, but would require synchronization of the slice.
// 			go func() {
// 				out := out
// 				d.output <- out
// 			}()

// 			// We return the error here, to communicate to d.workergroup.Wait
// 			// that there has been an error.
// 			// Since the group does not use a shared context, this should not
// 			// affect any of the other operations using the group.
// 			// TO-DO: in addition to this, if we want WaitAndClose to return a
// 			// multi err, we will need to record these errors somewhere.
// 			// Where-ever we record those, it will need synchronization since
// 			// this is concurrent.
// 			return out.Err
// 		})

// 	}
// }

// // drainInput consumes everything in the inputs slice and dispatches workers.
// // It will block if there are not enough workers to consume every input, until
// // all inputs are dispatched to an available worker.
// func (d *Downloader) drainInpu1t() {
// 	fmt.Println(len(d.inputs))

// 	for len(d.inputs) > 0 {
// 		d.inputsMu.Lock()
// 		if len(d.inputs) < 1 {
// 			return
// 		}
// 		input := d.inputs[0]
// 		d.inputs = d.inputs[1:]
// 		d.inputsMu.Unlock()

// 		// Start a worker. This may block, but only if there aren't enough workers.
// 		d.workerGroup.Go(func() error {
// 			// Do the download.
// 			// TODO: break down the input into smaller pieces if necessary; maybe as follows:
// 			// Only request partSize data to begin with. If no error and we haven't finished
// 			// reading the object, enqueue the remaining pieces of work
// 			// and mark in the out var the amount of shards to wait for.
// 			out := input.downloadShard(d.client, d.config.perOperationTimeout)

// 			// Either return the callback, or append to results.
// 			if input.callback != nil {
// 				(*input.callback)(out)
// 			} else {
// 				d.resultsMu.Lock()
// 				d.results = append(d.results, *out)
// 				fmt.Println(len(d.results))

// 				d.resultsMu.Unlock()
// 			}

// 			// We return the error here, to communicate to d.workergroup.Wait
// 			// that there has been an error.
// 			// Since the group does not use a shared context, this should not
// 			// affect any of the other operations using the group.
// 			// TO-DO: in addition to this, if we want WaitAndClose to return a
// 			// multi err, we will need to record these errors somewhere.
// 			// Where-ever we record those, it will need synchronization since
// 			// this is concurrent.
// 			return out.Err
// 		})
// 	}
// }
