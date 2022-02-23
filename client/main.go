/*
 * Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
 * SPDX-License-Identifier: MIT-0
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of this
 * software and associated documentation files (the "Software"), to deal in the Software
 * without restriction, including without limitation the rights to use, copy, modify,
 * merge, publish, distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED,
 * INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A
 * PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
 * HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
 * OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */
//go:generate protoc -I ../proto/ --go_out=./pb/ --go-grpc_out=./pb/ --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative responder.proto
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"aws-cloud-map-with-grpc/client/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	nextMsgId int // counter for providing unique message Ids
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	az := queryAvailabilityZone()

	c := newClient(az)

	// we handle a single pattern and ignore the http method provided
	http.Handle("/describe", c)

	log.Println("Now serving at 0.0.0.0:8080...")
	err := http.ListenAndServe("0.0.0.0:8080", nil)
	if err != nil {
		log.Fatalf("http serving failed: %v\n", err)
	}
}

// queryAvailabilityZone obtains the AZ in which the process runs
func queryAvailabilityZone() string {
	mdUrl := os.Getenv("ECS_CONTAINER_METADATA_URI_V4")
	if mdUrl == "" {
		return "az-localhost"
	}

	res, err := http.Get(fmt.Sprintf("%s/%s", mdUrl, "task"))
	if err != nil {
		log.Fatalf("HTTP Get on metadata endpoint failed: %v\n", err)
	}

	jsonBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatalf("could not read body: %v\n", err)
	}

	// struct local to func, holding just the info we seek from the json
	type MD struct {
		AvailabilityZone string `json:"AvailabilityZone"`
	}

	md := MD{}
	err = json.Unmarshal(jsonBytes, &md)
	if err != nil {
		log.Fatalf("could not parse json from body: %v\n", err)
	}

	return md.AvailabilityZone
}

// client is the client for conducting gRPC to the server executable
type client struct {
	client pb.ResponderClient
	az     string
}

// newClient initializes a new client.
//
// It sets up the gRPC connection which it configures with instances of a custom name
// resolver and custom loadbalancer.
// Note that the coding uses our custom "cloudmap" scheme and the service name
// and namespace name as configured in cloudmap.
//
// Caveat: This demo will split on the first "." in the host part of the URL. This
// means you cannot have service names with "." in this specific demo.
func newClient(az string) client {
	// the demo does not use channel security (no TLS in gRPC)
	noCreds := insecure.NewCredentials()

	balancer.Register(NewBalancerBuilder())
	resolverBuilder := &CloudmapResolver{}

	ctx, _ := context.WithTimeout(context.Background(), time.Second*1)

	// Note we use cloudmap as the scheme
	conn, err := grpc.DialContext(
		ctx,
		"cloudmap://server.grpc.demo",
		grpc.WithTransportCredentials(noCreds),
		grpc.WithResolvers(resolverBuilder),
		grpc.WithBalancerName("cm-az-aware"),
	)
	if err != nil {
		log.Fatalln("error: could not dial server. Aborting.")
	}

	c := pb.NewResponderClient(conn)

	return client{
		client: c,
		az:     az,
	}
}

// ServeHTTP implements http.Handler on client.
//
// This way we can directly mount an instance of client in http.Mux.
func (c client) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	// Not thread-safe actually as this is only a demo.
	id := nextMsgId
	nextMsgId = nextMsgId + 1

	req := &pb.DescribeServiceInstanceRequest{MsgId: strconv.Itoa(nextMsgId)}
	pbRes, err := c.client.DescribeServiceInstance(context.Background(), req)
	if err != nil {
		log.Printf("error when calling server instance: %v\n", err)
	}

	res := Response{
		MsgId:            strconv.Itoa(id),
		AvailabilityZone: pbRes.AvailabilityZone,
	}

	b := marshal(&res)

	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	_, err = w.Write(b)
	if err != nil {
		log.Fatalln("error: could not write out json response. Aborting.")
	}
}

// Response is used to serialize http responses as JSON
type Response struct {
	MsgId            string `json:"msgId,omitempty"`
	AvailabilityZone string `json:"availabilityZone,omitempty"`
}

// marshal is a helper method for JSON serialization
func marshal(in interface{}) []byte {
	if in == nil {
		return []byte{}
	}

	b, err := json.Marshal(in)
	if err != nil {
		log.Println("error: could not marshal value to json")
		return []byte{}
	}

	return b
}
