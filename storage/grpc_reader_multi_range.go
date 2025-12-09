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

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	gax "github.com/googleapis/gax-go/v2"
)

// readIDGenerator generates unique read IDs for multi-range reads.
// Call readIDGenerator.Next to get the next ID. Safe to be called concurrently.
type readIDGenerator struct {
	initOnce sync.Once
	nextID   chan int64 // do not use this field directly
}

func (g *readIDGenerator) init() {
	g.nextID = make(chan int64, 1)
	g.nextID <- 1
}

// Next returns the Next read ID. It initializes the readIDGenerator if needed.
func (g *readIDGenerator) Next() int64 {
	g.initOnce.Do(g.init)

	id := <-g.nextID
	n := id + 1
	g.nextID <- n

	return id
}

// --- internalMultiRangeDownloader Interface ---
type internalMultiRangeDownloader interface {
	add(output io.Writer, offset, length int64, callback func(int64, int64, error))
	close(err error) error
	wait()
	getHandle(ctx context.Context) []byte
	getPermanentError() error
	getAttrs(ctx context.Context) (ReaderObjectAttrs, error)
	getSpanCtx() context.Context
}

// --- grpcStorageClient method ---

func (c *grpcStorageClient) NewMultiRangeDownloader(ctx context.Context, params *newMultiRangeDownloaderParams, opts ...storageOption) (*MultiRangeDownloader, error) {
	s := callSettings(c.settings, opts...)
	if s.userProject != "" {
		ctx = setUserProjectMetadata(ctx, s.userProject)
	}

	b := bucketResourceName(globalProjectAlias, params.bucket)
	readSpec := &storagepb.BidiReadObjectSpec{
		Bucket:                    b,
		Object:                    params.object,
		CommonObjectRequestParams: toProtoCommonObjectRequestParams(params.encryptionKey),
	}
	if params.gen >= 0 {
		readSpec.Generation = params.gen
	}
	if params.handle != nil && len(*params.handle) > 0 {
		readSpec.ReadHandle = &storagepb.BidiReadHandle{
			Handle: *params.handle,
		}
	}

	mCtx, cancel := context.WithCancel(ctx)

	// Create the manager
	manager := newMultiRangeDownloaderManager(mCtx, c, s, params, readSpec, cancel, ctx)

	mrd := &MultiRangeDownloader{
		impl:    manager,
		spanCtx: ctx,
	}
	manager.publicMRD = mrd

	manager.wg.Add(1)
	go func() {
		defer manager.wg.Done()
		manager.eventLoop()
	}()

	// Wait for attributes to be ready
	select {
	case <-manager.attrsReady:
		if manager.permanentErr != nil {
			cancel()
			manager.wg.Wait()
			return nil, manager.permanentErr
		}
		mrd.Attrs = manager.attrs
		return mrd, nil
	case <-ctx.Done():
		cancel()
		manager.wg.Wait()
		return nil, ctx.Err()
	}
}

// --- mrdCommand Interface and Implementations ---
type mrdCommand interface {
	apply(ctx context.Context, m *multiRangeDownloaderManager)
}
type mrdAddCmd struct {
	output   io.Writer
	offset   int64
	length   int64
	callback func(int64, int64, error)
}

func (c *mrdAddCmd) apply(ctx context.Context, m *multiRangeDownloaderManager) {
	m.handleAddCmd(ctx, c)
}

type mrdCloseCmd struct {
	err error
}

func (c *mrdCloseCmd) apply(ctx context.Context, m *multiRangeDownloaderManager) {
	m.handleCloseCmd(ctx, c)
}

type mrdWaitCmd struct {
	doneC chan struct{}
}

func (c *mrdWaitCmd) apply(ctx context.Context, m *multiRangeDownloaderManager) {
	m.handleWaitCmd(ctx, c)
}

type mrdGetHandleCmd struct {
	ctx   context.Context
	respC chan []byte
}

func (c *mrdGetHandleCmd) apply(ctx context.Context, m *multiRangeDownloaderManager) {
	select {
	case <-m.attrsReady:
		select {
		case c.respC <- m.lastReadHandle:
		case <-m.ctx.Done():
			close(c.respC)
		case <-c.ctx.Done():
			close(c.respC)
		}
	case <-m.ctx.Done():
		close(c.respC)
	case <-c.ctx.Done():
		close(c.respC)
	}
}

