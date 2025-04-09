// Copyright 2025 Google LLC
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

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"cloud.google.com/go/internal/trace"
	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/status"
)

func (c *grpcStorageClient) NewMultiRangeDownloader(ctx context.Context, params *newMultiRangeDownloaderParams, opts ...storageOption) (mr *MultiRangeDownloader, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/storage.grpcStorageClient.NewMultiRangeDownloader")
	defer func() { trace.EndSpan(ctx, err) }()
	s := callSettings(c.settings, opts...)

	if s.userProject != "" {
		ctx = setUserProjectMetadata(ctx, s.userProject)
	}

	b := bucketResourceName(globalProjectAlias, params.bucket)
	object := params.object
	bidiObject := &storagepb.BidiReadObjectSpec{
		Bucket:                    b,
		Object:                    object,
		CommonObjectRequestParams: toProtoCommonObjectRequestParams(params.encryptionKey),
	}

	// The default is a negative value, which means latest.
	if params.gen >= 0 {
		bidiObject.Generation = params.gen
	}

	if params.handle != nil && len(*params.handle) != 0 {
		bidiObject.ReadHandle = &storagepb.BidiReadHandle{
			Handle: *params.handle,
		}
	}
	req := &storagepb.BidiReadObjectRequest{
		ReadObjectSpec: bidiObject,
	}

	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, contextMetadataFromBidiReadObject(req)...)

	openStream := func(readHandle ReadHandle) (*bidiReadStreamResponse, context.CancelFunc, error) {
		if err := applyCondsProto("grpcStorageClient.BidiReadObject", params.gen, params.conds, bidiObject); err != nil {
			return nil, nil, err
		}
		if len(readHandle) != 0 {
			req.GetReadObjectSpec().ReadHandle = &storagepb.BidiReadHandle{
				Handle: readHandle,
			}
		}
		var stream storagepb.Storage_BidiReadObjectClient
		var resp *storagepb.BidiReadObjectResponse
		cc, cancel := context.WithCancel(ctx)
		err = run(cc, func(ctx context.Context) error {
			stream, err = c.raw.BidiReadObject(ctx, s.gax...)
			if err != nil {
				// BidiReadObjectRedirectedError error is only returned on initial open in case of a redirect.
				// The routing token that should be used when reopening the read stream. Needs to be exported.
				rpcStatus := status.Convert(err)
				details := rpcStatus.Details()
				for _, detail := range details {
					if bidiError, ok := detail.(*storagepb.BidiReadObjectRedirectedError); ok {
						bidiObject.ReadHandle = bidiError.ReadHandle
						bidiObject.RoutingToken = bidiError.RoutingToken
						req.ReadObjectSpec = bidiObject
						ctx = gax.InsertMetadataIntoOutgoingContext(ctx, contextMetadataFromBidiReadObject(req)...)
					}
				}
				return err
			}
			// Incase stream opened succesfully, send first message on the stream.
			// First message to stream should contain read_object_spec
			err = stream.Send(req)
			if err != nil {
				return err
			}
			resp, err = stream.Recv()
			if err != nil {
				return err
			}
			return nil
		}, s.retry, s.idempotent)
		if err != nil {
			// Close the stream context we just created to ensure we don't leak
			// resources.
			cancel()
			return nil, nil, err
		}
		return &bidiReadStreamResponse{stream: stream, response: resp}, cancel, nil
	}

	// For the first time open stream without adding any range.
	resp, cancel, err := openStream(nil)
	if err != nil {
		return nil, err
	}

	// The first message was Recv'd on stream open, use it to populate the
	// object metadata.
	msg := resp.response
	obj := msg.GetMetadata()
	// This is the size of the entire object, even if only a range was requested.
	size := obj.GetSize()

	mrd := &gRPCBidiReader{
		stream:           resp.stream,
		cancel:           cancel,
		settings:         s,
		readHandle:       msg.GetReadHandle().GetHandle(),
		readIDGenerator:  &readIDGenerator{},
		reopen:           openStream,
		readSpec:         bidiObject,
		rangesToRead:     make(chan []mrdRange, 100),
		ctx:              ctx,
		closeReceiver:    make(chan bool, 10),
		closeSender:      make(chan bool, 10),
		senderRetry:      make(chan bool), // create unbuffered channel for closing the streamManager goroutine.
		receiverRetry:    make(chan bool), // create unbuffered channel for closing the streamReceiver goroutine.
		activeRanges:     make(map[int64]mrdRange),
		done:             false,
		numActiveRanges:  0,
		streamRecreation: false,
	}

	// sender receives ranges from user adds and requests these ranges from GCS.
	sender := func() {
		var currentSpec []mrdRange
		for {
			select {
			case <-mrd.ctx.Done():
				mrd.mu.Lock()
				mrd.done = true
				mrd.mu.Unlock()
				return
			case <-mrd.senderRetry:
				return
			case <-mrd.closeSender:
				mrd.mu.Lock()
				if len(mrd.activeRanges) != 0 {
					for key := range mrd.activeRanges {
						mrd.activeRanges[key].callback(mrd.activeRanges[key].offset, mrd.activeRanges[key].totalBytesWritten, fmt.Errorf("stream closed early"))
						delete(mrd.activeRanges, key)
					}
				}
				mrd.numActiveRanges = 0
				mrd.mu.Unlock()
				return
			case currentSpec = <-mrd.rangesToRead:
				var readRanges []*storagepb.ReadRange
				var err error
				mrd.mu.Lock()
				for _, v := range currentSpec {
					mrd.activeRanges[v.readID] = v
					readRanges = append(readRanges, &storagepb.ReadRange{ReadOffset: v.offset, ReadLength: v.limit, ReadId: v.readID})
				}
				mrd.mu.Unlock()
				// We can just send 100 request to gcs in one request.
				// In case of Add we will send only one range request to gcs but in case of retry we can have more than 100 ranges.
				// Hence be will divide the request in chunk of 100.
				// For example with 457 ranges on stream we will have 5 request to gcs [0:99], [100:199], [200:299], [300:399], [400:456]
				requestCount := len(readRanges) / 100
				if len(readRanges)%100 != 0 {
					requestCount++
				}
				for i := 0; i < requestCount; i++ {
					start := i * 100
					end := (i + 1) * 100
					if end > len(readRanges) {
						end = len(readRanges)
					}
					curReq := readRanges[start:end]
					err = mrd.stream.Send(&storagepb.BidiReadObjectRequest{
						ReadRanges: curReq,
					})
					if err != nil {
						// cancel stream and reopen the stream again.
						// Incase again an error is thrown close the streamManager goroutine.
						mrd.retrier(err, "manager")
						break
					}
				}

			}
		}
	}

	// receives ranges responses on the stream and executes the callback.
	receiver := func() {
		var resp *storagepb.BidiReadObjectResponse
		var err error
		for {
			select {
			case <-mrd.ctx.Done():
				mrd.done = true
				return
			case <-mrd.receiverRetry:
				return
			case <-mrd.closeReceiver:
				return
			default:
				// This function reads the data sent for a particular range request and has a callback
				// to indicate that output buffer is filled.
				resp, err = mrd.stream.Recv()
				if resp.GetReadHandle().GetHandle() != nil {
					mrd.readHandle = resp.GetReadHandle().GetHandle()
				}
				if err == io.EOF {
					err = nil
				}
				if err != nil {
					// cancel stream and reopen the stream again.
					// Incase again an error is thrown close the streamManager goroutine.
					mrd.retrier(err, "receiver")
				}

				if err == nil {
					mrd.mu.Lock()
					if len(mrd.activeRanges) == 0 && mrd.numActiveRanges == 0 {
						mrd.closeReceiver <- true
						mrd.closeSender <- true
						return
					}
					mrd.mu.Unlock()
					arr := resp.GetObjectDataRanges()
					for _, val := range arr {
						id := val.GetReadRange().GetReadId()
						mrd.mu.Lock()
						_, ok := mrd.activeRanges[id]
						if !ok {
							// it's ok to ignore responses for read_id not in map as user would have been notified by callback.
							continue
						}
						_, err = mrd.activeRanges[id].writer.Write(val.GetChecksummedData().GetContent())
						if err != nil {
							mrd.activeRanges[id].callback(mrd.activeRanges[id].offset, mrd.activeRanges[id].totalBytesWritten, err)
							mrd.numActiveRanges--
							delete(mrd.activeRanges, id)
						} else {
							mrd.activeRanges[id] = mrdRange{
								readID:              mrd.activeRanges[id].readID,
								writer:              mrd.activeRanges[id].writer,
								offset:              mrd.activeRanges[id].offset,
								limit:               mrd.activeRanges[id].limit,
								currentBytesWritten: mrd.activeRanges[id].currentBytesWritten + int64(len(val.GetChecksummedData().GetContent())),
								totalBytesWritten:   mrd.activeRanges[id].totalBytesWritten + int64(len(val.GetChecksummedData().GetContent())),
								callback:            mrd.activeRanges[id].callback,
							}
						}
						if val.GetRangeEnd() {
							mrd.activeRanges[id].callback(mrd.activeRanges[id].offset, mrd.activeRanges[id].totalBytesWritten, nil)
							mrd.numActiveRanges--
							delete(mrd.activeRanges, id)
						}
						mrd.mu.Unlock()
					}
				}
			}
		}
	}

	mrd.retrier = func(err error, thread string) {
		mrd.mu.Lock()
		if !mrd.streamRecreation {
			mrd.streamRecreation = true
		} else {
			mrd.mu.Unlock()
			return
		}
		mrd.mu.Unlock()
		// close both the go routines to make the stream recreation syncronous.
		if thread == "receiver" {
			mrd.senderRetry <- true
		} else {
			mrd.receiverRetry <- true
		}
		err = mrd.retryStream(err)
		if err != nil {
			mrd.mu.Lock()
			for key := range mrd.activeRanges {
				mrd.activeRanges[key].callback(mrd.activeRanges[key].offset, mrd.activeRanges[key].totalBytesWritten, err)
				delete(mrd.activeRanges, key)
			}
			// In case we hit an permanent error, delete entries from map and remove active tasks.
			mrd.numActiveRanges = 0
			mrd.mu.Unlock()
			mrd.close()
		} else {
			// If stream recreation happened successfully lets again start
			// both the goroutine making the whole flow asynchronous again.
			if thread == "receiver" {
				go sender()
			} else {
				go receiver()
			}
		}
		mrd.mu.Lock()
		mrd.streamRecreation = false
		mrd.mu.Unlock()
	}

	mrd.mu.Lock()
	mrd.objectSize = size
	mrd.mu.Unlock()

	go sender()
	go receiver()

	return &MultiRangeDownloader{
		Attrs: ReaderObjectAttrs{
			Size:            size,
			ContentType:     obj.GetContentType(),
			ContentEncoding: obj.GetContentEncoding(),
			CacheControl:    obj.GetCacheControl(),
			LastModified:    obj.GetUpdateTime().AsTime(),
			Metageneration:  obj.GetMetageneration(),
			Generation:      obj.GetGeneration(),
		},
		reader: mrd,
	}, nil
}

