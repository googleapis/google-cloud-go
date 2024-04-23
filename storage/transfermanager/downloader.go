package transfermanager

import (
	"context"
	"io"
	"time"

	"cloud.google.com/go/storage"
)

type TransferManagerOption interface {
	apply(*transferManagerConfig)
}

func WithWorkers(numWorkers int) TransferManagerOption {
	return &withWorkers{numWorkers: numWorkers}
}

type withWorkers struct {
	numWorkers int
}

func (ww withWorkers) apply(tm *transferManagerConfig) {
	tm.numWorkers = ww.numWorkers
}

func WithPartSize(partSize int) TransferManagerOption {
	return &withPartSize{partSize: partSize}
}

type withPartSize struct {
	partSize int
}

func (wps withPartSize) apply(tm *transferManagerConfig) {
	tm.partSize = wps.partSize
}

// etc for additional options.

type transferManagerConfig struct {
	// Workers in thread pool; default numCPU/2 based on previous benchmarks?
	numWorkers int
	// Size of shards to transfer; Python found 32 MiB to be good default for
	// JSON downloads but gRPC may benefit from larger.
	partSize int
	// Timeout for a single operation (including all retries).
	perOperationTimeout time.Duration

	// others, including opts that apply to uploads.
}

// etc

type Downloader struct {
	ctx             context.Context
	client          *storage.Client
	config          *transferManagerConfig
	objectInputs    []DownloadObjectInput    // Will be sent via a channel to workers
	directoryInputs []DownloadDirectoryInput // This does obj listing/os.File calls and then converts to object inputs
	output          <-chan *DownloadOutput   // Channel for completed downloads; used to feed results iterator
	done            <-chan bool              // Used to signal completion of all downloads.
	// etc
}

// Create a new Downloader to add operations to.
// Choice of transport, etc is configured on the client that's passed in.
func NewDownloader(c *storage.Client, opts ...TransferManagerOption) (*Downloader, error) {

	return nil, nil
}

// Input for a single object to download.
type DownloadObjectInput struct {
	// Required fields
	Bucket      string
	Source      string
	Destination io.WriterAt

	// Optional fields
	Generation     *int64
	Conditions     *storage.Conditions
	EncryptionKey  []byte
	Offset, Length int64 // if specified, reads only a range.
}

// Download a single object. If it's larger than the specified part size,
// the operation will automatically be broken up into multiple range reads.
// This will initiate the download but is non-blocking; wait on Downloader.Output
// to process the results.
func (d *Downloader) DownloadObject(ctx context.Context, input *DownloadObjectInput) {
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

// Download a set of objects to a local path. Downloader will
// resolve the query and created the needed directory structure locally as the
// operations progress.
// This will initiate the download but is non-blocking; wait on Downloader.Output
// to process the result.
func (d *Downloader) DownloadDirectory(ctx context.Context, input *DownloadDirectoryInput) {
}

// Waits for all outstanding downloads to complete. Then, closes the Output
// channel.
func (d *Downloader) WaitAndClose() error {
	return nil
}

// Results returns the iterator for download outputs.
func (d *Downloader) Results() *DownloadOutputIterator {
	return nil
}

// DownloadOutput provides output for a single object download, including all
// errors received while downloading object parts. If the download was successful,
// Attrs will be populated.
type DownloadOutput struct {
	Name  string                     // name of object
	Err   error                      // error occurring during download. Can use multi-error in Go 1.20+ if multiple failures.
	Attrs *storage.ReaderObjectAttrs // Attributes of downloaded object, if successful.
}

// DownloadOutputIterator allows the end user to iterate through completed
// object downloads.
type DownloadOutputIterator struct {
	// unexported fields including buffered DownloadOutputs
}

// Use this to iterate through results. When complete, will return error
// iterator.Done.
// DownloadOutputs will be available as the downloads complete; they can
// be iterated through asynchronously or at the end of the job.
func (it *DownloadOutputIterator) Next() (*DownloadOutput, error) {
	return nil, nil
}
