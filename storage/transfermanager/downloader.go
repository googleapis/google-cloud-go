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
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/googleapis/gax-go/v2/callctx"
	"google.golang.org/api/iterator"
)

// Downloader manages a set of parallelized downloads.
type Downloader struct {
	client              *storage.Client
	config              *transferManagerConfig
	inputs              []DownloadObjectInput
	results             []DownloadOutput
	errors              []error
	inputsMu            sync.Mutex
	resultsMu           sync.Mutex
	errorsMu            sync.Mutex
	work                chan *DownloadObjectInput // Piece of work to be executed.
	doneReceivingInputs chan bool                 // Indicates to finish up work; expecting no more inputs.
	workers             *sync.WaitGroup           // Keeps track of the workers that are currently running.
	downloadsInProgress *sync.WaitGroup           // Keeps track of how many objects have completed at least 1 shard, but are waiting on more
}

// DownloadObject queues the download of a single object. This will initiate the
// download but is non-blocking; call Downloader.Results or use the callback to
// process the result. DownloadObject is thread-safe and can be called
// simultaneously from different goroutines.
// The download may not start immediately if all workers are busy, so a deadline
// set on the ctx may time out before the download even starts. To set a timeout
// that starts with the download, use the [WithPerOpTimeout()] option.
func (d *Downloader) DownloadObject(ctx context.Context, input *DownloadObjectInput) error {
	if d.closed() {
		return errors.New("transfermanager: Downloader used after WaitAndClose was called")
	}
	if err := d.validateObjectInput(input); err != nil {
		return err
	}

	input.ctx = ctx
	d.addInput(input)
	return nil
}

// DownloadDirectory queues the download of a set of objects to a local path.
// This will initiate the download but is non-blocking; call Downloader.Results
// or use the callback to process the result. DownloadDirectory is thread-safe
// and can be called simultaneously from different goroutines.
// DownloadDirectory will resolve any filters on the input and create the needed
// directory structure locally as the operations progress.
// Note: DownloadDirectory overwrites existing files in the directory.
func (d *Downloader) DownloadDirectory(ctx context.Context, input *DownloadDirectoryInput) error {
	if d.closed() {
		return errors.New("transfermanager: Downloader used after WaitAndClose was called")
	}
	if err := d.validateDirectoryInput(input); err != nil {
		return err
	}

	query := &storage.Query{
		Prefix:      input.Prefix,
		StartOffset: input.StartOffset,
		EndOffset:   input.EndOffset,
		MatchGlob:   input.MatchGlob,
	}
	if err := query.SetAttrSelection([]string{"Name"}); err != nil {
		return fmt.Errorf("transfermanager: DownloadDirectory query.SetAttrSelection: %w", err)
	}

	// TODO: Clean up any created directory structure on failure.

	objectsToQueue := []string{}
	it := d.client.Bucket(input.Bucket).Objects(ctx, query)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("transfermanager: DownloadDirectory failed to list objects: %w", err)
		}

		objectsToQueue = append(objectsToQueue, attrs.Name)
	}

	outs := make(chan DownloadOutput, len(objectsToQueue))
	inputs := make([]DownloadObjectInput, 0, len(objectsToQueue))

	for _, object := range objectsToQueue {
		objDirectory := filepath.Join(input.LocalDirectory, filepath.Dir(object))
		filePath := filepath.Join(input.LocalDirectory, object)

		// Make sure all directories in the object path exist.
		err := os.MkdirAll(objDirectory, fs.ModeDir|fs.ModePerm)
		if err != nil {
			return fmt.Errorf("transfermanager: DownloadDirectory failed to make directory(%q): %w", objDirectory, err)
		}

		// Create file to download to.
		f, fErr := os.Create(filePath)
		if fErr != nil {
			return fmt.Errorf("transfermanager: DownloadDirectory failed to create file(%q): %w", filePath, fErr)
		}

		inputs = append(inputs, DownloadObjectInput{
			Bucket:                 input.Bucket,
			Object:                 object,
			Destination:            f,
			Callback:               input.OnObjectDownload,
			ctx:                    ctx,
			directory:              true,
			directoryObjectOutputs: outs,
		})
	}

	if d.config.asynchronous {
		d.downloadsInProgress.Add(1)
		go d.gatherObjectOutputs(input, outs, len(inputs))
	}
	d.addNewInputs(inputs)
	return nil
}

