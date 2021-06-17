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
	defer func (){table = nil}()

	config := cbtconfig.Config{Creds: "c", Project: "p", Instance: "i"}
	captureStdout(func() {doMain(&config, []string{"count", "mytable"})})

	_, deadline_set := ctxt.ctx.Deadline()
	if deadline_set {
		t.Errorf("Deadline set with no timeout")
	}

	config.Timeout = time.Duration(42e9)
	now := time.Now()
	captureStdout(func() {doMain(&config, []string{"count", "mytable"})})

	deadline, deadline_set := ctxt.ctx.Deadline()
	if ! deadline_set {
		t.Errorf("Deadline set with no timeout")
	}
	timeout := deadline.Sub(now).Nanoseconds()
	if ! (timeout > 42e9 && timeout < 43e9) {
		t.Errorf("Bad actual timeout nanoseconds %d", timeout)
	}
}
