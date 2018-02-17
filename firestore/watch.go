// Copyright 2018 Google Inc. All Rights Reserved.
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

package firestore

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/golang/protobuf/ptypes"
	gax "github.com/googleapis/gax-go"
	"golang.org/x/net/context"
	pb "google.golang.org/genproto/googleapis/firestore/v1beta1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Implementation of realtime updates (a.k.a. watch).
// This code is closely based on the Node.js implementation,
// https://github.com/googleapis/nodejs-firestore/blob/master/src/watch.js.

// The sole target ID for all streams from this client.
const watchTargetID int32 = 'g' + 'o'

var defaultBackoff = gax.Backoff{
	// Values from https://github.com/googleapis/nodejs-firestore/blob/master/src/backoff.js.
	Initial:    1 * time.Second,
	Max:        60 * time.Second,
	Multiplier: 1.5,
}

// not goroutine-safe
type watchStream struct {
	ctx      context.Context
	c        *Client
	lc       pb.Firestore_ListenClient // the gRPC stream
	target   *pb.Target                // document or query being watched
	backoff  gax.Backoff               // for stream retries
	err      error                     // sticky permanent error
	readTime time.Time                 // time of most recent snapshot
	current  bool                      // saw CURRENT, but not RESET; precondition for a snapshot
	// Note: changeMap is trivial now (one entry), but will be more useful when query support is added.
	changeMap   map[string]*DocumentSnapshot // doc changes by name
	hasReturned bool                         // have we returned a snapshot yet?
}

func newWatchStreamForDocument(ctx context.Context, dr *DocumentRef) *watchStream {
	return newWatchStream(ctx, dr.Parent.c, &pb.Target{
		TargetType: &pb.Target_Documents{
			Documents: &pb.Target_DocumentsTarget{[]string{dr.Path}},
		},
		TargetId: watchTargetID,
	})
}

func newWatchStream(ctx context.Context, c *Client, target *pb.Target) *watchStream {
	return &watchStream{
		ctx:       ctx,
		c:         c,
		target:    target,
		backoff:   defaultBackoff,
		changeMap: map[string]*DocumentSnapshot{},
	}
}

// Once nextSnapshot returns an error, it will always return the same error.
func (s *watchStream) nextSnapshot() (*DocumentSnapshot, time.Time, error) {
	if s.err != nil {
		return nil, time.Time{}, s.err
	}
	for !s.handleNextMessage() {
	}
	if s.err != nil {
		_ = s.close() // ignore error
		return nil, time.Time{}, s.err
	}
	// changeMap should have only one item, which is the document snapshot we want to return.
	var snap *DocumentSnapshot
	for _, v := range s.changeMap {
		snap = v
		break
	}
	s.changeMap = map[string]*DocumentSnapshot{}
	s.hasReturned = true
	return snap, s.readTime, nil
}

// Read a message from the stream and handle it. Return true when
// we're in a consistent state, or there is a permanent error.
func (s *watchStream) handleNextMessage() bool {
	res, err := s.recv()
	if err != nil {
		s.err = err
		// Errors returned by recv are permanent.
		return true
	}
	switch r := res.ResponseType.(type) {
	case *pb.ListenResponse_TargetChange:
		return s.handleTargetChange(r.TargetChange)
	case *pb.ListenResponse_DocumentChange:
		name := r.DocumentChange.Document.Name
		if hasWatchTargetID(r.DocumentChange.TargetIds) { // document changed
			ref, err := pathToDoc(name, s.c)
			if err == nil {
				s.changeMap[name], err = newDocumentSnapshot(ref, r.DocumentChange.Document, s.c, nil)
			}
			if err != nil {
				s.err = err
				return true
			}
		} else if hasWatchTargetID(r.DocumentChange.RemovedTargetIds) { // document removed
			s.changeMap[name] = nil
		}
	case *pb.ListenResponse_DocumentDelete:
		s.changeMap[r.DocumentDelete.Document] = nil
	case *pb.ListenResponse_DocumentRemove:
		s.changeMap[r.DocumentRemove.Document] = nil
	case *pb.ListenResponse_Filter:
		panic("unimplemented")
	default:
		s.err = fmt.Errorf("unknown response type %T", r)
		return true
	}
	return false
}

