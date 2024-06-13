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
	"context"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
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

	expectedErr := "transfermanager: WaitAndClose called before DownloadObject"
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
