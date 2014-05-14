package gcloud

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"

	"code.google.com/p/goprotobuf/proto"
)

type Client struct {
	// An authorized transport.
	Transport http.RoundTripper
}

func (c *Client) Call(url string, req proto.Message, resp proto.Message) (err error) {
	client := http.Client{Transport: c.Transport}
	payload, err := proto.Marshal(req)
	if err != nil {
		return
	}
	r, err := client.Post(url, "application/x-protobuf", bytes.NewBuffer(payload))
	if err != nil {
		return
	}
	if r.StatusCode != http.StatusOK {
		// TODO(jbd): Handle if there is no body
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return errors.New("gcloud: error during call")
		}
		return errors.New("gcloud: error during call: " + string(body))
	}
	body, _ := ioutil.ReadAll(r.Body)
	if err = proto.Unmarshal(body, resp); err != nil {
		return
	}
	return
}
