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
Package managedwriter provides a thick client around the BigQuery storage API's BigQueryWriteClient.
More information about this new write client may also be found in the public documentation: https://cloud.google.com/bigquery/docs/write-api

Currently, this client targets the BigQueryWriteClient present in the v1 endpoint, and is intended as a more
feature-rich successor to the classic BigQuery streaming interface, which is presented as the Inserter abstraction
in cloud.google.com/go/bigquery, and the tabledata.insertAll method if you're more familiar with the BigQuery v2 REST
methods.

# Creating a Client

To start working with this package, create a client:

	ctx := context.Background()
	client, err := managedwriter.NewClient(ctx, projectID)
	if err != nil {
		// TODO: Handle error.
	}

# Defining the Protocol Buffer Schema

The write functionality of BigQuery Storage requires data to be sent using encoded
protocol buffer messages using proto2 wire format.  As the protocol buffer is not
self-describing, you will need to provide the protocol buffer schema.
This is communicated using a DescriptorProto message, defined within the protocol
buffer libraries: https://pkg.go.dev/google.golang.org/protobuf/types/descriptorpb#DescriptorProto

More information about protocol buffers can be found in the proto2 language guide:
https://developers.google.com/protocol-buffers/docs/proto

Details about data type conversions between BigQuery and protocol buffers can be
found in the public documentation: https://cloud.google.com/bigquery/docs/write-api#data_type_conversions

For cases where the protocol buffer is compiled from a static ".proto" definition,
this process is straightforward.  Instantiate an example message, then convert the
descriptor into a descriptor proto:

	m := &myprotopackage.MyCompiledMessage{}
	descriptorProto := protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())

If the message uses advanced protocol buffer features like nested messages/groups,
or enums, the cloud.google.com/go/bigquery/storage/managedwriter/adapt subpackage
contains functionality to normalize the descriptor into a self-contained definition:

	m := &myprotopackage.MyCompiledMessage{}
	descriptorProto, err := adapt.NormalizeDescriptor(m.ProtoReflect().Descriptor())
	if err != nil {
		// TODO: Handle error.
	}

The adapt subpackage also contains functionality for generating a DescriptorProto using
a BigQuery table's schema directly.

# Constructing a ManagedStream

The ManagedStream handles management of the underlying write connection to the BigQuery
Storage service.  You can either create a write session explicitly and pass it in, or
create the write stream while setting up the ManagedStream.

It's easiest to register the protocol buffer descriptor you'll be using to send data when
setting up the managed stream using the WithSchemaDescriptor option, though you can also
set/change the schema as part of an append request once the ManagedStream is created.

	// Create a ManagedStream using an explicit stream identifer, either a default
	// stream or one explicitly created by CreateWriteStream.
	managedStream, err := client.NewManagedStream(ctx,
		WithStreamName(streamName),
		WithSchemaDescriptor(descriptorProto))
	if err != nil {
		// TODO: Handle error.
	}

In addition, NewManagedStream can create new streams implicitly:

	// Alternately, allow the ManagedStream to handle stream construction by supplying
	// additional options.
	tableName := fmt.Sprintf("projects/%s/datasets/%s/tables/%s", myProject, myDataset, myTable)
	manageStream, err := client.NewManagedStream(ctx,
		WithDestinationTable(tableName),
		WithType(managedwriter.BufferedStream),
		WithSchemaDescriptor(descriptorProto))
	if err != nil {
		// TODO: Handle error.
	}

# Writing Data

Use the AppendRows function to write one or more serialized proto messages to a stream. You
can choose to specify an offset in the stream to handle de-duplication for user-created streams,
but a "default" stream neither accepts nor reports offsets.

