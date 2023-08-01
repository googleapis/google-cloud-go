// Copyright 2023 Google LLC
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

package logg

import (
	"fmt"
	"log"
	"strings"
)

var (
	// Quiet is the global variable toggled by -q flags.
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