type gRPCBidiReader struct {
	ctx             context.Context
	stream          storagepb.Storage_BidiReadObjectClient
	cancel          context.CancelFunc
	settings        *settings
	readHandle      ReadHandle
	readIDGenerator *readIDGenerator
	reopen          func(ReadHandle) (*bidiReadStreamResponse, context.CancelFunc, error)
	readSpec        *storagepb.BidiReadObjectSpec
	objectSize      int64 // always use the mutex when accessing this variable
	closeReceiver   chan bool
	closeSender     chan bool
	senderRetry     chan bool
	receiverRetry   chan bool
	// rangesToRead are ranges that have not yet been sent or have been sent but
	// must be retried.
	rangesToRead chan []mrdRange
	// activeRanges are ranges that are currently being sent or are waiting for
	// a response from GCS.
	activeRanges     map[int64]mrdRange // always use the mutex when accessing the map
	numActiveRanges  int64              // always use the mutex when accessing this variable
	done             bool               // always use the mutex when accessing this variable, indicates whether stream is closed or not.
	mu               sync.Mutex         // protects all vars in gRPCBidiReader from concurrent access
	retrier          func(error, string)
	streamRecreation bool // This helps us identify if stream recreation is in progress or not. If stream recreation gets called from two goroutine then this will stop second one.
}

