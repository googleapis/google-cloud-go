// Copyright 2024 Google LLC
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

package transfermanager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

func TestWaitAndClose(t *testing.T) {
	t.Parallel()
	d, err := NewDownloader(nil)
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}

	if _, err := d.WaitAndClose(); err != nil {
		t.Fatalf("WaitAndClose: %v", err)
	}

	expectedErr := "transfermanager: Downloader used after WaitAndClose was called"
	err = d.DownloadObject(context.Background(), &DownloadObjectInput{})
	if err == nil {
		t.Fatalf("d.DownloadObject err was nil, should be %q", expectedErr)
	}
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected err %q, got: %v", expectedErr, err.Error())
	}
}

func TestNumShards(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		desc       string
		objRange   *DownloadRange
		objSize    int64
		partSize   int64
		transcoded bool
		want       int
	}{
		{
			desc:     "nil range",
			objSize:  100,
			partSize: 1000,
			want:     1,
		},
		{
			desc:     "nil - object equal to partSize",
			objSize:  100,
			partSize: 100,
			want:     1,
		},
		{
			desc:     "nil - object larger than partSize",
			objSize:  100,
			partSize: 10,
			want:     10,
		},
		{
			desc: "full object smaller than partSize",
			objRange: &DownloadRange{
				Length: 100,
			},
			objSize:  100,
			partSize: 101,
			want:     1,
		},
		{
			desc: "full object equal to partSize",
			objRange: &DownloadRange{
				Length: 100,
			},
			objSize:  100,
			partSize: 100,
			want:     1,
		},
		{
			desc: "full object larger than partSize",
			objRange: &DownloadRange{
				Length: 100,
			},
			objSize:  100,
			partSize: 99,
			want:     2,
		},
		{
			desc: "partial object smaller than partSize",
			objRange: &DownloadRange{
				Length: 50,
			},
			objSize:  100,
			partSize: 1000,
			want:     1,
		},
		{
			desc: "full object larger than partSize",
			objRange: &DownloadRange{
				Length: 5000,
			},
			objSize:  5001,
			partSize: 1000,
			want:     5,
		},
		{
			desc: "full object larger than partSize - off by one check",
			objRange: &DownloadRange{
				Length: 5001,
			},
			objSize:  5001,
			partSize: 1000,
			want:     6,
		},
		{
			desc: "length larger than object size",
			objRange: &DownloadRange{
				Length: 17000,
			},
			objSize:  5000,
			partSize: 1000,
			want:     5,
		},
		{
			desc: "negative length",
			objRange: &DownloadRange{
				Length: -1,
			},
			objSize:  5000,
			partSize: 1000,
			want:     5,
		},
		{
			desc: "offset object smaller than partSize",
			objRange: &DownloadRange{
				Offset: 50,
				Length: 99,
			},
			objSize:  100,
			partSize: 1000,
			want:     1,
		},
		{
			desc: "offset object larger than partSize",
			objRange: &DownloadRange{
				Offset: 1000,
				Length: 1999,
			},
			objSize:  2000,
			partSize: 100,
			want:     10,
		},
		{
			desc: "offset object larger than partSize - length larger than objSize",
			objRange: &DownloadRange{
				Offset: 1000,
				Length: 10000,
			},
			objSize:  2001,
			partSize: 100,
			want:     11,
		},
		{
			desc: "offset object larger than partSize - length larger than objSize",
			objRange: &DownloadRange{
				Offset: 1000,
				Length: 10000,
			},
			objSize:  2001,
			partSize: 100,
			want:     11,
		},
		{
			desc: "negative offset smaller than partSize",
			objRange: &DownloadRange{
				Offset: -5,
				Length: -1,
			},
			objSize:  1024 * 1024 * 1024 * 10,
			partSize: 100,
			want:     1,
		},
		{
			desc: "negative offset larger than partSize",
			objRange: &DownloadRange{
				Offset: -1000,
				Length: -1,
			},
			objSize:  2000,
			partSize: 100,
			want:     1,
		},
		{
			desc:       "transcoded",
			objSize:    2000,
			partSize:   100,
			transcoded: true,
			want:       1,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			attrs := &storage.ReaderObjectAttrs{
				Size: test.objSize,
			}

			if test.transcoded {
				attrs.ContentEncoding = "gzip"
			}

			got := numShards(attrs, test.objRange, test.partSize)

			if got != test.want {
				t.Errorf("numShards incorrect; expect object to be divided into %d shards, got %d", test.want, got)
			}
		})
	}
}