// WaitAndClose waits for all outstanding downloads to complete and closes the
// Downloader. Adding new downloads after this has been called will cause an error.
//
// WaitAndClose returns all the results of the downloads and an error wrapping
// all errors that were encountered by the Downloader when downloading objects.
// These errors are also returned in the respective DownloadOutput for the
// failing download. The results are not guaranteed to be in any order.
// Results will be empty if using the [WithCallbacks] option. WaitAndClose will
// wait for all callbacks to finish.
func (d *Downloader) WaitAndClose() ([]DownloadOutput, error) {
	errMsg := "transfermanager: at least one error encountered downloading objects:"
	select {
	case <-d.doneReceivingInputs: // this allows users to call WaitAndClose various times
		var err error
		if len(d.errors) > 0 {
			err = fmt.Errorf("%s\n%w", errMsg, errors.Join(d.errors...))
		}
		return d.results, err
	default:
		d.downloadsInProgress.Wait()
		d.doneReceivingInputs <- true
		d.workers.Wait()
		close(d.doneReceivingInputs)

		if len(d.errors) > 0 {
			return d.results, fmt.Errorf("%s\n%w", errMsg, errors.Join(d.errors...))
		}
		return d.results, nil
	}
}

// sendInputsToWorkChan polls the inputs slice until d.done.
// It will send all items in inputs to the d.work chan.
// Once it receives from d.done, it drains the remaining items in the inputs
// (sending them to d.work) and then closes the d.work chan.
func (d *Downloader) sendInputsToWorkChan() {
	for {
		select {
		case <-d.doneReceivingInputs:
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
	if input.shard == 0 {
		d.downloadsInProgress.Add(1)
	}
	d.inputsMu.Lock()
	d.inputs = append(d.inputs, *input)
	d.inputsMu.Unlock()
}

// addNewInputs adds a slice of inputs to the downloader.
// This should only be used to queue new objects.
func (d *Downloader) addNewInputs(inputs []DownloadObjectInput) {
	d.downloadsInProgress.Add(len(inputs))

	d.inputsMu.Lock()
	d.inputs = append(d.inputs, inputs...)
	d.inputsMu.Unlock()
}

func (d *Downloader) addResult(input *DownloadObjectInput, result *DownloadOutput) {
	copiedResult := *result // make a copy so that callbacks do not affect the  result

	if input.directory {
		f := input.Destination.(*os.File)
		if err := f.Close(); err != nil && result.Err == nil {
			result.Err = fmt.Errorf("closing file(%q): %w", f.Name(), err)
		}

		if d.config.asynchronous {
			input.directoryObjectOutputs <- copiedResult
		}
	}
	// TODO: check checksum if full object

	if d.config.asynchronous || input.directory {
		input.Callback(result)
	}
	if !d.config.asynchronous {
		d.resultsMu.Lock()
		d.results = append(d.results, copiedResult)
		d.resultsMu.Unlock()
	}

	// Track all errors that occurred.
	if result.Err != nil {
		d.error(fmt.Errorf("downloading %q from bucket %q: %w", input.Object, input.Bucket, result.Err))
	}
	d.downloadsInProgress.Done()
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

		out := input.downloadShard(d.client, d.config.perOperationTimeout, d.config.partSize)

		if input.shard == 0 {
			if out.Err != nil {
				// Don't queue more shards if the first failed.
				d.addResult(input, out)
			} else {
				numShards := numShards(out.Attrs, input.Range, d.config.partSize)

				if numShards <= 1 {
					// Download completed with a single shard.
					d.addResult(input, out)
				} else {
					// Queue more shards.
					outs := d.queueShards(input, out.Attrs.Generation, numShards)
					// Start a goroutine that gathers shards sent to the output
					// channel and adds the result once it has received all shards.
					go d.gatherShards(input, outs, numShards)
				}
			}
		} else {
			// If this isn't the first shard, send to the output channel specific to the object.
			// This should never block since the channel is buffered to exactly the number of shards.
			input.shardOutputs <- out
		}
	}
	d.workers.Done()
}

// queueShards queues all subsequent shards of an object after the first.
// The results should be forwarded to the returned channel.
func (d *Downloader) queueShards(in *DownloadObjectInput, gen int64, shards int) <-chan *DownloadOutput {
	// Create a channel that can be received from to compile the
	// shard outputs.
	outs := make(chan *DownloadOutput, shards)
	in.shardOutputs = outs

	// Create a shared context that we can cancel if a shard fails.
	in.ctx, in.cancelCtx = context.WithCancelCause(in.ctx)

	// Add generation in case the object changes between calls.
	in.Generation = &gen

	// Queue remaining shards.
	for i := 1; i < shards; i++ {
		newShard := in // this is fine, since the input should only differ in the shard num
		newShard.shard = i
		d.addInput(newShard)
	}

	return outs
}

var errCancelAllShards = errors.New("cancelled because another shard failed")

// gatherShards receives from the given channel exactly (shards-1) times (since
// the first shard should already be complete).
// It will add the result to the Downloader once it has received all shards.
// gatherShards cancels remaining shards if any shard errored.
// It does not do any checking to verify that shards are for the same object.
func (d *Downloader) gatherShards(in *DownloadObjectInput, outs <-chan *DownloadOutput, shards int) {
	errs := []error{}
	var shardOut *DownloadOutput
	for i := 1; i < shards; i++ {
		// Add monitoring here? This could hang if any individual piece does.
		shardOut = <-outs

		// We can ignore errors that resulted from a previous error.
		// Note that we may still get some cancel errors if they
		// occurred while the operation was already in progress.
		if shardOut.Err != nil && !(errors.Is(shardOut.Err, context.Canceled) && errors.Is(context.Cause(in.ctx), errCancelAllShards)) {
			// If a shard errored, track the error and cancel the shared ctx.
			errs = append(errs, shardOut.Err)
			in.cancelCtx(errCancelAllShards)
		}
	}

	// All pieces gathered; return output. Any shard output will do.
	shardOut.Range = in.Range
	if len(errs) != 0 {
		shardOut.Err = fmt.Errorf("download shard errors:\n%w", errors.Join(errs...))
	}
	d.addResult(in, shardOut)
}

// gatherObjectOutputs receives from the given channel exactly numObjects times.
// It will execute the callback once all object outputs are received.
// It does not do any verification on the outputs nor does it cancel other
// objects on error.
func (d *Downloader) gatherObjectOutputs(in *DownloadDirectoryInput, gatherOuts <-chan DownloadOutput, numObjects int) {
	outs := make([]DownloadOutput, 0, numObjects)
	for i := 0; i < numObjects; i++ {
		obj := <-gatherOuts
		outs = append(outs, obj)
	}

	// All objects have been gathered; execute the callback.
	in.Callback(outs)
	d.downloadsInProgress.Done()
}

func (d *Downloader) validateObjectInput(in *DownloadObjectInput) error {
	if d.config.asynchronous && in.Callback == nil {
		return errors.New("transfermanager: input.Callback must not be nil when the WithCallbacks option is set")
	}
	if !d.config.asynchronous && in.Callback != nil {
		return errors.New("transfermanager: input.Callback must be nil unless the WithCallbacks option is set")
	}
	return nil
}

func (d *Downloader) validateDirectoryInput(in *DownloadDirectoryInput) error {
	if d.config.asynchronous && in.Callback == nil {
		return errors.New("transfermanager: input.Callback must not be nil when the WithCallbacks option is set")
	}
	if !d.config.asynchronous && in.Callback != nil {
		return errors.New("transfermanager: input.Callback must be nil unless the WithCallbacks option is set")
	}
	return nil
}

func (d *Downloader) closed() bool {
	select {
	case <-d.doneReceivingInputs:
		return true
	default:
		return false
	}
}

// NewDownloader creates a new Downloader to add operations to.
// Choice of transport, etc is configured on the client that's passed in.
// The returned Downloader can be shared across goroutines to initiate downloads.
func NewDownloader(c *storage.Client, opts ...Option) (*Downloader, error) {
	d := &Downloader{
		client:              c,
		config:              initTransferManagerConfig(opts...),
		inputs:              []DownloadObjectInput{},
		results:             []DownloadOutput{},
		errors:              []error{},
		work:                make(chan *DownloadObjectInput),
		doneReceivingInputs: make(chan bool),
		workers:             &sync.WaitGroup{},
		downloadsInProgress: &sync.WaitGroup{},
	}

	// Start a polling routine to send work through.
	go d.sendInputsToWorkChan()

	// Start workers.
	for i := 0; i < d.config.numWorkers; i++ {
		d.workers.Add(1)
		go d.downloadWorker()
	}

	return d, nil
}

// DownloadRange specifies the object range.
// If the object's metadata property "Content-Encoding" is set to "gzip" or
// satisfies decompressive transcoding per https://cloud.google.com/storage/docs/transcoding
// that file will be served back whole, regardless of the requested range as
// Google Cloud Storage dictates.
type DownloadRange struct {
	// Offset is the starting offset (inclusive) from with the object is read.
	// If offset is negative, the object is not sharded and is read by a single
	// worker abs(offset) bytes from the end, and length must also be negative
	// to indicate all remaining bytes will be read.
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

	// Callback will be run once the object is finished downloading. It must be
	// set if and only if the [WithCallbacks] option is set; otherwise, it must
	// not be set.
	// A worker will be used to execute the callback; therefore, it should not
	// be a long-running function. WaitAndClose will wait for all callbacks to
	// finish.
	Callback func(*DownloadOutput)

	ctx                    context.Context
	cancelCtx              context.CancelCauseFunc
	shard                  int // the piece of the object range that should be downloaded
	shardOutputs           chan<- *DownloadOutput
	directory              bool // input was queued by calling DownloadDirectory
	directoryObjectOutputs chan<- DownloadOutput
}

// downloadShard will read a specific object piece into in.Destination.
// If timeout is less than 0, no timeout is set.
func (in *DownloadObjectInput) downloadShard(client *storage.Client, timeout time.Duration, partSize int64) (out *DownloadOutput) {
	out = &DownloadOutput{Bucket: in.Bucket, Object: in.Object, Range: in.Range}

	// Set timeout.
	ctx := in.ctx
	if timeout > 0 {
		c, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		ctx = c
	}

	// The first shard will be sent as download many, since we do not know yet
	// if it will be sharded.
	method := downloadMany
	if in.shard != 0 {
		method = downloadSharded
	}
	ctx = setUsageMetricHeader(ctx, method)

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

	objRange := shardRange(in.Range, partSize, in.shard)

	// Read.
	r, err := o.NewRangeReader(ctx, objRange.Offset, objRange.Length)
	if err != nil {
		out.Err = err
		return
	}

	// Determine the offset this shard should write to.
	offset := objRange.Offset
	if in.Range != nil {
		if in.Range.Offset > 0 {
			offset = objRange.Offset - in.Range.Offset
		} else {
			offset = 0
		}
	}

	w := io.NewOffsetWriter(in.Destination, offset)
	_, err = io.Copy(w, r)
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

// DownloadDirectoryInput is the input for a directory to download.
type DownloadDirectoryInput struct {
	// Bucket is the bucket in GCS to download from. Required.
	Bucket string

	// LocalDirectory specifies the directory to download the matched objects
	// to. Relative paths are allowed. The directory structure and contents
	// must not be modified while the download is in progress.
	// The directory will be created if it does not already exist. Required.
	LocalDirectory string

	// Prefix is the prefix filter to download objects whose names begin with this.
	// Optional.
	Prefix string

	// StartOffset is used to filter results to objects whose names are
	// lexicographically equal to or after startOffset. If endOffset is also
	// set, the objects listed will have names between startOffset (inclusive)
	// and endOffset (exclusive). Optional.
	StartOffset string

	// EndOffset is used to filter results to objects whose names are
	// lexicographically before endOffset. If startOffset is also set, the
	// objects listed will have names between startOffset (inclusive) and
	// endOffset (exclusive). Optional.
	EndOffset string

	// MatchGlob is a glob pattern used to filter results (for example, foo*bar). See
	// https://cloud.google.com/storage/docs/json_api/v1/objects/list#list-object-glob
	// for syntax details. Optional.
	MatchGlob string

	// Callback will run after all the objects in the directory as selected by
	// the provided filters are finished downloading.
	// It must be set if and only if the [WithCallbacks] option is set.
	// WaitAndClose will wait for all callbacks to finish.
	Callback func([]DownloadOutput)

	// OnObjectDownload will run after every finished object download. Optional.
	OnObjectDownload func(*DownloadOutput)
}

// DownloadOutput provides output for a single object download, including all
// errors received while downloading object parts. If the download was successful,
// Attrs will be populated.
type DownloadOutput struct {
	Bucket string
	Object string
	Range  *DownloadRange             // requested range, if it was specified
	Err    error                      // error occurring during download
	Attrs  *storage.ReaderObjectAttrs // attributes of downloaded object, if successful
}

// TODO: use built-in after go < 1.21 is dropped.
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// numShards calculates how many shards the given range should be divided into
// given the part size.
func numShards(attrs *storage.ReaderObjectAttrs, r *DownloadRange, partSize int64) int {
	objectSize := attrs.Size

	// Transcoded objects do not support ranged reads.
	if attrs.ContentEncoding == "gzip" {
		return 1
	}

	if r == nil {
		// Divide entire object into shards.
		return int(math.Ceil(float64(objectSize) / float64(partSize)))
	}
	// Negative offset reads the whole object in one go.
	if r.Offset < 0 {
		return 1
	}

	firstByte := r.Offset

	// Read to the end of the object, or, if smaller, read only the amount
	// of bytes requested.
	lastByte := min(objectSize-1, r.Offset+r.Length-1)

	// If length is negative, read to the end.
	if r.Length < 0 {
		lastByte = objectSize - 1
	}

	totalBytes := lastByte - firstByte

	return int(totalBytes/partSize) + 1
}

// shardRange calculates the range this shard corresponds to given the
// requested range. Expects the shard to be valid given the range.
func shardRange(r *DownloadRange, partSize int64, shard int) DownloadRange {
	if r == nil {
		// Entire object
		return DownloadRange{
			Offset: int64(shard) * partSize,
			Length: partSize,
		}
	}

	// Negative offset reads the whole object in one go.
	if r.Offset < 0 {
		return *r
	}

	shardOffset := int64(shard)*partSize + r.Offset
	shardLength := partSize // it's ok if we go over the object size

	// If requested bytes end before partSize, length should be smaller.
	if shardOffset+shardLength > r.Offset+r.Length {
		shardLength = r.Offset + r.Length - shardOffset
	}

	return DownloadRange{
		Offset: shardOffset,
		Length: shardLength,
	}
}

const (
	xGoogHeaderKey  = "x-goog-api-client"
	usageMetricKey  = "gccl-gcs-cmd"
	downloadMany    = "tm.download_many"
	downloadSharded = "tm.download_sharded"
)

// Sets invocation ID headers on the context which will be propagated as
// headers in the call to the service (for both gRPC and HTTP).
func setUsageMetricHeader(ctx context.Context, method string) context.Context {
	header := fmt.Sprintf("%s/%s", usageMetricKey, method)
	return callctx.SetHeaders(ctx, xGoogHeaderKey, header)
}
