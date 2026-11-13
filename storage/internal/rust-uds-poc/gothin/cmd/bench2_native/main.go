// bench2_native: the 1:1 workload against the real cloud.google.com/go/storage.
package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"runtime"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	htransport "google.golang.org/api/transport/http"
	"poc/gothin/bench2"
)

func main() {
	ctx := context.Background()
	var opts []option.ClientOption
	name := "go-json"
	if os.Getenv("NATIVE_HTTP1") == "1" {
		base := http.DefaultTransport.(*http.Transport).Clone()
		base.ForceAttemptHTTP2 = false
		base.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		base.TLSClientConfig = &tls.Config{NextProtos: []string{"http/1.1"}}
		base.MaxIdleConnsPerHost = 256
		rt, err := htransport.NewTransport(ctx, base, option.WithScopes(storage.ScopeFullControl))
		if err != nil {
			panic(err)
		}
		opts = append(opts, option.WithHTTPClient(&http.Client{Transport: rt}))
		name = "go-json-h1"
	}
	var c *storage.Client
	var err error
	if os.Getenv("NATIVE_GRPC") == "1" {
		// gRPC CloudPath baseline. DirectPath is disabled explicitly so the
		// comparison with the Rust core (which has no DirectPath) is fair.
		os.Setenv("GOOGLE_CLOUD_DISABLE_DIRECT_PATH", "true")
		name = "go-grpc-cp"
		// Parity with the Rust core: BidiReadObject reads, a connection pool of
		// NumCPU (the crate uses available_parallelism subchannels), and no
		// client-metrics exporter (the crate exports nothing by default).
		c, err = storage.NewGRPCClient(ctx,
			storage.WithGRPCBidiReads(),
			storage.WithDisabledClientMetrics(),
			option.WithGRPCConnectionPool(runtime.NumCPU()))
		if err != nil {
			panic(err)
		}
		ctrl, err := storage.NewGRPCClient(ctx,
			storage.WithDisabledClientMetrics(),
			option.WithGRPCConnectionPool(1))
		if err != nil {
			panic(err)
		}
		bench2.ListObjects = func(ctx context.Context, bucket string, q *storage.Query) bench2.Iter {
			return ctrl.Bucket(bucket).Objects(ctx, q)
		}
		if os.Getenv("NATIVE_APPEND") == "1" { // zonal buckets: appendable BidiWrite
			name = "go-grpc-cp-append"
			bench2.ConfigureWriter = func(w any) {
				if sw, ok := w.(*storage.Writer); ok {
					sw.Append = true
					sw.FinalizeOnClose = true
				}
			}
		}
	} else {
		c, err = storage.NewClient(ctx, opts...)
	}
	if err != nil {
		panic(err)
	}
	bench2.Run[*storage.Client, *storage.BucketHandle, *storage.ObjectHandle, *storage.Reader, *storage.Writer, *storage.ObjectIterator](name, c, nil)
}
