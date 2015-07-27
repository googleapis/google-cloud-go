package transport

import (
	"bytes"
	"io/ioutil"
	"net/http"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

type ProtoClient struct {
	client    *http.Client
	endpoint  string
	userAgent string
}

func (c *ProtoClient) Call(ctx context.Context, method string, req, resp proto.Message) error {
	payload, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequest("POST", c.endpoint+method, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	if ua := c.userAgent; ua != "" {
		httpReq.Header.Set("User-Agent", ua)
	}

	errc := make(chan error, 1)
	cancel := makeReqCancel(httpReq)

	go func() {
		r, err := c.client.Do(httpReq)
		if err != nil {
			errc <- err
			return
		}
		defer r.Body.Close()

		body, err := ioutil.ReadAll(r.Body)
		if r.StatusCode != http.StatusOK {
			err = &ErrHTTP{
				StatusCode: r.StatusCode,
				Body:       body,
				err:        err,
			}
		}
		if err != nil {
			errc <- err
			return
		}
		errc <- proto.Unmarshal(body, resp)
	}()

	select {
	case <-ctx.Done():
		cancel(c.client.Transport) // Cancel the HTTP request.
		return ctx.Err()
	case err := <-errc:
		return err
	}
}
