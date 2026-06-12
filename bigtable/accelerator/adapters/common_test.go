package adapters

import "testing"

func TestAdaptersExist(t *testing.T) {
	if DefaultReadRowRequestAdapter == nil {
		t.Error("expected DefaultReadRowRequestAdapter to be non-nil")
	}
	if DefaultReadRowResponseAdapter == nil {
		t.Error("expected DefaultReadRowResponseAdapter to be non-nil")
	}
	if DefaultMutateRowRequestAdapter == nil {
		t.Error("expected DefaultMutateRowRequestAdapter to be non-nil")
	}
	if DefaultMutateRowResponseAdapter == nil {
		t.Error("expected DefaultMutateRowResponseAdapter to be non-nil")
	}
}