func TestIsSubPath(t *testing.T) {
	t.Parallel()
	sep := string(filepath.Separator)

	// Create a temporary directory to work with relative paths.
	tempDir := t.TempDir()

	testCases := []struct {
		name           string
		localDirectory string
		filePath       string
		wantIsSub      bool
		wantErrMsg     string
	}{
		{
			name:           "filePath is a child",
			localDirectory: "/tmp/foo",
			filePath:       "/tmp/foo/bar",
			wantIsSub:      true,
		},
		{
			name:           "filePath is a nested child",
			localDirectory: "/tmp/foo",
			filePath:       "/tmp/foo/bar/baz",
			wantIsSub:      true,
		},
		{
			name:           "filePath is the same as localDirectory",
			localDirectory: "/tmp/foo",
			filePath:       "/tmp/foo",
			wantIsSub:      false,
		},
		{
			name:           "local directory is /",
			localDirectory: "/",
			filePath:       "/../../a",
			wantIsSub:      true,
		},
		{
			name:           "filePath is a sibling",
			localDirectory: "/tmp/foo",
			filePath:       "/tmp/bar",
			wantIsSub:      false,
		},
		{
			name:           "filePath is a parent",
			localDirectory: "/tmp/foo",
			filePath:       "/tmp",
			wantIsSub:      false,
		},
		{
			name:           "directory traversal attempt",
			localDirectory: "/tmp/foo",
			filePath:       "/tmp/foo/../bar", // resolves to /tmp/bar
			wantIsSub:      false,
		},
		{
			name:           "deeper directory traversal attempt",
			localDirectory: "/tmp/foo",
			filePath:       "/tmp/foo/bar/../../baz", // resolves to /tmp/baz
			wantIsSub:      false,
		},
		{
			name:           "relative paths - valid",
			localDirectory: tempDir,
			filePath:       filepath.Join(tempDir, "bar"),
			wantIsSub:      true,
		},
		{
			name:           "relative paths - traversal",
			localDirectory: tempDir,
			filePath:       filepath.Join(tempDir, "..", "bar"),
			wantIsSub:      false,
		},
		{
			name:           "relative path is just ..",
			localDirectory: "foo",
			filePath:       "foo" + sep + ".." + sep + "..",
			wantIsSub:      false,
		},
		{
			name:           "IsSubPath returns error when base dir is changed",
			localDirectory: "foo",
			filePath:       "bar",
			wantErrMsg:     "no such file or directory",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origWd, _ := os.Getwd()
			t.Cleanup(func() {
				os.Chdir(origWd)
			})
			wantErr := (tc.wantErrMsg != "")
			absLocalDir, err := filepath.Abs(tc.localDirectory)
			if err != nil {
				t.Fatalf("filepath.Abs(%q): %v", tc.localDirectory, err)
			}
			// induce filepath.Abs() error
			if wantErr {
				dir, _ := os.MkdirTemp("", "")
				os.Chdir(dir)
				os.RemoveAll(dir)
			}
			_, isSub, err := resolveSubPath(absLocalDir, tc.filePath)

			if (err != nil) != wantErr {
				t.Fatalf("resolveSubPath() error = %v, wantErr %v", err, tc.wantErrMsg)
			}
			if wantErr && !strings.Contains(err.Error(), tc.wantErrMsg) {
				t.Errorf("resolveSubPath() error = %s, want err containing %s", err.Error(), tc.wantErrMsg)
				return
			}
			if isSub != tc.wantIsSub {
				t.Errorf("resolveSubPath() = %v, want %v", isSub, tc.wantIsSub)
			}
		})
	}
}

