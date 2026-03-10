#!/bin/bash
sed -i 's/<-ctx.Done()/<-stream.Context().Done()/g' storage/grpc_writer.go
sed -i 's/return ctx.Err()/return stream.Context().Err()/g' storage/grpc_writer.go