type mrdErrorCmd struct {
	respC chan error
}

func (c *mrdErrorCmd) apply(ctx context.Context, m *multiRangeDownloaderManager) {
	select {
	case c.respC <- m.permanentErr:
	case <-ctx.Done():
		close(c.respC)
	}
}

// --- mrdSessionResult ---
type mrdSessionResult struct {
	resp     *storagepb.BidiReadObjectResponse
	err      error
	redirect *storagepb.BidiReadObjectRedirectedError
}

var errClosed = errors.New("downloader closed")

// --- multiRangeDownloaderManager ---

type multiRangeDownloaderManager struct {
	ctx          context.Context
	cancel       context.CancelFunc
	client       *grpcStorageClient
	settings     *settings
	params       *newMultiRangeDownloaderParams
	wg           sync.WaitGroup
	cmdC         chan mrdCommand
	sessionRespC chan mrdSessionResult
	publicMRD    *MultiRangeDownloader

	// State
	currentSession *bidiReadStreamSession
	readIDCounter  int64
	pendingRanges  map[int64]*rangeRequest
	permanentErr   error
	waiters        []chan struct{}
	readSpec       *storagepb.BidiReadObjectSpec
	lastReadHandle []byte
	attrs          ReaderObjectAttrs
	attrsReady     chan struct{}
	attrsOnce      sync.Once
	spanCtx        context.Context
	callbackWg     sync.WaitGroup
}

type rangeRequest struct {
	output   io.Writer
	offset   int64
	length   int64
	callback func(int64, int64, error)

	origOffset int64
	origLength int64

	readID       int64
	bytesWritten int64
	completed    bool
}

func newMultiRangeDownloaderManager(ctx context.Context, client *grpcStorageClient, settings *settings, params *newMultiRangeDownloaderParams, readSpec *storagepb.BidiReadObjectSpec, cancel context.CancelFunc, spanCtx context.Context) *multiRangeDownloaderManager {
	return &multiRangeDownloaderManager{
		ctx:           ctx,
		cancel:        cancel,
		client:        client,
		settings:      settings,
		params:        params,
		cmdC:          make(chan mrdCommand, 1),
		sessionRespC:  make(chan mrdSessionResult, 100),
		pendingRanges: make(map[int64]*rangeRequest),
		readIDCounter: 1,
		readSpec:      readSpec,
		attrsReady:    make(chan struct{}),
		spanCtx:       spanCtx,
	}
}

// Methods implementing internalMultiRangeDownloader
func (m *multiRangeDownloaderManager) add(output io.Writer, offset, length int64, callback func(int64, int64, error)) {
	if err := m.getPermanentError(); err != nil {
		m.runCallback(offset, length, err, callback)
		return
	}
	if m.ctx.Err() != nil {
		m.runCallback(offset, length, m.ctx.Err(), callback)
		return
	}
	if length < 0 {
		m.runCallback(offset, length, fmt.Errorf("storage: MultiRangeDownloader.Add limit cannot be negative"), callback)
		return
	}

	cmd := &mrdAddCmd{output: output, offset: offset, length: length, callback: callback}
	select {
	case m.cmdC <- cmd:
	case <-m.ctx.Done():
		m.runCallback(offset, length, m.ctx.Err(), callback)
	}
}

func (m *multiRangeDownloaderManager) close(err error) error {
	cmd := &mrdCloseCmd{err: err}
	select {
	case m.cmdC <- cmd:
		<-m.ctx.Done()
		m.wg.Wait()
		if m.permanentErr != nil && !errors.Is(m.permanentErr, errClosed) {
			return m.permanentErr
		}
		return nil
	case <-m.ctx.Done():
		m.wg.Wait()
		return m.ctx.Err()
	}
}

func (m *multiRangeDownloaderManager) wait() {
	doneC := make(chan struct{})
	cmd := &mrdWaitCmd{doneC: doneC}
	select {
	case m.cmdC <- cmd:
		select {
		case <-doneC:
			m.callbackWg.Wait()
			return
		case <-m.ctx.Done():
			m.callbackWg.Wait()
			return
		}
	case <-m.ctx.Done():
		m.callbackWg.Wait()
		return
	}
}

