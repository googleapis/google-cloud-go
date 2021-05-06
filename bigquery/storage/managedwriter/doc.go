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

/*
Package managedwriter provides a higher level abstraction on top of the
new BigQuery streaming write API.  This API and library is the successor
to the earlier tabledata.insertAll API, exposed in cloud.google.com/go/bigquery
as the Inserter abstraction.


Stability

This package is EXPERIMENTAL and subject to change or removal without notice.


Writer Types

Streaming writers often desire different semantics based on their use case.  This
library supports four distinct modes of writing, summarized as follows:

DefaultStream

BigQuery tables support a default stream that doesn't require explicit creation.
This type of writer most closely mimics the legacy insert API semantics,
in which rows are committed and visible immediately.  The default stream is best
used for high throughput cases where you're looking for uncoordinated appends to
a table, as it doesn't expose tracking of the offset of the write stream.

CommittedStream

This mode creates and maintains a write session, with a lifecycle tied to the
managed writer.  Like a default stream, rows written are visible immediately.
However, this mode does use offset tracking to monitor how many rows have been
written through the stream.

BufferedStream

A buffered stream is one in which you are allowed to write rows into the table,
but those rows are not visible until you mark them as ready.  It's best for use
cases where you have groups of rows that should be made visible at the same time
or not at all.

PendingStream

A pending stream is used for cases where you need very specific commit guarantees.
With a pending stream, the data in the stream must explicitly be made visible via
a commit after finalizing the stream so that it will no longer accept further rows.
This mode is best for more stringent exactly-once append use cases.


Appending Data

The managed writer uses protocol buffers for sending data rows to the
BigQuery service.  As each table in BigQuery can potentially have a unique schema,
writers must be able to communicate schema of the protocol buffers messages being
sent.  To that end, the managed writer exposes a RowSerializer interface, which
allows users to both Describe() the structure of the messages they'll be sending,
as well as providing a Convert() function which is responsible for accepting an
input and serializing that data into the appropriate binary format.

Additionally, the nature of the append returns a future-like object known as
an AppendResult, which corresponds 1:1 with each row.  Writers can use this
to track the write status of each row, and verify whether individual rows have
succeeded or failed.

Ordering Guarantees

Generally, database systems treat row ordering as a nondeterministic property without
the presence of a ordering constraint, and BigQuery is no different in that regard.
While appending may use offset tracking to monitor successful appends, the stream
based nature of the append and acknowledgement flow means that reordering of row
batches may occur.

*/
package managedwriter