func TestCalculateRange(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		desc     string
		objRange *DownloadRange
		partSize int64
		shard    int
		want     DownloadRange
	}{
		{
			desc:     "nil range - first shard",
			partSize: 1000,
			shard:    0,
			want: DownloadRange{
				Length: 1000,
			},
		},
		{
			desc:     "nil range",
			partSize: 1001,
			shard:    3,
			want: DownloadRange{
				Offset: 3003,
				Length: 1001,
			},
		},
		{
			desc: "first shard length smaller than partSize",
			objRange: &DownloadRange{
				Length: 99,
			},
			partSize: 1000,
			shard:    0,
			want: DownloadRange{
				Length: 99,
			},
		},
		{
			desc: "second shard",
			objRange: &DownloadRange{
				Length: 4999,
			},
			partSize: 1000,
			shard:    1,
			want: DownloadRange{
				Offset: 1000,
				Length: 1000,
			},
		},
		{
			desc: "last shard",
			objRange: &DownloadRange{
				Length: 5000,
			},
			partSize: 1000,
			shard:    4,
			want: DownloadRange{
				Offset: 4000,
				Length: 1000,
			},
		},
		{
			desc: "last shard",
			objRange: &DownloadRange{
				Length: 5001,
			},
			partSize: 1000,
			shard:    5,
			want: DownloadRange{
				Offset: 5000,
				Length: 1,
			},
		},
		{
			desc: "single shard with offset",
			objRange: &DownloadRange{
				Offset: 10,
				Length: 99,
			},
			partSize: 1000,
			shard:    0,
			want: DownloadRange{
				Offset: 10,
				Length: 99,
			},
		},
		{
			desc: "second shard with offset",
			objRange: &DownloadRange{
				Offset: 100,
				Length: 500,
			},
			partSize: 100,
			shard:    1,
			want: DownloadRange{
				Offset: 200,
				Length: 100,
			},
		},
		{
			desc: "off by one",
			objRange: &DownloadRange{
				Offset: 101,
				Length: 500,
			},
			partSize: 100,
			shard:    2,
			want: DownloadRange{
				Offset: 301,
				Length: 100,
			},
		},
		{
			desc: "last shard",
			objRange: &DownloadRange{
				Offset: 1,
				Length: 5000,
			},
			partSize: 1000,
			shard:    4,
			want: DownloadRange{
				Offset: 4001,
				Length: 1000,
			},
		},
		{
			desc: "sharding turned off",
			objRange: &DownloadRange{
				Offset: 1024 * 1024 * 1024 * 1024 / 2,
				Length: 1024 * 1024 * 1024 * 1024,
			},
			partSize: 0,
			shard:    0,
			want: DownloadRange{
				Offset: 1024 * 1024 * 1024 * 1024 / 2,
				Length: 1024 * 1024 * 1024 * 1024,
			},
		},
		{
			desc: "large object",
			objRange: &DownloadRange{
				Offset: 1024 * 1024 * 1024 * 1024 / 2,
				Length: 1024 * 1024 * 1024 * 1024, // 1TiB
			},
			partSize: 1024 * 1024 * 1024, // 1 Gib
			shard:    1024/2 - 1,         // last shard
			want: DownloadRange{
				Offset: 1024*1024*1024*1024 - 1024*1024*1024,
				Length: 1024 * 1024 * 1024,
			},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			got := shardRange(test.objRange, test.partSize, test.shard)

			if got != test.want {
				t.Errorf("want %v got %v", test.want, got)
			}
		})
	}
}