func (m *multiRangeDownloaderManager) getHandle(ctx context.Context) []byte {
	select {
	case <-m.attrsReady:
	case <-m.ctx.Done():
		return nil
	case <-ctx.Done():
		return nil
	}

	respC := make(chan []byte, 1)
	cmd := &mrdGetHandleCmd{ctx: ctx, respC: respC}
	select {
	case m.cmdC <- cmd:
		select {
		case h, ok := <-respC:
			if !ok {
				return nil
			}
			return h
		case <-m.ctx.Done():
			return nil
		case <-ctx.Done():
			return nil
		}
	case <-m.ctx.Done():
		return nil
	case <-ctx.Done():
		return nil
	}
}

func (m *multiRangeDownloaderManager) getPermanentError() error {
	return m.permanentErr
}

func (m *multiRangeDownloaderManager) getAttrs(ctx context.Context) (ReaderObjectAttrs, error) {
	select {
	case <-m.attrsReady:
		if m.permanentErr != nil {
			return ReaderObjectAttrs{}, m.permanentErr
		}
		return m.attrs, nil
	case <-m.ctx.Done():
		return ReaderObjectAttrs{}, m.ctx.Err()
	case <-ctx.Done():
		return ReaderObjectAttrs{}, ctx.Err()
	}
}

func (m *multiRangeDownloaderManager) getSpanCtx() context.Context {
	return m.spanCtx
}

func (m *multiRangeDownloaderManager) runCallback(origOffset, numBytes int64, err error, cb func(int64, int64, error)) {
	m.callbackWg.Add(1)
	go func() {
		defer m.callbackWg.Done()
		cb(origOffset, numBytes, err)
	}()
}

func (m *multiRangeDownloaderManager) eventLoop() {
	defer func() {
		if m.currentSession != nil {
			m.currentSession.Shutdown()
		}
		finalErr := m.permanentErr
		if finalErr == nil {
			if ctxErr := m.ctx.Err(); ctxErr != nil {
				finalErr = ctxErr
			}
		}
		if finalErr == nil {
			finalErr = errClosed
		}
		m.failAllPending(finalErr)
		for _, waiter := range m.waiters {
			close(waiter)
		}
		m.attrsOnce.Do(func() { close(m.attrsReady) })
		m.callbackWg.Wait()
	}()

	// Blocking call to establish the first session and get attributes.
	if err := m.establishInitialSession(); err != nil {
		// permanentErr is set within establishInitialSession if necessary.
		return // Exit eventLoop if we can't start.
	}

	for {
		select {
		case <-m.ctx.Done():
			return
		case cmd := <-m.cmdC:
			cmd.apply(m.ctx, m)
			if _, ok := cmd.(*mrdCloseCmd); ok {
				return
			}
		case result := <-m.sessionRespC:
			m.processSessionResult(result)
		}

		if len(m.pendingRanges) == 0 {
			for _, waiter := range m.waiters {
				close(waiter)
			}
			m.waiters = nil
		}
	}
}

func (m *multiRangeDownloaderManager) establishInitialSession() error {
	retry := m.settings.retry
	if retry == nil {
		retry = defaultRetry
	}

	err := run(m.ctx, func(ctx context.Context) error {
		if m.currentSession != nil {
			m.currentSession.Shutdown()
			m.currentSession = nil
		}

		session, err := newBidiReadStreamSession(m.ctx, m.sessionRespC, m.client, m.settings, m.params, proto.Clone(m.readSpec).(*storagepb.BidiReadObjectSpec))
		if err != nil {
			redirectErr, isRedirect := isRedirectError(err)
			if isRedirect {
				m.readSpec.RoutingToken = redirectErr.RoutingToken
				m.readSpec.ReadHandle = redirectErr.ReadHandle
				return fmt.Errorf("%w: %v", errBidiReadRedirect, err)
			}
			return err
		}
		m.currentSession = session

		// Wait for the first message to populate attributes
		select {
		case firstResult := <-m.sessionRespC:
			if firstResult.err != nil {
				m.currentSession.Shutdown()
				m.currentSession = nil
				// Pass the error back to run() to potentially retry
				return firstResult.err
			}
			// Process the first response to set attributes
			m.processSessionResult(firstResult)
			if m.permanentErr != nil {
				return m.permanentErr
			}
			return nil // Success
		case <-m.ctx.Done():
			return m.ctx.Err()
		}
	}, retry, true)

	if err != nil {
		if !m.isRetryable(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			if m.permanentErr == nil {
				m.permanentErr = err
			}
			m.attrsOnce.Do(func() { close(m.attrsReady) })
		}
	}
	return err
}

