package main

import (
	"github.com/pulumi/pulumi-docker/sdk/v2/go/docker"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Create shared network for docker images
		network, err := docker.NewNetwork(ctx, "pulumi-network", &docker.NetworkArgs{})
		if err != nil {
			return err
		}

		/*
		** Set up local Materialize container and launch it
		 */

		// Retrieve Materialize image
		mtzImage, err := docker.NewRemoteImage(ctx, "mtz-image", &docker.RemoteImageArgs{
			Name:        pulumi.String("materialize/materialized:v0.6.1"),
			KeepLocally: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}

		// Create container and start it
		_, err = docker.NewContainer(ctx, "mtz-container", &docker.ContainerArgs{
			Name:  pulumi.String("mtz-container-pulumi"),
			Image: mtzImage.Name,
			NetworksAdvanced: docker.ContainerNetworksAdvancedArray{
				docker.ContainerNetworksAdvancedArgs{Name: network.Name},
			},
			Restart: pulumi.String("on-failure"),
			Ports: docker.ContainerPortArray{
				docker.ContainerPortArgs{
					Internal: pulumi.Int(6875),
					External: pulumi.Int(6875),
				},
			},
			Command: pulumi.StringArray{
				pulumi.String("-w"), // workers
				pulumi.String("1"),
			},
		})
		if err != nil {
			return err
		}

		/*
		** Set up two Kafka brokers + Zookeeper
		 */

		// Retrieve Zookeeper image
		zookeperImage, err := docker.NewRemoteImage(ctx, "zookeeper-image", &docker.RemoteImageArgs{
			Name:        pulumi.String("confluentinc/cp-zookeeper:5.5.3"),
			KeepLocally: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}

		// Retrieve Kafka image
		kafkaImage, err := docker.NewRemoteImage(ctx, "kafka-image", &docker.RemoteImageArgs{
			Name:        pulumi.String("confluentinc/cp-server:5.5.3"),
			KeepLocally: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}

		// Create Zookeeper Container
		zookeeperContainer, err := docker.NewContainer(ctx, "zookeeper-container", &docker.ContainerArgs{
			Name:  pulumi.String("zk-container-pulumi"),
			Image: zookeperImage.Name,
			NetworksAdvanced: docker.ContainerNetworksAdvancedArray{
				docker.ContainerNetworksAdvancedArgs{Name: network.Name},
			},
			Restart: pulumi.String("on-failure"),
			Ports: docker.ContainerPortArray{
				docker.ContainerPortArgs{
					Internal: pulumi.Int(2181),
					External: pulumi.Int(2181),
				},
				docker.ContainerPortArgs{
					Internal: pulumi.Int(2182),
					External: pulumi.Int(2182),
				},
			},
			Envs: pulumi.StringArray{
				pulumi.String("ZOOKEEPER_CLIENT_PORT=2181"),
				pulumi.String("ZOOKEEPER_SECURE_CLIENT_PORT=2182"),
				pulumi.String("ZOOKEEPER_TICK_TIME=2000"),
				pulumi.String("ZOOKEEPER_SERVER_CNXN_FACTORY=org.apache.zookeeper.server.NettyServerCnxnFactory"),
			},
		})

		// Create first Kafka Broker
		_, err = docker.NewContainer(ctx, "kafka-broker1-container", &docker.ContainerArgs{
			Name:  pulumi.String("kafka-broker1-pulumi"),
			Image: kafkaImage.Name,
			NetworksAdvanced: docker.ContainerNetworksAdvancedArray{
				docker.ContainerNetworksAdvancedArgs{Name: network.Name},
			},
			Restart: pulumi.String("on-failure"),
			Ports: docker.ContainerPortArray{
				docker.ContainerPortArgs{
					Internal: pulumi.Int(9091),
					External: pulumi.Int(9091),
				},
			},
			Envs: pulumi.StringArray{
				pulumi.String("KAFKA_BROKER_ID=1"),
				pulumi.String("KAFKA_ZOOKEEPER_CONNECT=zk-container-pulumi:2181"),
				pulumi.String("KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=PLAINTEXT:PLAINTEXT,PLAINTEXT_HOST:PLAINTEXT"),
				pulumi.String("KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://kafka-broker1-pulumi:29091,PLAINTEXT_HOST://localhost:9091"),
				pulumi.String("KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1"),
				pulumi.String("KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS=0"),
			},
		}, pulumi.DependsOn([]pulumi.Resource{zookeeperContainer}))

		// Create second Kafka Broker
		_, err = docker.NewContainer(ctx, "kafka-broker2-container", &docker.ContainerArgs{
			Name:  pulumi.String("kafka-broker2-pulumi"),
			Image: kafkaImage.Name,
			NetworksAdvanced: docker.ContainerNetworksAdvancedArray{
				docker.ContainerNetworksAdvancedArgs{Name: network.Name},
			},
			Restart: pulumi.String("on-failure"),
			Ports: docker.ContainerPortArray{
				docker.ContainerPortArgs{
					Internal: pulumi.Int(9092),
					External: pulumi.Int(9092),
				},
			},
			Envs: pulumi.StringArray{
				pulumi.String("KAFKA_BROKER_ID=2"),
				pulumi.String("KAFKA_ZOOKEEPER_CONNECT=zk-container-pulumi:2181"),
				pulumi.String("KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=PLAINTEXT:PLAINTEXT,PLAINTEXT_HOST:PLAINTEXT"),
				pulumi.String("KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://kafka-broker2-pulumi:29092,PLAINTEXT_HOST://localhost:9092"),
				pulumi.String("KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1"),
				pulumi.String("KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS=0"),
			},
		}, pulumi.DependsOn([]pulumi.Resource{zookeeperContainer}))

		ctx.Export("MATERIALIZE_URL", pulumi.String("localhost:6875"))

		return nil
	})

}