// This tests that gather shards works as expected and cancels other shards
// without error after it encounters an error.
func TestGatherShards(t *testing.T) {
	content := "hello world"
	piece1 := "hello"
	piece2 := " "
	piece3 := "world"
	fullCRC := crc32c([]byte(content))
	firstPieceCRC := crc32c([]byte(piece1))

	for _, tc := range []struct {
		desc         string
		shards       int
		outputs      []*DownloadOutput
		checkCRC     bool
		wantErr      bool
		errMsg       string
		expectCancel bool
	}{
		{
			desc:   "all shards succeed",
			shards: 3,
			outputs: []*DownloadOutput{
				{shard: 1, crc32c: crc32c([]byte(piece2)), shardLength: int64(len(piece2))},
				{shard: 2, crc32c: crc32c([]byte(piece3)), shardLength: int64(len(piece3))},
			},
			checkCRC: true,
			wantErr:  false,
		},
		{
			desc:   "one shard fails",
			shards: 3,
			outputs: []*DownloadOutput{
				{shard: 0, Err: errors.New("shard failure")},
				{shard: 2, crc32c: crc32c([]byte(piece3)), shardLength: int64(len(piece3))},
			},
			checkCRC:     true,
			wantErr:      true,
			expectCancel: true,
			errMsg:       "shard failure",
		},
		{
			desc:   "checksum mismatch",
			shards: 3,
			outputs: []*DownloadOutput{
				{shard: 1, crc32c: crc32c([]byte(piece2)), shardLength: int64(len(piece2))},
				{shard: 2, crc32c: 999, shardLength: int64(len(piece3))}, // Wrong CRC
			},
			checkCRC: true,
			wantErr:  true,
			errMsg:   "bad CRC on read",
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, cancelCtx := context.WithCancelCause(context.Background())
			defer cancelCtx(nil)
			d, err := NewDownloader(nil, WithWorkers(2), WithCallbacks())
			if err != nil {
				t.Fatalf("NewDownloader: %v", err)
			}

			outChan := make(chan *DownloadOutput, tc.shards)
			in := &DownloadObjectInput{
				Callback:     func(o *DownloadOutput) {},
				ctx:          ctx,
				cancelCtx:    cancelCtx,
				shardOutputs: outChan,
				checkCRC:     tc.checkCRC,
			}
			out := &DownloadOutput{
				Attrs: &storage.ReaderObjectAttrs{CRC32C: fullCRC},
			}
			var wg sync.WaitGroup
			wg.Add(1)
			d.downloadsInProgress.Add(1)
			go func() {
				d.gatherShards(in, out, outChan, tc.shards, firstPieceCRC)
				wg.Done()
			}()

			for _, so := range tc.outputs {
				outChan <- so
			}

			wg.Wait()

			// WaitAndClose will wait for downloadsInProgress, including gatherShards goroutine.
			if _, err := d.WaitAndClose(); err != nil && !tc.wantErr {
				t.Logf("WaitAndClose returned error: %v", err)
			}
			if tc.expectCancel {
				select {
				case <-ctx.Done():
					if ctxErr := context.Cause(ctx); !errors.Is(ctxErr, errCancelAllShards) {
						t.Errorf("context.Cause: error should be %q; instead got: %v", errCancelAllShards, ctxErr)
					}
				default:
					t.Error("context was not cancelled for shard error")
				}
			}

			if (out.Err != nil) != tc.wantErr {
				t.Errorf("got error %v, wantErr %v", out.Err, tc.wantErr)
			}
			if tc.wantErr && tc.errMsg != "" && (out.Err == nil || !strings.Contains(out.Err.Error(), tc.errMsg)) {
				t.Errorf("expected error containing %q, got %v", tc.errMsg, out.Err)
			}
		})
	}
}

func TestCalculateCRC32C(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		desc   string
		pieces []string
	}{
		{
			desc:   "equal sized pieces",
			pieces: []string{"he", "ll", "o ", "wo", "rl", "d!"},
		},
		{
			desc:   "uneven pieces",
			pieces: []string{"hello", " ", "world!"},
		},
		{
			desc: "large pieces",
			pieces: []string{string(bytes.Repeat([]byte("a"), 1024*1024*32)),
				string(bytes.Repeat([]byte("b"), 1024*1024*32)),
				string(bytes.Repeat([]byte("c"), 1024*1024*32)),
			},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			initialChecksum := crc32c([]byte(test.pieces[0]))

			remainingChecksums := make([]crc32cPiece, len(test.pieces)-1)
			for i, piece := range test.pieces[1:] {
				remainingChecksums[i] = crc32cPiece{sum: crc32c([]byte(piece)), length: int64(len(piece))}
			}

			got := joinCRC32C(initialChecksum, remainingChecksums)
			want := crc32c([]byte(strings.Join(test.pieces, "")))

			if got != want {
				t.Errorf("crc32c not calculated correctly - want %v, got %v", want, got)
			}
		})
	}
}

