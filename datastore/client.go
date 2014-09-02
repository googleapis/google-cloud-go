package datastore

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"

	"code.google.com/p/goprotobuf/proto"
)

type client struct {
	transport http.RoundTripper
}

func (c *client) call(url string, req proto.Message, resp proto.Message) (err error) {
	client := http.Client{Transport: c.transport}
	payload, err := proto.Marshal(req)
	if err != nil {
		return
	}
	r, err := client.Post(url, "application/x-protobuf", bytes.NewBuffer(payload))
	if err != nil {
		return
	}
	if r.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return err
		}
		return errors.New("datastore: error during call: " + string(body))
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if err = proto.Unmarshal(body, resp); err != nil {
		return
	}
	return
}
