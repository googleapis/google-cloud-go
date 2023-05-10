package logg

import (
	"fmt"
	"log"
	"strings"
)

var (
	Quiet     bool
	logBuffer []string
)

// Printf is a potentially quiet log.Printf.
func Printf(format string, values ...interface{}) {
	if Quiet {
		logBuffer = append(logBuffer, fmt.Sprintf(format, values...))
		return
	}
	log.Printf(format, values...)
}

// Fatal is a potentially really loud log.Fatal.
// It dumps the log buffer if run in quiet mode.
func Fatal(err error) {
	if Quiet && len(logBuffer) > 0 {
		log.Print(strings.Join(logBuffer, "\n"))
	}
	log.Fatal(err)
}

// Fatalf is a potentially really loud log.Fatalf.
// It dumps the log buffer if run in quiet mode.
func Fatalf(format string, values ...interface{}) {
	if Quiet && len(logBuffer) > 0 {
		log.Print(strings.Join(logBuffer, "\n"))
	}
	log.Fatalf(format, values...)
}