AppendRows returns a future-like object that blocks until the write is successful or yields
an error.

		// Define a couple of messages.
		mesgs := []*myprotopackage.MyCompiledMessage{
			{
				UserName: proto.String("johndoe"),
				EmailAddress: proto.String("jd@mycompany.mydomain",
				FavoriteNumbers: []proto.Int64{1,42,12345},
			},
			{
				UserName: proto.String("janesmith"),
				EmailAddress: proto.String("smith@othercompany.otherdomain",
				FavoriteNumbers: []proto.Int64{1,3,5,7,9},
			},
		}

		// Encode the messages into binary format.
		encoded := make([][]byte, len(mesgs))
		for k, v := range mesgs{
			b, err := proto.Marshal(v)
			if err != nil {
				// TODO: Handle error.
			}
			encoded[k] = b
	 	}

		// Send the rows to the service, and specify an offset for managing deduplication.
		result, err := managedStream.AppendRows(ctx, encoded, WithOffset(0))

		// Block until the write is complete and return the result.
		returnedOffset, err := result.GetResult(ctx)
		if err != nil {
			// TODO: Handle error.
		}

# Buffered Stream Management

For Buffered streams, users control when data is made visible in the destination table/stream
independently of when it is written.  Use FlushRows on the ManagedStream to advance the flush
point ahead in the stream.

	// We've written 1500+ rows in the stream, and want to advance the flush point
	// ahead to make the first 1000 rows available.
	flushOffset, err := managedStream.FlushRows(ctx, 1000)

# Pending Stream Management

Pending streams allow users to commit data from multiple streams together once the streams
have been finalized, meaning they'll no longer allow further data writes.

	// First, finalize the stream we're writing into.
	totalRows, err := managedStream.Finalize(ctx)
	if err != nil {
		// TODO: Handle error.
	}

	req := &storagepb.BatchCommitWriteStreamsRequest{
		Parent: parentName,
		WriteStreams: []string{managedStream.StreamName()},
	}
	// Using the client, we can commit data from multple streams to the same
	// table atomically.
	resp, err := client.BatchCommitWriteStreams(ctx, req)

# Error Handling and Automatic Retries

Like other Google Cloud services, this API relies on common components that can provide an
enhanced set of errors when communicating about the results of API interactions.

Specifically, the apierror package (https://pkg.go.dev/github.com/googleapis/gax-go/v2/apierror)
provides convenience methods for extracting structured information about errors.

The BigQuery Storage API service augments applicable errors with service-specific details in
the form of a StorageError message. The StorageError message is accessed via the ExtractProtoMessage
method in the apierror package. Note that the StorageError messsage does not implement Go's error
interface.

An example of accessing the structured error details:

	// By way of example, let's assume the response from an append call returns an error.
	_, err := result.GetResult(ctx)
	if err != nil {
		if apiErr, ok := apierror.FromError(err); ok {
			// We now have an instance of APIError, which directly exposes more specific
			// details about multiple failure conditions include transport-level errors.
			storageErr := &storagepb.StorageError{}
			if e := apiErr.Details().ExtractProtoMessage(storageErr); e != nil {
				// storageErr now contains service-specific information about the error.
				log.Printf("Received service-specific error code %s", storageErr.GetCode().String())
			}
		}
	}

This library supports the ability to retry failed append requests, but this functionality is not
enabled by default.  You can enable it via the EnableWriteRetries option when constructing a new
managed stream.  Use of automatic retries can impact correctness when attempting certain exactly-once
write patterns, but is generally recommended for workloads that only need at-least-once writing.

With write retries enabled, failed writes will be automatically attempted a finite number of times
(currently 4) if the failure is considered retriable.

In support of the retry changes, the AppendResult returned as part of an append call now includes
TotalAttempts(), which returns the number of times that specific append was enqueued to the service.
Values larger than 1 are indicative of a specific append being enqueued multiple times.

# Usage of Contexts

The underlying rpc mechanism used to transmit requests and responses between this client and
the service uses a gRPC bidirectional streaming protocol, and the context provided when invoking
NewClient to instantiate the client is used to maintain those background connections.

This package also exposes context when instantiating a new writer (NewManagedStream), as well as
allowing a per-request context when invoking the AppendRows function to send a set of rows.  If the
context becomes invalid on the writer all subsequent AppendRows requests will be blocked.

Finally, there is a per-request context supplied as part of the AppendRows call on the ManagedStream
writer itself, useful for bounding individual requests.

# Connection Sharing (Multiplexing)

Note: This feature is EXPERIMENTAL and subject to change.

The BigQuery Write API enforces a limit on the number of concurrent open connections, documented
here: https://cloud.google.com/bigquery/quotas#write-api-limits

Users can now choose to enable connection sharing (multiplexing) when using ManagedStream writers
that use default streams.  The intent of this feature is to simplify connection management for users
who wish to write to many tables, at a cardinality beyond the open connection quota.  Please note that
explicit streams (Committed, Buffered, and Pending) cannot leverage the connection sharing feature.

Multiplexing features are controlled by the package-specific custom ClientOption options exposed within
this package.  Additionally, some of the connection-related WriterOptions that can be specified when
constructing ManagedStream writers are ignored for writers that leverage the shared multiplex connections.

At a high level, multiplexing uses some heuristics based on the flow control of the shared connections
to infer whether the pool should add additional connections up to a user-specific limit per region,
and attempts to balance traffic from writers to those connections.

To enable multiplexing for writes to default streams, simply instantiate the client with the desired options:

	ctx := context.Background()
	client, err := managedwriter.NewClient(ctx, projectID,
		WithMultiplexing,
		WithMultiplexPoolLimit(3),
	)
	if err != nil {
		// TODO: Handle error.
	}

Special Consideration:  The gRPC architecture is capable of its own sharing of underlying HTTP/2 connections.
For users who are sending significant traffic on multiple writers (independent of whether they're leveraging
multiplexing or not) may also wish to consider further tuning of this behavior.  The managedwriter library
sets a reasonable default, but this can be tuned further by leveraging the WithGRPCConnectionPool ClientOption,
documented here:
https://pkg.go.dev/google.golang.org/api/option#WithGRPCConnectionPool

A reasonable upper bound for the connection pool size is the number of concurrent writers for explicit stream
plus the configured size of the multiplex pool.

# Writing JSON Data

As an example, you can refer to this integration test that demonstrates writing JSON data to a stream:
https://github.com/googleapis/google-cloud-go/blob/7a46b5428f239871993d66be2c7c667121f60a6f/bigquery/storage/managedwriter/integration_test.go#L397

This integration test assumes the destination table already exists. In addition, it relies upon having a definition of
a BigQuery schema that is compatible with this table (for this example the schema is defined here:
https://github.com/googleapis/google-cloud-go/blob/2020edff24e3ffe127248cf9a90c67593c303e18/bigquery/storage/managedwriter/testdata/schemas.go#L31).
Given the schema, this test first utilizes the function setupDynamicDescriptors() to derive both a MessageDescriptor
and DescriptorProto from the schema. This function is defined here:
https://github.com/googleapis/google-cloud-go/blob/7a46b5428f239871993d66be2c7c667121f60a6f/bigquery/storage/managedwriter/integration_test.go#L100
The test initializes the ManagedStream it will write to with the derived DescriptorProto. The test then iterates
through each of the JSON rows to be written. For each row, it first dynamically creates an empty Message based on
the derived MessageDescriptor. Then it loads the JSON row into the Message. Finally it generates protocol buffer
bytes from the Message. These bytes are then sent to the ManagedStream within an AppendRows request.
*/
package managedwriter
