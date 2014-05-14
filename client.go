package gcloud

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
)

type Client struct {
	// An authorized transport.
	Transport http.RoundTripper
}

func (c *Client) Call(url string, req interface{}, resp interface{}) (err error) {
	client := http.Client{Transport: c.Transport}
	payload, err := json.Marshal(req)
	if err != nil {
		return
	}
	r, err := client.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return
	}
	if r.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(r.Body)
		return errors.New("gcloud: error during call: " + string(body))
	}
	if err = json.NewDecoder(r.Body).Decode(&resp); err != nil {
		return
	}
	return
}
