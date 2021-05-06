// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package managedwriter

// WriterOption enables the use of the option pattern when constructing writers.
type WriterOption func(*ManagedWriter)

// WithType sets the write type of the new writer.
func WithType(st StreamType) WriterOption {
	return func(mw *ManagedWriter) {
		mw.settings.StreamType = st
	}
}

// WithMaxInflightRequests bounds the inflight appends on the write connection.
func WithMaxInflightRequests(n int) WriterOption {
	return func(mw *ManagedWriter) {
		mw.settings.MaxInflightRequests = n
	}
}

// WithMaxInflightBytes bounds the inflight append request bytes on the write connection.
func WithMaxInflightBytes(n int) WriterOption {
	return func(mw *ManagedWriter) {
		mw.settings.MaxInflightBytes = n
	}
}

// WithRowSerializer registers a specific RowSerializer, responsible for converting/describing rows
// when transmitting to the backend.
func WithRowSerializer(rs RowSerializer) WriterOption {
	return func(mw *ManagedWriter) {
		mw.settings.Serializer = rs
	}
}

func WithTracePrefix(prefix string) WriterOption {
	return func(mw *ManagedWriter) {
		mw.settings.TracePrefix = prefix
	}
}

// To Consider:
// WithClientOptions(opts ...option.ClientOption)
// WithSchemaChangeNotification(f func(offset int64,schema *storagepb.TableSchema)))

// StreamType governs how appending data to tables works, and
// when that data is made visible to readers within BigQuery.
//
// Note that for streams that allow data to be uncommitted,
// BigQuery does enforce a per-project limit shared by all tables
// in the project.
type StreamType string

var (
	// DefaultStream most closely mimics the legacy bigquery
	// tabledata.insertAll semantics.  Successful inserts are
	// committed immediately, and there's no tracking state as
	// all writes go into a "default" stream that allows multiple
	// connections.
	DefaultStream StreamType = "DEFAULT"

	// CommittedStream appends data immediately, but creates a
	// discrete stream for the work so that offset tracking can
	// be used to track writes.
	CommittedStream StreamType = "COMMITTED"

	// BufferedStream is a form of checkpointed stream, that allows
	// you to advance the offset of visible rows via Flush operations.
	BufferedStream StreamType = "BUFFERED"

	// PendingStream is a stream in which no data is made visible to
	// readers until the stream is finalized.
	PendingStream StreamType = "PENDING"
)