func (m *multiRangeDownloaderManager) handleAddCmd(ctx context.Context, cmd *mrdAddCmd) {
	if m.permanentErr != nil {
		m.runCallback(cmd.offset, cmd.length, m.permanentErr, cmd.callback)
		return
	}

	req := &rangeRequest{
		output:     cmd.output,
		offset:     cmd.offset,
		length:     cmd.length,
		origOffset: cmd.offset,
		origLength: cmd.length,
		callback:   cmd.callback,
		readID:     m.readIDCounter,
	}
	m.readIDCounter++

	// Attributes should be ready if we are processing Add commands
	if req.offset < 0 {
		m.convertToPositiveOffset(req)
	}
	m.pendingRanges[req.readID] = req

	if m.currentSession == nil {
		// This should not happen if establishInitialSession was successful
		m.failRange(req, errors.New("storage: session not available"))
		return
	}

	protoReq := &storagepb.BidiReadObjectRequest{
		ReadRanges: []*storagepb.ReadRange{{
			ReadOffset: req.offset,
			ReadLength: req.length,
			ReadId:     req.readID,
		}},
	}
	m.currentSession.SendRequest(protoReq)
}

func (m *multiRangeDownloaderManager) convertToPositiveOffset(req *rangeRequest) {
	if req.offset >= 0 {
		return
	}
	objSize := m.attrs.Size
	if objSize <= 0 {
		m.failRange(req, errors.New("storage: cannot resolve negative offset without object size"))
		return
	}
	if req.length != 0 {
		m.failRange(req, fmt.Errorf("storage: negative offset with non-zero length is not supported (offset: %d, length: %d)", req.origOffset, req.origLength))
		return
	}
	start := objSize + req.offset
	if start < 0 {
		start = 0
	}
	req.offset = start
	req.length = objSize - start
}

func (m *multiRangeDownloaderManager) handleCloseCmd(ctx context.Context, cmd *mrdCloseCmd) {
	if m.permanentErr == nil {
		m.permanentErr = cmd.err
		if m.permanentErr == nil {
			m.permanentErr = errClosed
		}
	}
	m.attrsOnce.Do(func() { close(m.attrsReady) })
	m.cancel()
}

func (m *multiRangeDownloaderManager) handleWaitCmd(ctx context.Context, cmd *mrdWaitCmd) {
	if len(m.pendingRanges) == 0 {
		close(cmd.doneC)
	} else {
		m.waiters = append(m.waiters, cmd.doneC)
	}
}

func (m *multiRangeDownloaderManager) processSessionResult(result mrdSessionResult) {
	if result.err != nil {
		m.handleStreamEnd(result)
		return
	}

	resp := result.resp
	if handle := resp.GetReadHandle().GetHandle(); len(handle) > 0 {
		m.lastReadHandle = handle
		if m.params.handle != nil {
			*m.params.handle = handle
		}
	}

	m.attrsOnce.Do(func() {
		if meta := resp.GetMetadata(); meta != nil {
			obj := newObjectFromProto(meta)
			attrs := readerAttrsFromObject(obj)
			m.attrs = attrs
			close(m.attrsReady)

			for _, req := range m.pendingRanges {
				if req.offset < 0 {
					m.convertToPositiveOffset(req)
				}
			}
		} else {
			m.handleStreamEnd(mrdSessionResult{err: errors.New("storage: first response from BidiReadObject stream missing metadata")})
		}
	})

	for _, dataRange := range resp.GetObjectDataRanges() {
		readID := dataRange.GetReadRange().GetReadId()
		req, exists := m.pendingRanges[readID]
		if !exists || req.completed {
			continue
		}

		content := dataRange.GetChecksummedData().GetContent()
		req.bytesWritten += int64(len(content))
		_, err := req.output.Write(content)
		if err != nil {
			m.failRange(req, err)
			continue
		}

		if dataRange.GetRangeEnd() {
			req.completed = true
			delete(m.pendingRanges, req.readID)
			m.runCallback(req.origOffset, req.bytesWritten, nil, req.callback)
		}
	}
}

