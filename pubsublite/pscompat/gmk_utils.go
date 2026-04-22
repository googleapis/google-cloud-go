// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pscompat

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/IBM/sarama"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// KafkaPublishConfig holds configuration for connecting to Google Managed Kafka
// for publishing.
type KafkaPublishConfig struct {
	// BootstrapServers is the Kafka bootstrap server address.
	// Use BuildGMKBootstrapServer() to construct this for GMK clusters.
	BootstrapServers string

	// TopicName is the Kafka topic name to publish to.
	TopicName string

	// SaramaConfig is an optional pre-built Sarama configuration. If nil,
	// NewGMKSaramaConfig() will be called to build one with GCP OAUTHBEARER
	// authentication.
	SaramaConfig *sarama.Config
}

// KafkaSubscribeConfig holds configuration for connecting to Google Managed
// Kafka for receiving messages.
type KafkaSubscribeConfig struct {
	// BootstrapServers is the Kafka bootstrap server address.
	// Use BuildGMKBootstrapServer() to construct this for GMK clusters.
	BootstrapServers string

	// TopicName is the Kafka topic name to subscribe to.
	TopicName string

	// SubscriptionName is used as the Kafka consumer group ID. Consumers with the
	// same SubscriptionName share the load of consuming from the topic.
	SubscriptionName string

	// SaramaConfig is an optional pre-built Sarama configuration. If nil,
	// NewGMKSaramaConfig() will be called to build one with GCP OAUTHBEARER
	// authentication. Auto-commit is disabled for PSL-like semantics.
	SaramaConfig *sarama.Config
}

// BuildGMKBootstrapServer constructs the bootstrap server URL for a Google
// Managed Kafka cluster.
func BuildGMKBootstrapServer(projectID, region, clusterID string) string {
	return fmt.Sprintf("bootstrap.%s.%s.managedkafka.%s.cloud.goog:9092",
		clusterID, region, projectID)
}

// NewGMKSaramaConfig creates a Sarama configuration pre-configured for Google
// Managed Kafka with SASL_SSL/OAUTHBEARER authentication using GCP default
// credentials.
func NewGMKSaramaConfig(ctx context.Context) (*sarama.Config, error) {
	config := sarama.NewConfig()
	config.Version = sarama.V2_6_0_0

	// Producer settings
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Idempotent = true
	config.Net.MaxOpenRequests = 1

	// TLS
	config.Net.TLS.Enable = true
	config.Net.TLS.Config = &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// OAUTHBEARER with GCP credentials
	config.Net.SASL.Enable = true
	config.Net.SASL.Mechanism = sarama.SASLTypeOAuth

	tokenProvider, err := newGCPTokenProvider(ctx)
	if err != nil {
		return nil, err
	}
	config.Net.SASL.TokenProvider = tokenProvider

	return config, nil
}

// gmkJWTHeader is the static JWT header matching Java's GcpLoginCallbackHandler.
// Must match exactly: {"typ":"JWT","alg":"GOOG_OAUTH2_TOKEN"} (key order matters).
const gmkJWTHeader = `{"typ":"JWT","alg":"GOOG_OAUTH2_TOKEN"}`

// gmkJWTClaims controls JSON field ordering to match the Java handler's output.
type gmkJWTClaims struct {
	Exp   int64  `json:"exp"`
	Iat   int64  `json:"iat"`
	Scope string `json:"scope"`
	Sub   string `json:"sub"`
}

// gcpTokenProvider implements sarama.AccessTokenProvider using GCP default
// credentials.
//
// Google Managed Kafka expects the OAUTHBEARER token to be a custom JWT-like
// string with the format:
//
//	base64url(header) + "." + base64url(claims) + "." + base64url(access_token)
//
// Where:
//   - header is {"typ":"JWT","alg":"GOOG_OAUTH2_TOKEN"}
//   - claims is {"exp":<expiry>,"iat":<now>,"scope":"kafka","sub":"<account>"}
//   - the third segment is the base64url-encoded GCP access token
//
// This matches the token format produced by the Java GcpLoginCallbackHandler
// in managed-kafka-auth-login-handler.
type gcpTokenProvider struct {
	tokenSource oauth2.TokenSource
	email       string // service account email or user email
}

func newGCPTokenProvider(ctx context.Context) (*gcpTokenProvider, error) {
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("gmk: failed to find GCP default credentials: %w", err)
	}

	// Extract the email/account from the credentials, similar to the Java
	// GcpLoginCallbackHandler which checks credential type and extracts email.
	email := extractEmailFromCredentials(ctx, creds)

	return &gcpTokenProvider{
		tokenSource: creds.TokenSource,
		email:       email,
	}, nil
}

// extractEmailFromCredentials attempts to get the email/account from
// credentials, mirroring the Java GcpLoginCallbackHandler behavior:
//   - service_account → client_email from JSON
//   - authorized_user → email from userinfo endpoint
//   - compute engine → email from metadata (via userinfo)
func extractEmailFromCredentials(ctx context.Context, creds *google.Credentials) string {
	if len(creds.JSON) > 0 {
		var parsed struct {
			Type        string `json:"type"`
			ClientEmail string `json:"client_email"`
		}
		if err := json.Unmarshal(creds.JSON, &parsed); err == nil && parsed.ClientEmail != "" {
			return parsed.ClientEmail
		}
	}

	// For user credentials (authorized_user) and compute engine metadata,
	// fetch the email from the userinfo endpoint using the access token.
	token, err := creds.TokenSource.Token()
	if err != nil {
		return ""
	}
	return fetchEmailFromUserinfo(ctx, token.AccessToken)
}

// fetchEmailFromUserinfo retrieves the email from Google's userinfo endpoint.
func fetchEmailFromUserinfo(ctx context.Context, accessToken string) string {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var info struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return ""
	}
	return info.Email
}

// b64Encode encodes a string using base64url without padding, matching the
// Java GcpLoginCallbackHandler's b64Encode method.
func b64Encode(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func (p *gcpTokenProvider) Token() (*sarama.AccessToken, error) {
	token, err := p.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("gmk: failed to get GCP access token: %w", err)
	}

	now := time.Now()
	claims := gmkJWTClaims{
		Exp:   token.Expiry.Unix(),
		Iat:   now.Unix(),
		Scope: "kafka",
		Sub:   p.email,
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("gmk: failed to marshal JWT claims: %w", err)
	}

	// Token format: base64url(header).base64url(claims).base64url(access_token)
	kafkaToken := b64Encode(gmkJWTHeader) + "." +
		b64Encode(string(claimsJSON)) + "." +
		b64Encode(token.AccessToken)

	return &sarama.AccessToken{Token: kafkaToken}, nil
}
