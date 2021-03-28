package kafka

import (
	"strings"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

// SaslOpts holds SASL options
type SaslOpts struct {
	mechanism string
	username  string
	password  string
}

// ConfigureKafkaTopics establishes all of the Kafka topics that our app servers use
func ConfigureKafkaTopics(ctx *pulumi.Context, bootstrap string, protocol string, saslOpts *SaslOpts) error {

	/*
	** Create Admin
	 */

	kafkaConfig := kafka.ConfigMap{
		"bootstrap.servers": bootstrap,
		"security.protocol": protocol,
	}

	if strings.Contains("protocol", "sasl") {
		kafkaConfig["sasl.mechanism"] = saslOpts.mechanism
		kafkaConfig["sasl.username"] = saslOpts.username
		kafkaConfig["sasl.password"] = saslOpts.password
	}

	_, err := kafka.NewAdminClient(&kafkaConfig)
	if err != nil {
		return err
	}

	/*
	** Create Topics
	 */

	return nil
}