func (s *watchStream) handleTargetChange(tc *pb.TargetChange) bool {
	switch tc.TargetChangeType {
	case pb.TargetChange_NO_CHANGE:
		if len(tc.TargetIds) == 0 && tc.ReadTime != nil && s.current {
			// Everything is up-to-date, so we are ready to return a snapshot.
			rt, err := ptypes.Timestamp(tc.ReadTime)
			if err != nil {
				s.err = err
				return true
			}
			s.readTime = rt
			s.target.ResumeType = &pb.Target_ResumeToken{tc.ResumeToken}
			// We only actually return the snapshot if something has changed (or this is the
			// first snapshot).
			return !s.hasReturned || len(s.changeMap) > 0
		}

	case pb.TargetChange_ADD:
		if tc.TargetIds[0] != watchTargetID {
			s.err = errors.New("unexpected target ID sent by server")
			return true
		}

	case pb.TargetChange_REMOVE:
		// We should never see a remove.
		if tc.Cause != nil {
			s.err = status.Error(codes.Code(tc.Cause.Code), tc.Cause.Message)
		} else {
			s.err = status.Error(codes.Internal, "firestore: internal error (client)")
		}
		return true

	// The targets reflect all changes committed before the targets were added
	// to the stream.
	case pb.TargetChange_CURRENT:
		s.current = true

	// The targets have been reset, and a new initial state for the targets will be
	// returned in subsequent changes. Whatever changes have happened so far no
	// longer matter.
	case pb.TargetChange_RESET:
		s.resetDocs()

	default:
		s.err = fmt.Errorf("firestore: unknown TargetChange type %s", tc.TargetChangeType)
		return true
	}
	// If we see a resume token and our watch ID is affected, we assume the stream
	// is now healthy, so we reset our backoff time to the minimum.
	if tc.ResumeToken != nil && (len(tc.TargetIds) == 0 || hasWatchTargetID(tc.TargetIds)) {
		s.backoff = defaultBackoff
	}
	return false // not in a consistent state, keep receiving
}

func (s *watchStream) resetDocs() {
	s.target.ResumeType = nil // clear resume token
	s.current = false
	s.changeMap = map[string]*DocumentSnapshot{}
}

func hasWatchTargetID(ids []int32) bool {
	for _, id := range ids {
		if id == watchTargetID {
			return true
		}
	}
	return false
}

// Close the stream. From this point on, calls to nextSnapshot will return
// io.EOF, or the error from CloseSend.
func (s *watchStream) stop() {
	err := s.close()
	if s.err != nil { // don't change existing error
		return
	}
	if err != nil {
		s.err = err
	}
	s.err = io.EOF // normal shutdown
}

func (s *watchStream) close() error {
	if s.lc == nil {
		return nil
	}
	return s.lc.CloseSend()
}

// recv receives the next message from the stream. It also handles opening the stream
// initially, and reopening it on non-permanent errors.
// recv doesn't have to be goroutine-safe.
func (s *watchStream) recv() (*pb.ListenResponse, error) {
	var err error
	for {
		if s.lc == nil {
			s.lc, err = s.open()
			if err != nil {
				// Do not retry if open fails.
				return nil, err
			}
		}
		res, err := s.lc.Recv()
		if err == nil || isPermanentWatchError(err) {
			return res, err
		}
		// Non-permanent error. Sleep and retry.
		s.changeMap = map[string]*DocumentSnapshot{} // clear changeMap
		dur := s.backoff.Pause()
		// If we're out of quota, wait a long time before retrying.
		if status.Code(err) == codes.ResourceExhausted {
			dur = s.backoff.Max
		}
		if err := gax.Sleep(s.ctx, dur); err != nil {
			return nil, err
		}
		s.lc = nil
	}
}

func (s *watchStream) open() (pb.Firestore_ListenClient, error) {
	dbPath := s.c.path()
	lc, err := s.c.c.Listen(withResourceHeader(s.ctx, dbPath))
	if err == nil {
		err = lc.Send(&pb.ListenRequest{
			Database:     dbPath,
			TargetChange: &pb.ListenRequest_AddTarget{AddTarget: s.target},
		})
	}
	if err != nil {
		return nil, err
	}
	return lc, nil
}

func isPermanentWatchError(err error) bool {
	if err == io.EOF {
		// Retry on normal end-of-stream.
		return false
	}
	switch status.Code(err) {
	case codes.Canceled, codes.Unknown, codes.DeadlineExceeded, codes.ResourceExhausted,
		codes.Internal, codes.Unavailable, codes.Unauthenticated:
		return false
	default:
		return true
	}
}
