# Using AWS Cloud Map as gRPC name resolver

This repository show how AWS Cloud Map can be used as a name resolver in gRPC clients. Using the AWS Cloud Map HTTP API instead of DNS based resolution allows to create custom client-side loadbalancing policies leveraging the metadata associated with registered instances of AWS Cloud Map services. 

Potential benefits of this approach are:
* Cutting out one or more loadbalancers which would be fronting Fargate services. This reduces cost, removes a network hop and may reduce efforts to maintain the loadbalancer. 
* Options for advanced loadbalancing scenarios. This sample shows, as an example, how AWS Cloud Map metadata can be used to prefer calling service instances which reside in the same az as the client.  

## Running the sample 

Build the client and server:

```bash
#---
cd client
# generate gRPC code 
mkdir pb
go generate 
# build executable
GOOS=linux GOARCH=amd64 go build -o client .

#---
cd server
# generate gRPC code 
mkdir pb
go generate 
# build executable
GOOS=linux GOARCH=amd64 go build -o server .
```

Provision the CDK app. Make sure docker is running as the Docker images are built as CDK assets during CDK app execution.

Account and Region will be picked up during CDK app execution.

**Important**: Region **must** be eu-central-1 currently, as this region is used in the coding.

```bash
cd cdk
cdk deploy
```

## Create sample data

After the CDK app is deployed, test drive with a couple of calls. It is best to use tools like `hey` or `ab` to provide a large enough number of calls to see the loadbalancing preference show up.

Obtain the Public IP of the single task of the client service from the ECS console

```bash
# with curl
curl http://<pubip>:8080/describe

# with hey, runs a total of 200 requests by default over 10s
hey -c 2 -q 10 -m GET http://<pubip>:8080/describe
# this will take a couple of seconds to run without giving an output 
```

## Observe custom loadbalancing policy 

We route approx 50% of requests to the same availability zone (see balancer.go:53).

Use CloudWatch Insights for the log group and run the following query on log group `grpcdemo`:

```
fields @timestamp, @message
| limit 1000
| filter @message like 'server response from az'
| parse 'server response from az: *' as az
| stats count(az) as az_calls by az
```

**Be sure to allow a couple of minutes for all logsto arrive, otherwise the count will not be correct.**

You should see approx. 50% of calls going to the AZ in which the single task of service `client` is located in.