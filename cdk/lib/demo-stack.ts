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
import {RemovalPolicy, Stack, StackProps} from 'aws-cdk-lib';
import {Construct} from 'constructs';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import {Peer, Port, SubnetType} from 'aws-cdk-lib/aws-ec2';
import * as ecs from 'aws-cdk-lib/aws-ecs';
import {FargatePlatformVersion, LogDriver} from 'aws-cdk-lib/aws-ecs';
import * as ecra from 'aws-cdk-lib/aws-ecr-assets';
import {NamespaceType} from "aws-cdk-lib/aws-servicediscovery";
import {LogGroup} from "aws-cdk-lib/aws-logs";
import {Effect, PolicyStatement} from "aws-cdk-lib/aws-iam";

export class DemoStack extends Stack {
    constructor(scope: Construct, id: string, props?: StackProps) {
        super(scope, id, props);

        const vpc = new ec2.Vpc(this, "vpc", {
            vpcName: "demovpc",
            maxAzs: 3,
            natGateways: 0,
            subnetConfiguration: [{
                subnetType: SubnetType.PUBLIC,
                name: "public"
            }]
        });

        const cluster = new ecs.Cluster(this, "democluster", {
            vpc,
            clusterName: "democluster",
            defaultCloudMapNamespace: {
                name: "grpc.demo",
                type: NamespaceType.HTTP
            }
        });

        const clientSg = new ec2.SecurityGroup(this, "client-sg", {
            vpc,
            securityGroupName: "clientSecurityGroup",
            allowAllOutbound: true
        });
        clientSg.addIngressRule(Peer.anyIpv4(), Port.tcp(8080));

        const serverSg = new ec2.SecurityGroup(this, "server-sg", {
            vpc,
            securityGroupName: "serverSecurityGroup"
        });
        serverSg.addIngressRule(clientSg, Port.allTcp())

        const loggroup = new LogGroup(this, "loggroup", {
            logGroupName: "grpcdemo",
            retention: 3,
            removalPolicy: RemovalPolicy.DESTROY
        });

        const serverImage = new ecra.DockerImageAsset(this, "serverimage", {
            directory: "./../server",
        });

        const serverTask = new ecs.FargateTaskDefinition(this, "serverTask", {
            cpu: 512,
            memoryLimitMiB: 1024,
            family: "server",
        });
        serverTask.addContainer("main", {
            image: ecs.ContainerImage.fromDockerImageAsset(serverImage),
            logging: LogDriver.awsLogs({
                logGroup: loggroup,
                streamPrefix: "server"
            })
        });

        const serverService = new ecs.FargateService(this, "serverSvc", {
            cluster,
            assignPublicIp: true,
            serviceName: "server",
            cloudMapOptions: {
                name: "server",
                containerPort: 9000
            },
            taskDefinition: serverTask,
            desiredCount: 3,
            platformVersion: FargatePlatformVersion.VERSION1_4,
            securityGroups: [serverSg],
        });

        const clientImage = new ecra.DockerImageAsset(this, "clientimage", {
            directory: "./../client",
        });

        const clientTask = new ecs.FargateTaskDefinition(this, "clientTask", {
            cpu: 512,
            memoryLimitMiB: 1024,
            family: "client",
        });
        clientTask.addContainer("main", {
            image: ecs.ContainerImage.fromDockerImageAsset(clientImage),
            portMappings: [
                {
                    containerPort: 8080
                }
            ],
            logging: LogDriver.awsLogs({
                logGroup: loggroup,
                streamPrefix: "client"
            })
        });
        clientTask.addToTaskRolePolicy(new PolicyStatement({
            effect: Effect.ALLOW,
            actions: [
                "servicediscovery:DiscoverInstances"
            ],
            resources: ["*"]
        }));

        const clientService = new ecs.FargateService(this, "clientSvc", {
            cluster,
            assignPublicIp: true,
            serviceName: "client",
            taskDefinition: clientTask,
            desiredCount: 1,
            platformVersion: FargatePlatformVersion.VERSION1_4,
            securityGroups: [clientSg]
        });

        clientService.node.addDependency(serverService);
    }
}
