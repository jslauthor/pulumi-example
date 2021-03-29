package main

import (
	"fmt"

	"github.com/jslauthor/infra/pkg/kafka"
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
		// mtzImage, err := docker.NewRemoteImage(ctx, "mtz-image", &docker.RemoteImageArgs{
		// 	Name:        pulumi.String("materialize/materialized:v0.6.1"),
		// 	KeepLocally: pulumi.Bool(true),
		// })
		// if err != nil {
		// 	return err
		// }

		// // Create container and start it
		// _, err = docker.NewContainer(ctx, "mtz-container", &docker.ContainerArgs{
		// 	Name:  pulumi.String("mtz-container-pulumi"),
		// 	Image: mtzImage.Name,
		// 	NetworksAdvanced: docker.ContainerNetworksAdvancedArray{
		// 		docker.ContainerNetworksAdvancedArgs{Name: network.Name},
		// 	},
		// 	Restart: pulumi.String("on-failure"),
		// 	Ports: docker.ContainerPortArray{
		// 		docker.ContainerPortArgs{
		// 			Internal: pulumi.Int(6875),
		// 			External: pulumi.Int(6875),
		// 		},
		// 	},
		// 	Command: pulumi.StringArray{
		// 		pulumi.String("-w"), // workers
		// 		pulumi.String("1"),
		// 	},
		// })
		// if err != nil {
		// 	return err
		// }

		/*
		** Set up two Kafka brokers + Zookeeper
		 */

		// Retrieve Zookeeper image
		zookeperImage, err := docker.NewRemoteImage(ctx, "zookeeper-image", &docker.RemoteImageArgs{
			Name:        pulumi.String("bitnami/zookeeper:latest"),
			KeepLocally: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}

		// Retrieve Kafka image
		kafkaImage, err := docker.NewRemoteImage(ctx, "kafka-image", &docker.RemoteImageArgs{
			Name:        pulumi.String("bitnami/kafka:latest"),
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
			},
			Envs: pulumi.StringArray{
				pulumi.String("ZOO_PORT_NUMBER=2181"),
				pulumi.String("ZOO_SERVER_ID=1"),
				pulumi.String("ZOO_TICK_TIME=2000"),
				pulumi.String("ALLOW_ANONYMOUS_LOGIN=yes"), // For development only
			},
		})

		// Create a utility function to generate Kafka brokers
		createKakfaContainer := func(brokerId uint8, port uint16) (*docker.Container, error) {
			pulumiName := fmt.Sprintf("kafka-broker%d-container", brokerId)

			return docker.NewContainer(ctx, pulumiName, &docker.ContainerArgs{
				Name:  pulumi.String(pulumiName),
				Image: kafkaImage.Name,
				NetworksAdvanced: docker.ContainerNetworksAdvancedArray{
					docker.ContainerNetworksAdvancedArgs{Name: network.Name},
				},
				Restart: pulumi.String("on-failure"),
				// Restart: pulumi.String("no"),
				Ports: docker.ContainerPortArray{
					docker.ContainerPortArgs{
						Internal: pulumi.Int(port),
						External: pulumi.Int(port),
					},
				},
				Envs: pulumi.StringArray{
					pulumi.String(fmt.Sprintf("KAFKA_BROKER_ID=%d", brokerId)),

					// We don't care if the brokers don't use SSL/auth to connect to Zookeeper
					// since MSK abstracts that away in production (no need to replicate it locally)
					pulumi.String("KAFKA_CFG_ZOOKEEPER_CONNECT=zk-container-pulumi:2181"), // relies on the docker network
					pulumi.String("ALLOW_PLAINTEXT_LISTENER=yes"),

					// Set up internal (other kf brokers) vs external (app servers) listeners
					// No auth to keep things simple
					pulumi.String(fmt.Sprintf("KAFKA_LISTENERS=INTERNAL://%s:9088,EXTERNAL://:%d", pulumiName, port)),
					pulumi.String(fmt.Sprintf("KAFKA_ADVERTISED_LISTENERS=INTERNAL://%s:9088,EXTERNAL://localhost:%d", pulumiName, port)),
					pulumi.String("KAFKA_INTER_BROKER_LISTENER_NAME=INTERNAL"),
					pulumi.String("KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=INTERNAL:PLAINTEXT,EXTERNAL:PLAINTEXT"),
				},
			}, pulumi.DependsOn([]pulumi.Resource{zookeeperContainer}))
		}

		// Create first Kafka Broker
		kafka1Broker, err := createKakfaContainer(1, 9092)

		// // Create second Kafka Broker
		kafka2Broker, err := createKakfaContainer(2, 9093)

		// // Create the third Kafka Broker
		kafka3Broker, err := createKakfaContainer(3, 9094)

		kafka1Broker.IpAddress.ApplyString(func(ip string) (string, error) {
			/*
			** Configure Kafka Topics
			 */

			err = kafka.ConfigureKafkaTopics(
				ctx,
				"localhost:9092,localhost:9093,localhost:9094",
				"PLAINTEXT",
				nil,
			)
			return ip, err
		})

		/* Set up Confluent Inc's Kafka REST Proxy */

		restProxyImage, err := docker.NewRemoteImage(ctx, "kafka-rest-proxy", &docker.RemoteImageArgs{
			Name:        pulumi.String("confluentinc/cp-kafka-rest"),
			KeepLocally: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}

		_, err = docker.NewContainer(ctx, "kafka-rest-proxy-container", &docker.ContainerArgs{
			Name:  pulumi.String("kafka-rest-proxy-pulumi"),
			Image: restProxyImage.Name,
			NetworksAdvanced: docker.ContainerNetworksAdvancedArray{
				docker.ContainerNetworksAdvancedArgs{Name: network.Name},
			},
			Restart: pulumi.String("on-failure"),
			Ports: docker.ContainerPortArray{
				docker.ContainerPortArgs{
					Internal: pulumi.Int(8082),
					External: pulumi.Int(8082),
				},
			},
			Envs: pulumi.StringArray{
				// pulumi.String("KAFKA_REST_ZOOKEEPER_CONNECT=PLAINTEXT://zk-container-pulumi:2181"),
				// pulumi.String("KAFKA_REST_HOST_NAME=localhost"),
				pulumi.String("KAFKA_REST_ID=1"),
				pulumi.String("KAFKA_REST_BOOTSTRAP_SERVERS=PLAINTEXT://kafka-broker1-container:9088"),
				// pulumi.String("KAFKA_REST_LISTENERS=http://localhost:8082"),
			},
		}, pulumi.DependsOn([]pulumi.Resource{zookeeperContainer, kafka1Broker, kafka2Broker, kafka3Broker}))

		/*
		** Export variables for use in .env and secrets
		 */

		// ctx.Export("MATERIALIZE_URL", pulumi.String("localhost:6875"))
		ctx.Export("KAFKA_BROKERS", pulumi.String("localhost:9092,localhost:9093,localhost:9094"))

		return nil
	})

}
