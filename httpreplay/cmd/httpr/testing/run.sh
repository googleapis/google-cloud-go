# Script to test the httpr binary.
# It starts the binary in record mode, runs a Go program to record some traffic,
# then starts it again in replay mode, runs the Go program again, and compares
# the outputs.

#!/bin/bash

rm test.replay

go install cloud.google.com/go/httpreplay/cmd/httpr

$GOPATH/bin/httpr -record test.replay &
httpr_pid=$!

want=$(HTTP_PROXY=localhost:8080 go run bucket_attrs.go -mode record)

kill -2 $httpr_pid

$GOPATH/bin/httpr -replay test.replay &
httpr_pid=$!

got=$(HTTP_PROXY=localhost:8080 go run bucket_attrs.go -mode replay)

kill -2 $httpr_pid

if [[ $got == $want ]]; then
  echo PASS
else
  echo FAIL
  echo got: $got
  echo want: $want
fi

rm test.replay
