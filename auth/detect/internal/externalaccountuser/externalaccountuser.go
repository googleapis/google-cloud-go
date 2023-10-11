package externalaccountuser

import (
	"context"
	"errors"
	"net/http"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/detect/internal/stsexchange"
	"cloud.google.com/go/auth/internal"
)

type Options struct {
	// Audience is the Secure Token Service (STS) audience which contains the
	// resource name for the workforce pool and the provider identifier in that
	// pool.
	Audience string
	// RefreshToken is the OAuth 2.0 refresh token.
	RefreshToken string
	// TokenURL is the STS token exchange endpoint for refresh.
	TokenURL string
	// TokenInfoURL is the STS endpoint URL for token introspection. Optional.
	TokenInfoURL string
	// ClientID is only required in conjunction with ClientSecret, as described
	// below.
	ClientID string
	// ClientSecret is currently only required if token_info endpoint also needs
	// to be called with the generated a cloud access token. When provided, STS
	// will be called with additional basic authentication using client_id as
	// username and client_secret as password.
	ClientSecret string
	// Scopes contains the desired scopes for the returned access token.
	Scopes []string

	// Client for token request.
	Client *http.Client
}

func (c *Options) validate() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.RefreshToken != "" && c.TokenURL != ""
}

func NewTokenProvider(opts *Options) (auth.TokenProvider, error) {
	if !opts.validate() {
		return nil, errors.New("detect: invalid external_account_authorized_user configuration")
	}

	tp := &tokenProvider{
		o:            opts,
		refreshToken: opts.RefreshToken,
	}
	return auth.NewCachedTokenProvider(tp, nil), nil
}

type tokenProvider struct {
	o *Options
	// guarded by the wrapping with CachedTokenProvider
	refreshToken string
}

func (tp *tokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	opts := tp.o

	clientAuth := stsexchange.ClientAuthentication{
		AuthStyle:    auth.StyleInHeader,
		ClientID:     opts.ClientID,
		ClientSecret: opts.ClientSecret,
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	stsResponse, err := stsexchange.RefreshAccessToken(ctx, &stsexchange.Options{
		Client:         opts.Client,
		Endpoint:       opts.TokenURL,
		RefreshToken:   tp.refreshToken,
		Authentication: clientAuth,
		Headers:        headers,
	})
	if err != nil {
		return nil, err
	}
	if stsResponse.ExpiresIn < 0 {
		return nil, errors.New("detect: invalid expiry from security token service")
	}

	if stsResponse.RefreshToken != "" {
		tp.refreshToken = stsResponse.RefreshToken
	}
	return &auth.Token{
		Value:  stsResponse.AccessToken,
		Expiry: time.Now().UTC().Add(time.Duration(stsResponse.ExpiresIn) * time.Second),
		Type:   internal.TokenTypeBearer,
	}, nil
}
