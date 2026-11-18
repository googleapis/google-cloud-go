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

	// Create a temporary directory to work with relative paths and symlinks.
	tempDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(tempDir, "symdir")); err != nil {
		t.Fatalf("os.Symlink symdir: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(tempDir, "symfile")); err != nil {
		t.Fatalf("os.Symlink symfile: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", filepath.Base(outsideDir), "missing.txt"), filepath.Join(tempDir, "dangling_out")); err != nil {
		t.Fatalf("os.Symlink dangling_out: %v", err)
	}
	if err := os.Symlink(filepath.Join("sub", "inside.txt"), filepath.Join(tempDir, "internal_sym")); err != nil {
		t.Fatalf("os.Symlink internal_sym: %v", err)
	}
	if err := os.Symlink(filepath.Join(tempDir, "sub", "inside.txt"), filepath.Join(tempDir, "abs_internal_sym")); err != nil {
		t.Fatalf("os.Symlink abs_internal_sym: %v", err)
	}
	if err := os.Symlink("symdir/..", filepath.Join(tempDir, "dangling_chain")); err != nil {
		t.Fatalf("os.Symlink dangling_chain: %v", err)
	}
	if err := os.Symlink(filepath.Join("nonexistent_dir", "..", "symdir", "escaped.txt"), filepath.Join(tempDir, "backtrack_escape")); err != nil {
		t.Fatalf("os.Symlink backtrack_escape: %v", err)
	}
	linkToTempDir := filepath.Join(outsideDir, "link_to_temp")
	if err := os.Symlink(tempDir, linkToTempDir); err != nil {
		t.Fatalf("os.Symlink linkToTempDir: %v", err)
	}

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
			name:           "symlink directory escaping localDirectory",
			localDirectory: tempDir,
			filePath:       filepath.Join(tempDir, "symdir", "escaped.txt"),
			wantIsSub:      false,
		},
		{
			name:           "symlink file escaping localDirectory",
			localDirectory: tempDir,
			filePath:       filepath.Join(tempDir, "symfile"),
			wantIsSub:      false,
		},
		{
			name:           "dangling symlink escaping localDirectory",
			localDirectory: tempDir,
			filePath:       filepath.Join(tempDir, "dangling_out"),
			wantIsSub:      false,
		},
		{
			name:           "dangling symlink chaining through escaping symlink with ..",
			localDirectory: tempDir,
			filePath:       filepath.Join(tempDir, "dangling_chain", "escaped.txt"),
			wantIsSub:      false,
		},
		{
			name:           "symlink backtracking out of nonexistent segment into escaping symlink",
			localDirectory: tempDir,
			filePath:       filepath.Join(tempDir, "backtrack_escape"),
			wantIsSub:      false,
		},
		{
			name:           "absolute symlink inside localDirectory is rejected",
			localDirectory: tempDir,
			filePath:       filepath.Join(tempDir, "abs_internal_sym"),
			wantIsSub:      false,
		},
		{
			name:           "symlink staying within localDirectory",
			localDirectory: tempDir,
			filePath:       filepath.Join(tempDir, "internal_sym"),
			wantIsSub:      true,
		},
		{
			name:           "localDirectory itself is a symlink",
			localDirectory: linkToTempDir,
			filePath:       filepath.Join(linkToTempDir, "child.txt"),
			wantIsSub:      true,
		},
		{
			name:           "lexical escape from symlinked localDirectory is rejected",
			localDirectory: linkToTempDir,
			filePath:       linkToTempDir + sep + ".." + sep + filepath.Base(tempDir) + sep + "child.txt",
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
			// induce filepath.Abs() error
			if wantErr {
				dir, _ := os.MkdirTemp("", "")
				os.Chdir(dir)
				os.RemoveAll(dir)
			}
			isSub, err := isSubPath(tc.localDirectory, tc.filePath)

			if (err != nil) != wantErr {
				t.Fatalf("isSubPath() error = %v, wantErr %v", err, tc.wantErrMsg)
			}
			if wantErr && !strings.Contains(err.Error(), tc.wantErrMsg) {
				t.Errorf("isSubPath() error = %s, want err containing %s", err.Error(), tc.wantErrMsg)
				return
			}
			if isSub != tc.wantIsSub {
				t.Errorf("isSubPath() = %v, want %v", isSub, tc.wantIsSub)
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

func TestDownloadDirectoryPathConfinement(t *testing.T) {
	t.Parallel()

	newMockServer := func(t *testing.T, objects map[string]string) *storage.Client {
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
				var items []item
				for _, name := range names {
					items = append(items, item{Name: name, Size: strconv.Itoa(len(objects[name]))})
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(struct {
					Items []item `json:"items"`
				}{Items: items})
			case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/test-bucket/"):
				objName := strings.TrimPrefix(r.URL.Path, "/test-bucket/")
				body, ok := objects[objName]
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

		ctx := context.Background()
		client, err := storage.NewClient(ctx,
			option.WithEndpoint(srv.URL+"/storage/v1/"),
			option.WithoutAuthentication(),
			storage.WithDisabledClientMetrics(),
		)
		if err != nil {
			t.Fatalf("storage.NewClient: %v", err)
		}
		t.Cleanup(func() { _ = client.Close() })
		return client
	}

	t.Run("skips traversal and symlink escapes without touching outside files", func(t *testing.T) {
		t.Parallel()
		parentDir := t.TempDir()
		localDir := filepath.Join(parentDir, "downloads")
		outsideDir := filepath.Join(parentDir, "outside")
		if err := os.MkdirAll(localDir, 0755); err != nil {
			t.Fatalf("os.MkdirAll localDir: %v", err)
		}
		if err := os.MkdirAll(outsideDir, 0755); err != nil {
			t.Fatalf("os.MkdirAll outsideDir: %v", err)
		}

		// Pre-create a file outside localDir so "../outside/existing.txt" would
		// hit an existing file if os.Stat ran before path confinement.
		existingOutside := filepath.Join(outsideDir, "existing.txt")
		if err := os.WriteFile(existingOutside, []byte("untouched"), 0644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		if err := os.Symlink(outsideDir, filepath.Join(localDir, "symdir")); err != nil {
			t.Fatalf("os.Symlink symdir: %v", err)
		}
		if err := os.Symlink(existingOutside, filepath.Join(localDir, "symfile")); err != nil {
			t.Fatalf("os.Symlink symfile: %v", err)
		}
		danglingTarget := filepath.Join(outsideDir, "dangling.txt")
		if err := os.Symlink(danglingTarget, filepath.Join(localDir, "dangling")); err != nil {
			t.Fatalf("os.Symlink dangling: %v", err)
		}

		client := newMockServer(t, map[string]string{
			"data/safe.txt":                "safe",
			"data/nested/dir/safe.txt":     "nested-safe",
			"data/../outside/existing.txt": "evil",
			"data/symdir/escaped.txt":      "evil",
			"data/symfile":                 "evil",
			"data/dangling":                "evil",
		})

		d, err := NewDownloader(client, WithWorkers(2))
		if err != nil {
			t.Fatalf("NewDownloader: %v", err)
		}
		if err := d.DownloadDirectory(context.Background(), &DownloadDirectoryInput{
			Bucket:         "test-bucket",
			LocalDirectory: localDir,
			StripPrefix:    "data/",
		}); err != nil {
			t.Fatalf("DownloadDirectory: %v", err)
		}

		results, err := d.WaitAndClose()
		if err == nil {
			t.Fatal("WaitAndClose succeeded, expected error for skipped unsafe paths")
		}
		if len(results) != 6 {
			t.Fatalf("got %d results, want 6", len(results))
		}
		for _, res := range results {
			switch res.Object {
			case "data/safe.txt", "data/nested/dir/safe.txt":
				if res.Err != nil {
					t.Errorf("unexpected error for safe object %q: %v", res.Object, res.Err)
				}
			default:
				if res.Err == nil || !strings.Contains(res.Err.Error(), "skipping download of object with unsafe path") {
					t.Errorf("object %q Err = %v, want skipping unsafe path error", res.Object, res.Err)
				}
			}
		}

		gotSafe, err := os.ReadFile(filepath.Join(localDir, "safe.txt"))
		if err != nil || string(gotSafe) != "safe" {
			t.Errorf("safe.txt = (%q, %v), want (\"safe\", nil)", gotSafe, err)
		}
		gotNested, err := os.ReadFile(filepath.Join(localDir, "nested", "dir", "safe.txt"))
		if err != nil || string(gotNested) != "nested-safe" {
			t.Errorf("nested/dir/safe.txt = (%q, %v), want (\"nested-safe\", nil)", gotNested, err)
		}
		gotOutside, err := os.ReadFile(existingOutside)
		if err != nil || string(gotOutside) != "untouched" {
			t.Errorf("existingOutside = (%q, %v), want (\"untouched\", nil)", gotOutside, err)
		}
		if _, err := os.Stat(filepath.Join(outsideDir, "escaped.txt")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("escaped.txt should not exist, stat err = %v", err)
		}
		if _, err := os.Stat(danglingTarget); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("dangling.txt should not exist, stat err = %v", err)
		}
	})

	t.Run("WithCallbacks and nil OnObjectDownload delivers skipped unsafe results", func(t *testing.T) {
		t.Parallel()
		localDir := t.TempDir()
		client := newMockServer(t, map[string]string{
			"data/sub/safe.txt": "safe",
			"data/../evil.txt":  "evil",
		})

		d, err := NewDownloader(client, WithWorkers(2), WithCallbacks())
		if err != nil {
			t.Fatalf("NewDownloader: %v", err)
		}

		var gotOutputs []DownloadOutput
		if err := d.DownloadDirectory(context.Background(), &DownloadDirectoryInput{
			Bucket:         "test-bucket",
			LocalDirectory: localDir,
			StripPrefix:    "data/",
			Callback: func(outs []DownloadOutput) {
				gotOutputs = outs
			},
		}); err != nil {
			t.Fatalf("DownloadDirectory: %v", err)
		}
		if _, err := d.WaitAndClose(); err == nil {
			t.Fatal("WaitAndClose succeeded, expected error for skipped unsafe path")
		}
		if len(gotOutputs) != 2 {
			t.Fatalf("got %d callback outputs, want 2", len(gotOutputs))
		}
		for _, out := range gotOutputs {
			if out.Object == "data/../evil.txt" && (out.Err == nil || !strings.Contains(out.Err.Error(), "skipping download of object with unsafe path")) {
				t.Errorf("unsafe object Err = %v, want skipping unsafe path error", out.Err)
			}
			if out.Object == "data/sub/safe.txt" && out.Err != nil {
				t.Errorf("safe object Err = %v, want nil", out.Err)
			}
		}
	})

	t.Run("StripPrefix colliding destination paths returns ErrExist", func(t *testing.T) {
		t.Parallel()
		localDir := t.TempDir()
		client := newMockServer(t, map[string]string{
			"foo.txt":        "first",
			"prefix/foo.txt": "second",
		})
		d, err := NewDownloader(client, WithWorkers(2))
		if err != nil {
			t.Fatalf("NewDownloader: %v", err)
		}
		defer d.WaitAndClose()

		err = d.DownloadDirectory(context.Background(), &DownloadDirectoryInput{
			Bucket:         "test-bucket",
			LocalDirectory: localDir,
			StripPrefix:    "prefix/",
		})
		if !errors.Is(err, os.ErrExist) {
			t.Fatalf("DownloadDirectory err = %v, want os.ErrExist", err)
		}
	})

	t.Run("StripPrefix colliding destination paths with SkipIfExists keeps first object", func(t *testing.T) {
		t.Parallel()
		localDir := t.TempDir()
		client := newMockServer(t, map[string]string{
			"foo.txt":        "first",
			"prefix/foo.txt": "second",
		})
		d, err := NewDownloader(client, WithWorkers(2), SkipIfExists())
		if err != nil {
			t.Fatalf("NewDownloader: %v", err)
		}
		if err := d.DownloadDirectory(context.Background(), &DownloadDirectoryInput{
			Bucket:         "test-bucket",
			LocalDirectory: localDir,
			StripPrefix:    "prefix/",
		}); err != nil {
			t.Fatalf("DownloadDirectory: %v", err)
		}
		if _, err := d.WaitAndClose(); err != nil {
			t.Fatalf("WaitAndClose: %v", err)
		}
		got, err := os.ReadFile(filepath.Join(localDir, "foo.txt"))
		if err != nil || string(got) != "first" {
			t.Errorf("foo.txt = (%q, %v), want (\"first\", nil)", got, err)
		}
	})

	t.Run("StripPrefix checks stripped destination path for ErrExist", func(t *testing.T) {
		t.Parallel()
		localDir := t.TempDir()
		existingFile := filepath.Join(localDir, "foo.txt")
		if err := os.WriteFile(existingFile, []byte("original"), 0644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		client := newMockServer(t, map[string]string{
			"prefix/foo.txt": "overwritten",
		})
		d, err := NewDownloader(client, WithWorkers(1))
		if err != nil {
			t.Fatalf("NewDownloader: %v", err)
		}
		defer d.WaitAndClose()

		err = d.DownloadDirectory(context.Background(), &DownloadDirectoryInput{
			Bucket:         "test-bucket",
			LocalDirectory: localDir,
			StripPrefix:    "prefix/",
		})
		if !errors.Is(err, os.ErrExist) {
			t.Fatalf("DownloadDirectory err = %v, want os.ErrExist", err)
		}
		got, err := os.ReadFile(existingFile)
		if err != nil || string(got) != "original" {
			t.Errorf("foo.txt = (%q, %v), want (\"original\", nil)", got, err)
		}
	})

	t.Run("StripPrefix with SkipIfExists skips existing stripped file", func(t *testing.T) {
		t.Parallel()
		localDir := t.TempDir()
		existingFile := filepath.Join(localDir, "foo.txt")
		if err := os.WriteFile(existingFile, []byte("original"), 0644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		client := newMockServer(t, map[string]string{
			"prefix/foo.txt": "overwritten",
			"prefix/new.txt": "newcontent",
		})
		d, err := NewDownloader(client, WithWorkers(1), SkipIfExists())
		if err != nil {
			t.Fatalf("NewDownloader: %v", err)
		}
		if err := d.DownloadDirectory(context.Background(), &DownloadDirectoryInput{
			Bucket:         "test-bucket",
			LocalDirectory: localDir,
			StripPrefix:    "prefix/",
		}); err != nil {
			t.Fatalf("DownloadDirectory: %v", err)
		}
		if _, err := d.WaitAndClose(); err != nil {
			t.Fatalf("WaitAndClose: %v", err)
		}

		gotExisting, err := os.ReadFile(existingFile)
		if err != nil || string(gotExisting) != "original" {
			t.Errorf("foo.txt = (%q, %v), want (\"original\", nil)", gotExisting, err)
		}
		gotNew, err := os.ReadFile(filepath.Join(localDir, "new.txt"))
		if err != nil || string(gotNew) != "newcontent" {
			t.Errorf("new.txt = (%q, %v), want (\"newcontent\", nil)", gotNew, err)
		}
	})
}
