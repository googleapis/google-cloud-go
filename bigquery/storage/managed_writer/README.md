# BigQuery Storage API - v1beta2 notes

## To be documented:

* default stream returns a response, but doesn't return an offset value.  It's an always available
  stream without an explicit writestream lifecycle, so the offset from start is irrelevant.

  Consider: Update API response message to acknowledge rows received, and optionally tag an append
  with an ID string.

* requests and responses:  we expect 1 response for each append, they're not collapsed.

* currently invalid: calling GetWriteStream on the default write stream.


## To add

* retry mechanism, which is stream type dependent

* checking for done: monitor all rows, or monitor writer for 0 pending writes.

func TestIntegration_MultipleStreamBehavior(t *testing.T) {
	// TODO: you can open multiple connections to a default stream,
	// but an explicitly created COMMIT stream should only allow a
	// single connection.
}

## To consider

* schema evolution



##

Retries


* what to do with stream EOF/closing?

* if we miss offset, can we resume?
- need to know prior and expected



# Missing in API

* listing streams with uncommitted data.

* FlushRowsRequest
  WriteStream should be stream name?
  Offset should be a bare int64
  