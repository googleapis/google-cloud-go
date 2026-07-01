// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import "time"

// defaultClientConfig is the configuration the ClientConfigurationManager
// uses before the first successful GetClientConfiguration poll and as the
// fallback when a poll fails after the previous server-supplied
// configuration's validity window has expired.
//
// Values are kept in this file (rather than inline in
// client_configuration_manager.go) so they read as a tunable defaults
// table independent of the polling machinery that consumes them.
var defaultClientConfig = clientConfig{
	Polling: pollingConfig{
		PollingInterval:  300 * time.Second,
		ValidityDuration: 100 * 365 * 24 * time.Hour, // Safe representation of 10,000 years.
		MaxRPCRetryCount: 5,
	},
	Session: sessionConfig{
		SessionLoad: 0,
		ChannelPool: channelPoolConfig{
			MinServerCount:             2,
			MaxServerCount:             25,
			PerServerSessionCount:      10,
			Mode:                       modeDirectAccessWithFallback,
			DirectAccessCheckInterval:  60 * time.Second,
			DirectAccessErrorThreshold: 0.8,
		},
		SessionPool: sessionPoolConfig{
			Headroom:                           0.5,
			MinSessionCount:                    5,
			MaxSessionCount:                    400,
			NewSessionCreationBudget:           50,
			NewSessionCreationPenalty:          60 * time.Second,
			ConsecutiveSessionFailureThreshold: 10,
			NewSessionQueueLength:              10,
			LoadBalancing: loadBalancingOptions{
				Strategy:         StrategyLeastInFlight,
				RandomSubsetSize: 0,
			},
		},
	},
	HasSessionConfig: true,
}
