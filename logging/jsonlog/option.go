// Copyright 2021 Google LLC
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

package jsonlog

import (
	"io"
)

// LoggerOption is a configuration option for a Logger.
type LoggerOption interface {
	set(*Logger)
}

// OnErrorHook registers a function that will be called anytime an error occurs
// during logging.
func OnErrorHook(hook func(error)) LoggerOption { return onErrorHook{hook: hook} }

type onErrorHook struct{ hook func(error) }

func (h onErrorHook) set(l *Logger) {
	l.errhook = h.hook
}

// WithWriter changes where the JSON payloads of a Logger
// are written to. By default they are written to Stderr.
func WithWriter(w io.Writer) LoggerOption { return withWriter{w: w} }

type withWriter struct {
	w io.Writer
}

func (w withWriter) set(l *Logger) {
	l.w = w.w
}

// CommonLabels are labels that apply to all log entries written from a Logger,
// so that you don't have to repeat them in each log entry's Labels field. If
// any of the log entries contains a (key, value) with the same key that is in
// CommonLabels, then the entry's (key, value) overrides the one in
// CommonLabels.
func CommonLabels(m map[string]string) LoggerOption { return commonLabels(m) }

type commonLabels map[string]string

func (c commonLabels) set(l *Logger) {
	labels := map[string]string{}
	for k, v := range c {
		labels[k] = v
	}
	l.labels = labels
}
