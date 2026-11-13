package udsthin

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	pb "poc/gothin/udsthin/gcscorepb"
)

func (c *Client) startSession(ctx context.Context) error {
	tok, err := c.opts.TokenSource.Token()
	if err != nil {
		return fmt.Errorf("token source: %w", err)
	}
	resp, err := c.stub().CreateSession(ctx, &pb.CreateSessionRequest{
		AccessToken:    tok.AccessToken,
		ExpiryUnixSecs: tok.Expiry.Unix(),
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	c.session = resp.SessionId
	go c.refreshLoop(tok.Expiry)
	return nil
}

// refreshLoop pushes a fresh token to the sidecar 2 minutes before expiry
// (the TokenSource itself handles caching and refresh with Google).
func (c *Client) refreshLoop(expiry time.Time) {
	backoff := 10 * time.Second
	for {
		wait := time.Until(expiry.Add(-2 * time.Minute))
		if wait < backoff {
			wait = backoff
		}
		select {
		case <-c.stopRef:
			return
		case <-time.After(wait):
		}
		tok, err := c.opts.TokenSource.Token()
		if err != nil {
			backoff = min(backoff*2, time.Minute)
			continue
		}
		if _, err := c.stub().UpdateToken(context.Background(), &pb.UpdateTokenRequest{
			SessionId:      c.session,
			AccessToken:    tok.AccessToken,
			ExpiryUnixSecs: tok.Expiry.Unix(),
		}); err == nil {
			expiry = tok.Expiry
			backoff = 10 * time.Second
		} else {
			backoff = min(backoff*2, time.Minute)
		}
	}
}

func (c *Client) withSession(ctx context.Context) context.Context {
	if c.session == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "x-session", c.session)
}

func (c *Client) unarySession(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return invoker(c.withSession(ctx), method, req, reply, cc, opts...)
}

func (c *Client) streamSession(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return streamer(c.withSession(ctx), desc, cc, method, opts...)
}
