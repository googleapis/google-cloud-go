package main

import (
	"io/ioutil"
	"path/filepath"
	"os"
	"runtime"
	"testing"
	
)

func assertNoError(t *testing.T, err error) bool {
	if err != nil {
		_, fpath, lno, ok := runtime.Caller(1)
		if ok {
			_, fname := filepath.Split(fpath)
			t.Errorf("%s:%d: %s", fname, lno, err)
		} else {
			t.Error(err)
		}
		return true
	}
	return false
}

func captureStdout(f func()) (string, error) {
	buf := make([]byte, 9999)
	n := 0

	saved := os.Stdout
	defer func() { os.Stdout = saved }()

	tmp, err := ioutil.TempFile("", "test")
	if err == nil {
		defer os.Remove(tmp.Name())
		defer tmp.Close()
		os.Stdout = tmp
		f()
		_, err = tmp.Seek(0, 0)
	}
	if err == nil {
		n, err = tmp.Read(buf)
	}
	return string(buf[:n]), err
}
