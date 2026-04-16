/*
Copyright 2026 Google LLC

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

package spanner

import (
	"time"

	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type suppressRetryCodesOption struct {
	codes map[codes.Code]struct{}
}

func newSuppressRetryCodesOption(suppressedCodes ...codes.Code) suppressRetryCodesOption {
	suppressed := make(map[codes.Code]struct{}, len(suppressedCodes))
	for _, code := range suppressedCodes {
		suppressed[code] = struct{}{}
	}
	return suppressRetryCodesOption{codes: suppressed}
}

type resourceExhaustedMarkerOption struct {
	mark                         func(error)
	allowRetryWithoutServerDelay bool
}

func appendResourceExhaustedMarkerOptions(base []gax.CallOption, mark func(error), allowRetryWithoutServerDelay bool) []gax.CallOption {
	if mark == nil && !allowRetryWithoutServerDelay {
		return base
	}
	opts := append([]gax.CallOption{}, base...)
	opts = append(opts, resourceExhaustedMarkerOption{
		mark:                         mark,
		allowRetryWithoutServerDelay: allowRetryWithoutServerDelay,
	})
	return opts
}

func (opt resourceExhaustedMarkerOption) Resolve(cs *gax.CallSettings) {
	if cs.Retry == nil {
		return
	}

	originalRetryFactory := cs.Retry
	cs.Retry = func() gax.Retryer {
		originalRetryer := originalRetryFactory()
		if originalRetryer == nil {
			return nil
		}
		if opt.allowRetryWithoutServerDelay {
			if originalSpannerRetryer, ok := originalRetryer.(*spannerRetryer); ok {
				originalRetryer = &spannerRetryer{
					Retryer:                                originalSpannerRetryer.Retryer,
					allowResourceExhaustedWithoutRetryInfo: true,
				}
			}
		}
		if opt.mark == nil {
			return originalRetryer
		}

		return wrapRetryFn(func(err error) (time.Duration, bool) {
			if status.Code(err) == codes.ResourceExhausted {
				opt.mark(err)
			}
			return originalRetryer.Retry(err)
		})
	}
}

func (opt suppressRetryCodesOption) Resolve(cs *gax.CallSettings) {
	if len(opt.codes) == 0 || cs.Retry == nil {
		return
	}

	originalRetryFactory := cs.Retry
	cs.Retry = func() gax.Retryer {
		originalRetryer := originalRetryFactory()
		if originalRetryer == nil {
			return nil
		}

		return wrapRetryFn(func(err error) (time.Duration, bool) {
			if _, found := opt.codes[status.Code(err)]; found {
				return 0, false
			}
			return originalRetryer.Retry(err)
		})
	}
}
