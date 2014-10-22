// Copyright 2014 Google Inc. All Rights Reserved.
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

// This is a simple command line tool for Cloud Pub/Sub
// Cloud Pub/Sub docs: https://cloud.google.com/pubsub/docs
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/go.net/context"

	"github.com/golang/oauth2"
	"github.com/golang/oauth2/google"
	"google.golang.org/cloud"
	"google.golang.org/cloud/pubsub"
)

var (
	jsonFile  = flag.String("j", "", "Secrets json file of your service account, not needed if you run it on Compute Engine instances")
	reportMPS = flag.Bool("report_mps", false, "Whether or not to report msgs per seconds")
)

const (
	usage = `Available arguments are:
    list_topics
    create_topic TOPIC
    delete_topic TOPIC
    list_subscriptions
    create_subscription SUBSCRIPTION LINKED_TOPIC
    delete_subscription SUBSCRIPTION
    publish TOPIC MESSAGE
    pull_messages SUBSCRIPTION workers
    publish_messages TOPIC workers
`
	tick               = 1 * time.Second
	googOAuth2Endpoint = "https://accounts.google.com/o/oauth2/token"
	metadataServer     = "metadata" // host name of the GCE metadata server
	projectIDPath      = "/computeMetadata/v1/project/project-id"
)

func usageAndExit(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	fmt.Println("Flags:")
	flag.PrintDefaults()
	fmt.Fprint(os.Stderr, usage)
	os.Exit(2)
}

// Check the length of the arguments.
func checkArgs(argv []string, min int) {
	if len(argv) < min {
		usageAndExit("Missing arguments")
	}
}

// jwtConfig is for JWT json file parsing.
type jwtConfig struct {
	Type         string `json:"type"`
	Scope        string `json:"scope"`
	ProjectId    string `json:"project_id"`
	PrivateKeyId string `json:"private_key_id"`
	PrivateKey   string `json:"private_key" binding:"required"`
	ClientEmail  string `json:"client_email" binding:"required"`
	ClientId     string `json:"client_id" binding:"required"`
}

// clientAndId creates http.Client and determines project id to use,
// with a jwt service account when jsonFile flag is specified,
// otherwise by obtaining the GCE service account's access token and
// project ID from the metadata server.
func clientAndId(jsonFile string) (*http.Client, string) {
	if jsonFile != "" {
		file, err := ioutil.ReadFile(jsonFile)
		if err != nil {
			log.Fatalf("Can not read file %s", jsonFile)
		}
		var config jwtConfig
		if err := json.Unmarshal(file, &config); err != nil {
			log.Fatalf("Can not parse the json file: %s, %v", jsonFile, err)
		}
		projectID := strings.SplitN(config.ClientId, "-", 2)[0]
		options := &oauth2.JWTOptions{
			Email:      config.ClientEmail,
			PrivateKey: []byte(config.PrivateKey),
			Scopes:     []string{pubsub.ScopePubSub},
		}
		conf, err := oauth2.NewJWTConfig(options, googOAuth2Endpoint)
		if err != nil {
			log.Fatalf("NewJWTConfig failed, %v", err)
		}
		client := &http.Client{Transport: conf.NewTransport()}
		return client, projectID
	} else {
		gceConfig := google.NewComputeEngineConfig("")
		client := &http.Client{Transport: gceConfig.NewTransport()}
		projectID, err := getProjectID()
		if err != nil {
			log.Fatalf("Failed to get the project id: %v", err)
		}
		return client, projectID
	}
}