func (mrd *gRPCBidiReader) activeRange() []mrdRange {
	mrd.mu.Lock()
	defer mrd.mu.Unlock()
	var activeRange []mrdRange
	for k, v := range mrd.activeRanges {
		activeRange = append(activeRange, mrdRange{
			readID:              k,
			writer:              v.writer,
			offset:              (v.offset + v.currentBytesWritten),
			limit:               v.limit - v.currentBytesWritten,
			callback:            v.callback,
			currentBytesWritten: 0,
			totalBytesWritten:   v.totalBytesWritten,
		})
		mrd.activeRanges[k] = activeRange[len(activeRange)-1]
	}
	return activeRange
}

// retryStream cancel's stream and reopen the stream again.
func (mrd *gRPCBidiReader) retryStream(err error) error {
	var shouldRetry = ShouldRetry
	if mrd.settings.retry != nil && mrd.settings.retry.shouldRetry != nil {
		shouldRetry = mrd.settings.retry.shouldRetry
	}
	if shouldRetry(err) {
		// This will "close" the existing stream and immediately attempt to
		// reopen the stream, but will backoff if further attempts are necessary.
		// When Reopening the stream only failed readID will be added to stream.
		return mrd.reopenStream(mrd.activeRange())
	}
	return err
}

// reopenStream "closes" the existing stream and attempts to reopen a stream and
// sets the Reader's stream and cancelStream properties in the process.
func (mrd *gRPCBidiReader) reopenStream(failSpec []mrdRange) error {
	// Close existing stream and initialize new stream with updated offset.
	if mrd.cancel != nil {
		mrd.cancel()
	}

	res, cancel, err := mrd.reopen(mrd.readHandle)
	if err != nil {
		return err
	}
	mrd.stream = res.stream
	mrd.cancel = cancel
	mrd.readHandle = res.response.GetReadHandle().GetHandle()
	if failSpec != nil {
		mrd.rangesToRead <- failSpec
	}
	return nil
}

