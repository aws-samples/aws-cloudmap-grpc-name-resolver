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
	"log"
	"math/rand"
	"time"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
)

// BalancerName is used for registration and lookup of the loadbalancer
const BalancerName = "cm-az-aware"

// NewBalancerBuilder provides a new balancer.Builder to configure gRPC channels
func NewBalancerBuilder() balancer.Builder {
	return base.NewBalancerBuilder(BalancerName, &Picker{}, base.Config{HealthCheck: false})
}

// Picker implements the balancer.Picker interface as well as the PickerBuilder interface.
//
// With PickerBuilder a new Picker instance can be constructed, which will hold SubConns. Within
// the balancer.Picker#Pick method the actual SubConn for a gRPC call is selected.
type Picker struct {
	subConns []subConn
}

// subConn combines a balancer.SubConn with metadata handed down from the name resolver.
type subConn struct {
	sc balancer.SubConn
	az string
}

// Pick selects a SubConn and is biased towards SubConns in the same AZ as the current task.
func (p *Picker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	az := queryAvailabilityZone()

	var selected balancer.SubConn
	sameAz := make([]balancer.SubConn, 0, len(p.subConns))
	otherAz := make([]balancer.SubConn, 0, len(p.subConns))

	// split subConns in lists for same AZ and different AZ
	for _, conn := range p.subConns {
		if az == conn.az {
			sameAz = append(sameAz, conn.sc)
		} else {
			otherAz = append(otherAz, conn.sc)
		}
	}

	// coin flip for same AZ loadbalancing
	// productive use cases may implement more sophisticated strategies
	useSameAz := rand.Int()%2 == 0

	if useSameAz {
		idx := rand.Intn(len(sameAz))
		selected = sameAz[idx]
	} else {
		idx := rand.Intn(len(otherAz))
		selected = otherAz[idx]
	}

	// safety net if no subConn has been selected due to incorrect config or other issues
	if selected == nil {
		log.Println("could not decide on subConn to use. falling back to subConn at idx 0")
		selected = p.subConns[0].sc
	}

	return balancer.PickResult{
		SubConn: selected,
	}, nil
}

// Build creates a new balancer.Picker and initializes it.
func (p *Picker) Build(info base.PickerBuildInfo) balancer.Picker {

	// extract metadata from the provided SubConns and carry it forward
	// for use in the created Picker
	p.subConns = make([]subConn, 0, len(info.ReadySCs))
	for sc, sci := range info.ReadySCs {
		preparedSubconn := subConn{
			sc: sc,
			// populated in CloudmapResolver#resolveForTesting
			az: sci.Address.BalancerAttributes.Value("az").(string),
		}
		p.subConns = append(p.subConns, preparedSubconn)
	}

	// re-seed the pseudo rng so we can have random guesses in Pick
	// if we want same az loadbalancing
	rand.Seed(time.Now().Unix())

	return p
}