// ensureSession is now only for reconnecting *after* the initial session is up.
func (m *multiRangeDownloaderManager) ensureSession(ctx context.Context) error {
	if m.currentSession != nil {
		return nil
	}
	if m.permanentErr != nil {
		return m.permanentErr
	}

	// Using run for retries
	return run(ctx, func(ctx context.Context) error {
		if m.currentSession != nil {
			return nil
		}
		if m.permanentErr != nil {
			return m.permanentErr
		}

		session, err := newBidiReadStreamSession(m.ctx, m.sessionRespC, m.client, m.settings, m.params, proto.Clone(m.readSpec).(*storagepb.BidiReadObjectSpec))
		if err != nil {
			redirectErr, isRedirect := isRedirectError(err)
			if isRedirect {
				m.readSpec.RoutingToken = redirectErr.RoutingToken
				m.readSpec.ReadHandle = redirectErr.ReadHandle
				return fmt.Errorf("%w: %v", errBidiReadRedirect, err)
			}
			return err
		}
		m.currentSession = session

		var rangesToResend []*storagepb.ReadRange
		for _, req := range m.pendingRanges {
			if !req.completed {
				readLength := req.length
				if req.length > 0 {
					readLength -= req.bytesWritten
				}
				if readLength < 0 {
					readLength = 0
				}

				if req.length == 0 || readLength > 0 {
					rangesToResend = append(rangesToResend, &storagepb.ReadRange{
						ReadOffset: req.offset + req.bytesWritten,
						ReadLength: readLength,
						ReadId:     req.readID,
					})
				}
			}
		}
		if len(rangesToResend) > 0 {
			m.currentSession.SendRequest(&storagepb.BidiReadObjectRequest{ReadRanges: rangesToResend})
		}
		return nil
	}, m.settings.retry, true)
}

var errBidiReadRedirect = errors.New("bidi read object redirected")

func (m *multiRangeDownloaderManager) handleStreamEnd(result mrdSessionResult) {
	m.currentSession = nil
	err := result.err

	if result.redirect != nil {
		m.readSpec.RoutingToken = result.redirect.RoutingToken
		m.readSpec.ReadHandle = result.redirect.ReadHandle
		if ensureErr := m.ensureSession(m.ctx); ensureErr != nil {
			if !m.isRetryable(ensureErr) {
				m.permanentErr = ensureErr
				m.attrsOnce.Do(func() { close(m.attrsReady) })
				m.failAllPending(m.permanentErr)
			}
		}
	} else if m.isRetryable(err) {
		if len(m.pendingRanges) > 0 {
			if ensureErr := m.ensureSession(m.ctx); ensureErr != nil {
				if !m.isRetryable(ensureErr) {
					m.permanentErr = ensureErr
					m.attrsOnce.Do(func() { close(m.attrsReady) })
					m.failAllPending(m.permanentErr)
				}
			}
		}
	} else {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, errClosed) {
			if m.permanentErr == nil {
				m.permanentErr = err
			}
		} else if m.permanentErr == nil {
			m.permanentErr = errClosed
		}
		m.failAllPending(m.permanentErr)
		m.attrsOnce.Do(func() { close(m.attrsReady) })
	}
}

func (m *multiRangeDownloaderManager) isRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, errClosed) || err == io.EOF {
		return false
	}
	if errors.Is(err, errBidiReadRedirect) {
		return true
	}
	s, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch s.Code() {
	case codes.Unavailable, codes.ResourceExhausted, codes.Internal, codes.DeadlineExceeded:
		return true
	case codes.Aborted:
		_, isRedirect := isRedirectError(err)
		return isRedirect
	default:
		return false
	}
}

func (m *multiRangeDownloaderManager) failRange(req *rangeRequest, err error) {
	if req.completed {
		return
	}
	req.completed = true
	delete(m.pendingRanges, req.readID)
	m.runCallback(req.origOffset, req.bytesWritten, err, req.callback)
}

func (m *multiRangeDownloaderManager) failAllPending(err error) {
	for _, req := range m.pendingRanges {
		if !req.completed {
			req.completed = true
			m.runCallback(req.origOffset, req.bytesWritten, err, req.callback)
		}
	}
	m.pendingRanges = make(map[int64]*rangeRequest)
}

