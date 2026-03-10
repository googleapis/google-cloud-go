#!/bin/bash
sed -i 's/cs.completions <- \*c/select {\n\t\t\t\t\tcase cs.completions <- *c:\n\t\t\t\t\tcase <-ctx.Done():\n\t\t\t\t\t\treturn ctx.Err()\n\t\t\t\t\t}/g' storage/grpc_writer.go
sed -i 's/cs.completions <- gRPCBidiWriteCompletion{flushOffset: r.offset + int64(len(r.buf))}/select {\n\t\t\t\t\t\tcase cs.completions <- gRPCBidiWriteCompletion{flushOffset: r.offset + int64(len(r.buf))}:\n\t\t\t\t\t\tcase <-ctx.Done():\n\t\t\t\t\t\t\treturn ctx.Err()\n\t\t\t\t\t\t}/g' storage/grpc_writer.go
