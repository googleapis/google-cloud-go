package main

import (
	"bytes"
	"io"
	"os"
)

func captureStdout(f func()) string {
	saved := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = saved }()

	outC := make(chan string)
	// https://stackoverflow.com/questions/10473800/in-go-how-do-i-capture-stdout-of-a-function-into-a-string
	// copy the output in a separate goroutine so printing can't block indefinitely
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()

	f()

	// back to normal state
	w.Close()
	return <-outC
}