func newMockDownloadDirClient(t *testing.T, objects map[string]string) *storage.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/storage/v1/b/test-bucket/o":
			type item struct {
				Name string `json:"name"`
				Size string `json:"size"`
			}
			names := make([]string, 0, len(objects))
			for name := range objects {
				names = append(names, name)
			}
			sort.Strings(names)
			items := make([]item, 0, len(names))
			for _, name := range names {
				items = append(items, item{Name: name, Size: strconv.Itoa(len(objects[name]))})
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(struct {
				Items []item `json:"items"`
			}{Items: items})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/test-bucket/"):
			body, ok := objects[strings.TrimPrefix(r.URL.Path, "/test-bucket/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c, err := storage.NewClient(context.Background(),
		option.WithEndpoint(srv.URL+"/storage/v1/"),
		option.WithoutAuthentication(),
		storage.WithDisabledClientMetrics(),
	)
	if err != nil {
		t.Fatalf("storage.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestDownloadDirectoryPathConfinement(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		opts           []Option
		objects        map[string]string
		setup          func(t *testing.T, localDir, outsideDir string)
		wantSyncErr    error
		wantSyncErrSub string
		wantWaitErrSub string
		wantLocal      map[string]string
	}{
		{
			name: "skips lexical traversal and creates nested dirs",
			objects: map[string]string{
				"data/nested/safe.txt":         "safe",
				"data/../outside/existing.txt": "evil",
			},
			setup: func(t *testing.T, _, outsideDir string) {
				if err := os.WriteFile(filepath.Join(outsideDir, "existing.txt"), []byte("untouched"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			wantWaitErrSub: "skipping download of object with unsafe path",
			wantLocal:      map[string]string{"nested/safe.txt": "safe"},
		},
		{
			name:    "rejects symlink directory escape",
			objects: map[string]string{"data/symdir/escaped.txt": "evil"},
			setup: func(t *testing.T, localDir, outsideDir string) {
				if err := os.Symlink(outsideDir, filepath.Join(localDir, "symdir")); err != nil {
					t.Fatal(err)
				}
			},
			wantSyncErrSub: "failed to make directory",
		},
		{
			name:    "rejects dangling symlink escape",
			objects: map[string]string{"data/dangling": "evil"},
			setup: func(t *testing.T, localDir, outsideDir string) {
				if err := os.Symlink(filepath.Join(outsideDir, "dangling.txt"), filepath.Join(localDir, "dangling")); err != nil {
					t.Fatal(err)
				}
			},
			wantSyncErrSub: "failed to create file",
		},
		{
			name:    "StripPrefix checks stripped path for ErrExist",
			objects: map[string]string{"data/foo.txt": "overwritten"},
			setup: func(t *testing.T, localDir, _ string) {
				if err := os.WriteFile(filepath.Join(localDir, "foo.txt"), []byte("original"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			wantSyncErr: os.ErrExist,
			wantLocal:   map[string]string{"foo.txt": "original"},
		},
		{
			name:    "StripPrefix with SkipIfExists preserves existing stripped file",
			opts:    []Option{SkipIfExists()},
			objects: map[string]string{"data/foo.txt": "overwritten", "data/new.txt": "new"},
			setup: func(t *testing.T, localDir, _ string) {
				if err := os.WriteFile(filepath.Join(localDir, "foo.txt"), []byte("original"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			wantLocal: map[string]string{"foo.txt": "original", "new.txt": "new"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parent := t.TempDir()
			localDir := filepath.Join(parent, "local")
			outsideDir := filepath.Join(parent, "outside")
			if err := os.MkdirAll(localDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(outsideDir, 0755); err != nil {
				t.Fatal(err)
			}
			if tc.setup != nil {
				tc.setup(t, localDir, outsideDir)
			}

			d, err := NewDownloader(newMockDownloadDirClient(t, tc.objects), append([]Option{WithWorkers(2)}, tc.opts...)...)
			if err != nil {
				t.Fatalf("NewDownloader: %v", err)
			}
			err = d.DownloadDirectory(context.Background(), &DownloadDirectoryInput{
				Bucket:         "test-bucket",
				LocalDirectory: localDir,
				StripPrefix:    "data/",
			})
			if tc.wantSyncErr != nil && !errors.Is(err, tc.wantSyncErr) {
				t.Fatalf("DownloadDirectory err = %v, want %v", err, tc.wantSyncErr)
			}
			if tc.wantSyncErrSub != "" && (err == nil || !strings.Contains(err.Error(), tc.wantSyncErrSub)) {
				t.Fatalf("DownloadDirectory err = %v, want substring %q", err, tc.wantSyncErrSub)
			}
			if tc.wantSyncErr == nil && tc.wantSyncErrSub == "" && err != nil {
				t.Fatalf("DownloadDirectory unexpected err: %v", err)
			}

			_, waitErr := d.WaitAndClose()
			if tc.wantWaitErrSub != "" && (waitErr == nil || !strings.Contains(waitErr.Error(), tc.wantWaitErrSub)) {
				t.Fatalf("WaitAndClose err = %v, want substring %q", waitErr, tc.wantWaitErrSub)
			}
			if tc.wantWaitErrSub == "" && waitErr != nil {
				t.Fatalf("WaitAndClose unexpected err: %v", waitErr)
			}

			for rel, want := range tc.wantLocal {
				got, err := os.ReadFile(filepath.Join(localDir, rel))
				if err != nil || string(got) != want {
					t.Errorf("%s = (%q, %v), want (%q, nil)", rel, got, err, want)
				}
			}
			entries, _ := os.ReadDir(outsideDir)
			for _, e := range entries {
				if e.Name() != "existing.txt" {
					t.Errorf("unexpected file created in outsideDir: %q", e.Name())
				}
			}
			if b, err := os.ReadFile(filepath.Join(outsideDir, "existing.txt")); err == nil && string(b) != "untouched" {
				t.Errorf("outside/existing.txt modified to %q", b)
			}
		})
	}
}

func TestDownloadDirectoryEmptyLocalDirectory(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("os.Chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWd)
	})

	d, err := NewDownloader(newMockDownloadDirClient(t, map[string]string{"file.txt": "hello"}), WithWorkers(1))
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}
	if err := d.DownloadDirectory(context.Background(), &DownloadDirectoryInput{
		Bucket:         "test-bucket",
		LocalDirectory: "",
	}); err != nil {
		t.Fatalf("DownloadDirectory: %v", err)
	}
	if _, err := d.WaitAndClose(); err != nil {
		t.Fatalf("WaitAndClose: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(tmpDir, "file.txt"))
	if err != nil || string(got) != "hello" {
		t.Fatalf("file.txt = (%q, %v), want (%q, nil)", got, err, "hello")
	}
}

func TestDownloadDirectoryCleanupNonExistentDirNoPanic(t *testing.T) {
	// Place a regular file in the path so os.MkdirAll fails and triggers cleanFiles
	// on a non-existent LocalDirectory.
	tmpDir := t.TempDir()
	blocker := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	d, err := NewDownloader(newMockDownloadDirClient(t, map[string]string{"file.txt": "hello"}), WithWorkers(1))
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}
	err = d.DownloadDirectory(context.Background(), &DownloadDirectoryInput{
		Bucket:         "test-bucket",
		LocalDirectory: filepath.Join(blocker, "nested"),
	})
	if err == nil {
		t.Fatal("DownloadDirectory = nil, want error when parent path is a regular file")
	}
	_, _ = d.WaitAndClose()
}
