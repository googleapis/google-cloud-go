/*
Copyright 2021 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
	"cloud.google.com/go/bigtable/internal/cbtconfig"
)

type ctxTable struct {
	ctx context.Context
}

func (ct *ctxTable) ReadRows(
	ctx context.Context,
	arg bigtable.RowSet,
	f func(bigtable.Row) bool,
	opts ...bigtable.ReadOption,
) (err error) {
	ct.ctx = ctx
	return nil
}

func TestTimeout(t *testing.T) {
	ctxt := ctxTable{}
	table = &ctxt
	defer func() { table = nil }()

	config := cbtconfig.Config{Creds: "c", Project: "p", Instance: "i"}
	captureStdout(func() { doMain(&config, []string{"count", "mytable"}) })

	_, deadlineSet := ctxt.ctx.Deadline()
	if deadlineSet {
		t.Errorf("Deadline set with no timeout in config")
	}

	config.Timeout = time.Duration(42e9)
	now := time.Now()
	captureStdout(func() { doMain(&config, []string{"count", "mytable"}) })

	deadline, deadlineSet := ctxt.ctx.Deadline()
	if !deadlineSet {
		t.Errorf("No deadline set, even though the config set one")
	}
	timeout := deadline.Sub(now).Nanoseconds()
	if !(timeout > 42e9 && timeout < 43e9) {
		t.Errorf("Bad actual timeout nanoseconds %d", timeout)
	}
}
