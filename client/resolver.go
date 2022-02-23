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
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"

	"github.com/aws/aws-sdk-go-v2/config"
	sd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
)

// CloudmapResolver implements both a resolver.Builder and resolver.Resolver
type CloudmapResolver struct {
	service    string
	namespace  string
	clientConn resolver.ClientConn
}

// ResolveNow triggers the actual name resolution.
func (r CloudmapResolver) ResolveNow(options resolver.ResolveNowOptions) {

	// we default to eu-central-1 in this demo
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithDefaultRegion("eu-central-1"))
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	svc := sd.NewFromConfig(cfg)

	res, err := svc.DiscoverInstances(context.Background(), &sd.DiscoverInstancesInput{
		NamespaceName: aws.String(r.namespace),
		ServiceName:   aws.String(r.service),
	})
	if err != nil {
		log.Fatalf("error in DiscoverInstances: %v\n", err)
	}

	// extract metadata of registered instances, i.e. the availability zone
	// for Fargate tasks with service discovery ECS will automatically create those
	// attributes
	//
	// the availability zone is then put into BalancerAttributes of the
	// resolver.Address struct, so the balancer.PickerBuilder can access
	// and use them
	addr := make([]resolver.Address, 0, len(res.Instances))
	for i := range res.Instances {
		instAtt := res.Instances[i].Attributes
		add := fmt.Sprintf("%s:%s", instAtt["AWS_INSTANCE_IPV4"], "9000")

		log.Printf("resolved target: %v", add)

		balancerAtts := attributes.New("az", instAtt["AVAILABILITY_ZONE"])
		addr = append(addr, resolver.Address{
			Addr:               add,
			BalancerAttributes: balancerAtts,
		})
	}

	r.clientConn.UpdateState(resolver.State{
		Addresses: addr,
	})
}

func (r CloudmapResolver) Close() {
	// nothing to do
}

// Build returns a new instance of a resolver.Resolver
//
// target is split on the first "." into a service name with the remainder as the namespace.
// Example: my.name.local will resolve to service "my" and namespace "name.local"
func (r CloudmapResolver) Build(
	target resolver.Target,
	cc resolver.ClientConn,
	opts resolver.BuildOptions,
) (resolver.Resolver, error) {
	if target.URL.Scheme != "cloudmap" {
		return nil, errors.New("error: unsupported scheme in service discovery. Must be cloudmap")
	}
	comps := strings.SplitN(target.URL.Host, ".", 2)

	cmResolver := &CloudmapResolver{
		clientConn: cc,
		service:    comps[0],
		namespace:  comps[1],
	}

	cmResolver.ResolveNow(resolver.ResolveNowOptions{})

	return cmResolver, nil
}

// Scheme always returns "cloudmap"
func (r CloudmapResolver) Scheme() string {
	return "cloudmap"
}
