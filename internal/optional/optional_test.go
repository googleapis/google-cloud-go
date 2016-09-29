package optional

import "testing"

func TestConvertSuccess(t *testing.T) {
	if got, want := ToBool(false), false; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := ToString(""), ""; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := ToInt(0), 0; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := ToUint(uint(0)), uint(0); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := ToFloat64(0.0), 0.0; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestConvertFailure(t *testing.T) {
	for _, f := range []func(){
		func() { ToBool(nil) },
		func() { ToBool(3) },
		func() { ToString(nil) },
		func() { ToString(3) },
		func() { ToInt(nil) },
		func() { ToInt("") },
		func() { ToUint(nil) },
		func() { ToUint("") },
		func() { ToFloat64(nil) },
		func() { ToFloat64("") },
	} {
		if !panics(f) {
			t.Error("got no panic, want panic")
		}
	}
}

func panics(f func()) (b bool) {
	defer func() {
		if recover() != nil {
			b = true
		}
	}()
	f()
	return false
}
