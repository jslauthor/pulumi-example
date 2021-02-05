package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

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

		path, err := os.Getwd()

		// Create a utility function to generate Kafka brokers
		createKakfaContainer := func(brokerId uint8, port uint16) (*docker.Container, error) {
			pulumiName := fmt.Sprintf("kafka-broker%d-container", brokerId)
			log.Print(pulumi.String(filepath.Join(path, "/secrets/keystore/kafka.keystore.jks")))
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

					pulumi.String(fmt.Sprintf("KAFKA_LISTENERS=PLAINTEXT://:%d", port)),
					pulumi.String(fmt.Sprintf("KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://127.0.0.1:%d", port)),
					// pulumi.String(fmt.Sprintf("KAFKA_CFG_ADVERTISED_LISTENERS=INTERNAL://%s:%d,CLIENT://%s:%d", pulumiName, port+1, pulumiName, port)),
					// pulumi.String("KAFKA_CFG_INTER_BROKER_LISTENER_NAME=INTERNAL"),
					// pulumi.String("KAFKA_CFG_SASL_ENABLED_MECHANISMS=PLAIN"),
					// pulumi.String("KAFKA_CFG_SASL_MECHANISM_INTER_BROKER_PROTOCOL=PLAIN"),

					// We don't care if the brokers don't use SSL/auth to connect to Zookeeper
					// since MSK abstracts that away in production (no need to replicate it locally)
					pulumi.String("KAFKA_CFG_ZOOKEEPER_CONNECT=zk-container-pulumi:2181"), // relies on the docker network
					pulumi.String("ALLOW_PLAINTEXT_LISTENER=yes"),                         // no auth at all

					// Login setup
					// pulumi.String("KAFKA_CFG_SSL_ENDPOINT_IDENTIFICATION_ALGORITHM=''"),
					// pulumi.String("KAFKA_CLIENT_USER=pulumiLocal"),
					// pulumi.String("KAFKA_CLIENT_PASSWORD=m$lk#421jas!vd3d4"),
					// pulumi.String("KAFKA_CERTIFICATE_PASSWORD=hfB2290XZdm&$"),
					// pulumi.String("KAFKA_CFG_TLS_TYPE=JKS"),
				},
				// Volumes: docker.ContainerVolumeArray{
				// 	docker.ContainerVolumeArgs{
				// 		HostPath:      pulumi.String(filepath.Join(path, "/secrets/keystore2/kafka.keystore.jks")),
				// 		ContainerPath: pulumi.String("/opt/bitnami/kafka/config/certs/kafka.keystore.jks"),
				// 		ReadOnly:      pulumi.Bool(true),
				// 	},
				// 	docker.ContainerVolumeArgs{
				// 		HostPath:      pulumi.String(filepath.Join(path, "/secrets/truststore2/kafka.truststore.jks")),
				// 		ContainerPath: pulumi.String("/opt/bitnami/kafka/config/certs/kafka.truststore.jks"),
				// 		ReadOnly:      pulumi.Bool(true),
				// 	},
				// },
			}, pulumi.DependsOn([]pulumi.Resource{zookeeperContainer}))
		}

		// Create first Kafka Broker
		_, err = createKakfaContainer(1, 9092)

		// // Create second Kafka Broker
		// _, err = createKakfaContainer(2, 9093)

		// // Create the third Kafka Broker
		// _, err = createKakfaContainer(3, 9094)

		ctx.Export("MATERIALIZE_URL", pulumi.String("localhost:6875"))

		return nil
	})

}