// Add will add current range to stream.
func (mrd *gRPCBidiReader) add(output io.Writer, offset, limit int64, callback func(int64, int64, error)) {
	mrd.mu.Lock()
	objectSize := mrd.objectSize
	mrd.mu.Unlock()

	if offset > objectSize {
		callback(offset, 0, fmt.Errorf("storage: offset should not be larger than size of object (%v)", objectSize))
		return
	}
	if limit < 0 {
		callback(offset, 0, errors.New("storage: cannot add range because the limit cannot be negative"))
		return
	}
	id := mrd.readIDGenerator.Next()
	mrd.mu.Lock()
	if !mrd.done {
		spec := mrdRange{readID: id, writer: output, offset: offset, limit: limit, currentBytesWritten: 0, totalBytesWritten: 0, callback: callback}
		mrd.numActiveRanges++
		mrd.rangesToRead <- []mrdRange{spec}
	} else {
		callback(offset, 0, errors.New("storage: cannot add range because the stream is closed"))
	}
	mrd.mu.Unlock()
}

func (mrd *gRPCBidiReader) wait() {
	mrd.mu.Lock()
	// we should wait until there is active task or an entry in the map.
	// there can be a scenario we have nothing in map for a moment or too but still have active task.
	// hence in case we have permanent errors we reduce active task to 0 so that this does not block wait.
	keepWaiting := len(mrd.activeRanges) != 0 || mrd.numActiveRanges != 0
	mrd.mu.Unlock()

	for keepWaiting {
		mrd.mu.Lock()
		keepWaiting = len(mrd.activeRanges) != 0 || mrd.numActiveRanges != 0
		mrd.mu.Unlock()
	}
}

// Close will notify stream manager goroutine that the reader has been closed, if it's still running.
func (mrd *gRPCBidiReader) close() error {
	if mrd.cancel != nil {
		mrd.cancel()
	}
	mrd.mu.Lock()
	mrd.done = true
	mrd.numActiveRanges = 0
	mrd.mu.Unlock()
	mrd.closeReceiver <- true
	mrd.closeSender <- true
	return nil
}

func (mrd *gRPCBidiReader) getHandle() []byte {
	return mrd.readHandle
}

func (mrd *gRPCBidiReader) error() error {
	mrd.mu.Lock()
	defer mrd.mu.Unlock()
	if mrd.done {
		return errors.New("storage: stream is permanently closed")
	}
	return nil
}

func contextMetadataFromBidiReadObject(req *storagepb.BidiReadObjectRequest) []string {
	if len(req.GetReadObjectSpec().GetRoutingToken()) > 0 {
		return []string{"x-goog-request-params", fmt.Sprintf("bucket=%s&routing_token=%s", req.GetReadObjectSpec().GetBucket(), req.GetReadObjectSpec().GetRoutingToken())}
	}
	return []string{"x-goog-request-params", fmt.Sprintf("bucket=%s", req.GetReadObjectSpec().GetBucket())}
}

type mrdRange struct {
	readID              int64
	writer              io.Writer
	offset              int64
	limit               int64
	currentBytesWritten int64
	totalBytesWritten   int64
	callback            func(int64, int64, error)
}

// readIDGenerator generates unique read IDs for multi-range reads.
// Call readIDGenerator.Next to get the next ID. Safe to be called concurrently.
type readIDGenerator struct {
	initOnce sync.Once
	nextId   chan int64
}

func (g *readIDGenerator) init() {
	g.nextId = make(chan int64, 1)
	g.nextId <- 1
}

// Next returns the Next read ID. It initializes the readIDGenerator if needed.
func (g *readIDGenerator) Next() int64 {
	g.initOnce.Do(g.init)

	id := <-g.nextId
	n := id + 1
	g.nextId <- n

	return id
}