// --- bidiReadStreamSession ---
type bidiReadStreamSession struct {
	ctx    context.Context
	cancel context.CancelFunc

	stream   storagepb.Storage_BidiReadObjectClient
	client   *grpcStorageClient
	settings *settings
	params   *newMultiRangeDownloaderParams
	readSpec *storagepb.BidiReadObjectSpec

	reqC  chan *storagepb.BidiReadObjectRequest
	respC chan<- mrdSessionResult
	wg    sync.WaitGroup

	errOnce   sync.Once
	streamErr error
}

func newBidiReadStreamSession(ctx context.Context, respC chan<- mrdSessionResult, client *grpcStorageClient, settings *settings, params *newMultiRangeDownloaderParams, readSpec *storagepb.BidiReadObjectSpec) (*bidiReadStreamSession, error) {
	sCtx, cancel := context.WithCancel(ctx)

	s := &bidiReadStreamSession{
		ctx:      sCtx,
		cancel:   cancel,
		client:   client,
		settings: settings,
		params:   params,
		readSpec: readSpec,
		reqC:     make(chan *storagepb.BidiReadObjectRequest, 100),
		respC:    respC,
	}

	initialReq := &storagepb.BidiReadObjectRequest{
		ReadObjectSpec: s.readSpec,
	}
	reqCtx := gax.InsertMetadataIntoOutgoingContext(s.ctx, contextMetadataFromBidiReadObject(initialReq)...)

	var err error
	s.stream, err = client.raw.BidiReadObject(reqCtx, s.settings.gax...)
	if err != nil {
		cancel()
		return nil, err
	}

	if err := s.stream.Send(initialReq); err != nil {
		s.stream.CloseSend()
		cancel()
		return nil, err
	}

	s.wg.Add(2)
	go s.sendLoop()
	go s.receiveLoop()

	go func() {
		s.wg.Wait()
		s.cancel()
	}()

	return s, nil
}
func (s *bidiReadStreamSession) SendRequest(req *storagepb.BidiReadObjectRequest) {
	select {
	case s.reqC <- req:
	case <-s.ctx.Done():
	}
}
func (s *bidiReadStreamSession) Shutdown() {
	s.cancel()
	s.wg.Wait()
}
func (s *bidiReadStreamSession) setError(err error) {
	s.errOnce.Do(func() {
		s.streamErr = err
	})
}
func (s *bidiReadStreamSession) sendLoop() {
	defer s.wg.Done()
	defer s.stream.CloseSend()
	for {
		select {
		case req, ok := <-s.reqC:
			if !ok {
				return
			}
			if err := s.stream.Send(req); err != nil {
				s.setError(err)
				s.cancel()
				return
			}
		case <-s.ctx.Done():
			return
		}
	}
}
func (s *bidiReadStreamSession) receiveLoop() {
	defer s.wg.Done()
	defer s.cancel()
	for {
		if err := s.ctx.Err(); err != nil {
			return
		}

		resp, err := s.stream.Recv()
		if err != nil {
			redirectErr, isRedirect := isRedirectError(err)
			result := mrdSessionResult{err: err}
			if isRedirect {
				result.redirect = redirectErr
				err = fmt.Errorf("%w: %v", errBidiReadRedirect, err)
				result.err = err
			}
			s.setError(err)

			select {
			case s.respC <- result:
			case <-s.ctx.Done():
			}
			return
		}
		select {
		case s.respC <- mrdSessionResult{resp: resp}:
		case <-s.ctx.Done():
			return
		}
	}
}
func isRedirectError(err error) (*storagepb.BidiReadObjectRedirectedError, bool) {
	st, ok := status.FromError(err)
	if !ok {
		return nil, false
	}
	if st.Code() != codes.Aborted {
		return nil, false
	}
	for _, d := range st.Details() {
		if bidiError, ok := d.(*storagepb.BidiReadObjectRedirectedError); ok {
			if bidiError.RoutingToken != nil {
				return bidiError, true
			}
		}
	}
	return nil, false
}

func readerAttrsFromObject(o *ObjectAttrs) ReaderObjectAttrs {
	if o == nil {
		return ReaderObjectAttrs{}
	}
	return ReaderObjectAttrs{
		Size:            o.Size,
		ContentType:     o.ContentType,
		ContentEncoding: o.ContentEncoding,
		CacheControl:    o.CacheControl,
		LastModified:    o.Updated,
		Generation:      o.Generation,
		Metageneration:  o.Metageneration,
		CRC32C:          o.CRC32C,
	}
}
