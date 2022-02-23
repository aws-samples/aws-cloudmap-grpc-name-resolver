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
	"net"
	"net/http"
	"os"

	"aws-cloud-map-with-grpc/server/pb"
	"google.golang.org/grpc"
)

func main() {
	md := queryMetadata()
	log.Printf("server starting with in AZ: %v\n", md.AvailabilityZone)

	s := grpc.NewServer()
	pb.RegisterResponderServer(s, &server{md: md})

	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", 9000))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	log.Printf("server listening at 0.0.0.0:9000\n")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

// server implements the gRPC server interface for the Responder service
// as specified under proto/responder.proto
type server struct {
	pb.UnimplementedResponderServer

	md metadata
}

// DescribeServiceInstance responds by describing the current task with various attributes
func (r server) DescribeServiceInstance(
	ctx context.Context,
	request *pb.DescribeServiceInstanceRequest,
) (*pb.DescribeServiceInstanceResponse, error) {

	res := &pb.DescribeServiceInstanceResponse{
		MsgId:              request.MsgId,
		Cluster:            r.md.Cluster,
		TaskArn:            r.md.TaskArn,
		TaskFamily:         r.md.Family,
		TaskFamilyRevision: r.md.Revision,
		AvailabilityZone:   r.md.AvailabilityZone,
	}

	log.Printf("server response from az: %s\n", res.AvailabilityZone)

	return res, nil
}

// metadata holds metadata of a Fargate task
type metadata struct {
	AvailabilityZone string `json:"AvailabilityZone"`
	Cluster          string `json:"Cluster"`
	TaskArn          string `json:"TaskArn"`
	Family           string `json:"Family"`
	Revision         string `json:"Revision"`
}

// queryMetadata populates the metadata struct from runtime information
func queryMetadata() metadata {
	mdUrl := os.Getenv("ECS_CONTAINER_METADATA_URI_V4")
	if mdUrl == "" {
		return metadata{
			AvailabilityZone: "az-localhost",
			Cluster:          "no-cluster",
			TaskArn:          "no-task-arn",
			Family:           "no-task-family",
			Revision:         "0",
		}
	}

	res, err := http.Get(fmt.Sprintf("%s/%s", mdUrl, "task"))
	if err != nil {
		log.Fatalf("HTTP Get on metadata endpoint failed: %v\n", err)
	}

	jsonBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatalf("could not read body: %v\n", err)
	}

	md := metadata{}
	err = json.Unmarshal(jsonBytes, &md)
	if err != nil {
		log.Fatalf("could not parse json from body: %v\n", err)
	}

	return md
}