// getProjectID fetches the project id from GCE metadata server.
func getProjectID() (string, error) {
	projectIDURL := &url.URL{
		Scheme: "http",
		Host:   metadataServer,
		Path:   projectIDPath,
	}
	req, err := http.NewRequest("GET", projectIDURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Metadata-Flavor", "Google")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func listTopics(ctx context.Context, argv []string) {
	panic("listTopics not implemented yet")
}

func createTopic(ctx context.Context, argv []string) {
	checkArgs(argv, 2)
	topic := argv[1]
	err := pubsub.CreateTopic(ctx, topic)
	if err != nil {
		log.Fatalf("CreateTopic failed, %v", err)
	}
	fmt.Printf("Topic %s was created.\n", topic)
}

func deleteTopic(ctx context.Context, argv []string) {
	checkArgs(argv, 2)
	topic := argv[1]
	err := pubsub.DeleteTopic(ctx, topic)
	if err != nil {
		log.Fatalf("DeleteTopic failed, %v", err)
	}
	fmt.Printf("Topic %s was deleted.\n", topic)
}

func listSubscriptions(ctx context.Context, argv []string) {
	panic("listSubscriptions not implemented yet")
}

func createSubscription(ctx context.Context, argv []string) {
	checkArgs(argv, 3)
	sub := argv[1]
	topic := argv[2]
	err := pubsub.CreateSub(ctx, sub, topic, 60*time.Second, "")
	if err != nil {
		log.Fatalf("CreateSub failed, %v", err)
	}
	fmt.Printf("Subscription %s was created.\n", sub)
}

func deleteSubscription(ctx context.Context, argv []string) {
	checkArgs(argv, 2)
	sub := argv[1]
	err := pubsub.DeleteSub(ctx, sub)
	if err != nil {
		log.Fatalf("DeleteSub failed, %v", err)
	}
	fmt.Printf("Subscription %s was deleted.\n", sub)
}

func publish(ctx context.Context, argv []string) {
	checkArgs(argv, 3)
	topic := argv[1]
	message := argv[2]
	err := pubsub.Publish(ctx, topic, []byte(message), nil)
	if err != nil {
		log.Fatalf("Publish failed, %v", err)
	}
	fmt.Printf("Message '%s' published to a topic %s\n", message, topic)
}

type puller struct {
	lastC  uint64
	c      uint64
	ctx    context.Context
	sub    string
	result chan struct{}
}

func (p *puller) pullLoop() {
	for {
		msg, err := pubsub.PullWait(p.ctx, p.sub)
		if err != nil {
			log.Printf("PullWait failed, %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if *reportMPS {
			p.result <- struct{}{}
		} else {
			fmt.Printf("Got a message: %s\n", msg.Data)
		}
	}
}

func (p *puller) ack(ackID string) {
	err := pubsub.Ack(p.ctx, p.sub, ackID)
	if err != nil {
		log.Printf("Ack failed, %v\n", err)
	}
}

func (p *puller) report() {
	ticker := time.NewTicker(tick)
	defer func() {
		ticker.Stop()
	}()
	for {
		select {
		case <-ticker.C:
			n := p.c - p.lastC
			p.lastC = p.c
			mps := n / uint64(tick/time.Second)
			log.Printf("Received ~%d msgs/s, total: %d", mps, p.c)
		case <-p.result:
			p.c += 1
		}
	}
}

func pullMessages(ctx context.Context, argv []string) {
	checkArgs(argv, 3)
	sub := argv[1]
	workers, err := strconv.Atoi(argv[2])
	if err != nil {
		log.Fatalf("Atoi failed, %v", err)
	}
	p := puller{
		ctx:    ctx,
		sub:    sub,
		result: make(chan struct{}, 1024),
	}
	for i := 0; i < int(workers); i++ {
		go p.pullLoop()
	}
	if *reportMPS {
		p.report()
	} else {
		select {}
	}
}

type publisher struct {
	lastC  uint64
	c      uint64
	ctx    context.Context
	topic  string
	result chan struct{}
}

func (p *publisher) publishLoop(workerid int) {
	var i uint64
	for {
		message := fmt.Sprintf("Worker: %d, Message: %d", workerid, i)
		err := pubsub.Publish(p.ctx, p.topic, []byte(message), nil)
		if err != nil {
			log.Printf("Publish failed, %v\n", err)
		} else {
			i++
			if *reportMPS {
				p.result <- struct{}{}
			}
		}
	}
}

func (p *publisher) report() {
	ticker := time.NewTicker(tick)
	defer func() {
		ticker.Stop()
	}()
	for {
		select {
		case <-ticker.C:
			n := p.c - p.lastC
			p.lastC = p.c
			mps := n / uint64(tick/time.Second)
			log.Printf("Sent ~%d msg/s, total: %d", mps, p.c)
		case <-p.result:
			p.c += 1
		}
	}
}

func publishMessages(ctx context.Context, argv []string) {
	checkArgs(argv, 3)
	topic := argv[1]
	workers, err := strconv.Atoi(argv[2])
	if err != nil {
		log.Fatalf("Atoi failed, %v", err)
	}
	p := publisher{
		ctx:    ctx,
		topic:  topic,
		result: make(chan struct{}, 1024),
	}
	for i := 0; i < int(workers); i++ {
		go p.publishLoop(i)
	}
	if *reportMPS {
		p.report()
	} else {
		select {}
	}
}

// This example demonstrates calling the Cloud Pub/Sub API. As of 22
// Oct 2014, the Cloud Pub/Sub API is only available if you're
// whitelisted. If you're interested in using it, please apply for the
// Limited Preview program at the following form:
// http://goo.gl/Wql9HL
//
// Also, before running this example, be sure to enable Cloud Pub/Sub
// service on your project in Developer Console at:
// https://console.developers.google.com/
//
// Unless you run this sample on Compute Engine instance, please
// create a new service account and download a json file for it at the
// developer console at: https://console.developers.google.com/
//
// It has the following subcommands:
//
// create_topic TOPIC
// delete_topic TOPIC
// create_subscription SUBSCRIPTION LINKED_TOPIC
// delete_subscription SUBSCRIPTION
// publish TOPIC MESSAGE
// pull_messages SUBSCRIPTION workers
// publish_messages TOPIC workers
//
// You can choose any names for TOPIC and SUBSCRIPTION as long as they
// follow the naming rule described at:
// https://cloud.google.com/pubsub/overview#names
//
// You can create/delete topics/subscriptions by self-explanatory
// subcommands.
//
// The "publish" subcommand is for publishing a single message to a
// specified Cloud Pub/Sub topic.
//
// The "pull_messages" subcommand is for continuously pulling messages
// from a specified Cloud Pub/Sub subscription with specified number
// of workers.
//
// The "publish_messages" subcommand is for continuously publishing
// messages to a specified Cloud Pub/Sub topic with specified number
// of workers.
func main() {
	flag.Parse()
	argv := flag.Args()
	checkArgs(argv, 1)
	client, projectID := clientAndId(*jsonFile)
	ctx := cloud.NewContext(projectID, client)
	m := map[string]func(ctx context.Context, argv []string){
		"create_topic":        createTopic,
		"delete_topic":        deleteTopic,
		"create_subscription": createSubscription,
		"delete_subscription": deleteSubscription,
		"publish":             publish,
		"pull_messages":       pullMessages,
		"publish_messages":    publishMessages,
	}
	subcommand := argv[0]
	f, ok := m[subcommand]
	if !ok {
		usageAndExit(fmt.Sprintf("Function not found for %s", subcommand))
	}
	f(ctx, argv)
}
