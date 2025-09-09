package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type NotificationManager struct {
	conn                    *amqp.Connection
	channel                 *amqp.Channel
	winner_auction_queue    amqp.Queue
	winner_auction_messages <-chan amqp.Delivery
	valid_bids_queue        amqp.Queue
	valid_bids_messages     <-chan amqp.Delivery
	context                 context.Context
	cancel                  context.CancelFunc
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Panicf("%s: %s", msg, err)
	}
}

func (n *NotificationManager) Init() {
	var err error
	n.conn, err = amqp.Dial("amqp://guest:guest@localhost:5672/")
	failOnError(err, "Failed to connect to RabbitMQ!")

	n.channel, err = n.conn.Channel()
	failOnError(err, "Failed to open a channel")

	//notification auction exchange
	err = n.channel.ExchangeDeclare(
		"notification_auction_exchange", // name
		"topic",                         // type
		true,                            // durable
		false,                           // auto-deleted
		false,                           // internal
		false,                           // no-wait
		nil,                             // arguments
	)
	failOnError(err, "Failed to create exchange")

	//winner auction exchange
	err = n.channel.ExchangeDeclare(
		"winner_auction_exchange", // name
		"direct",                  // type
		true,                      // durable
		false,                     // auto-deleted
		false,                     // internal
		false,                     // no-wait
		nil,                       // arguments
	)
	failOnError(err, "Failed to create exchange")

	//valid bids exchange
	err = n.channel.ExchangeDeclare(
		"valid_bids_exchange", // name
		"direct",              // type
		true,                  // durable
		false,                 // auto-deleted
		false,                 // internal
		false,                 // no-wait
		nil,                   // arguments
	)
	failOnError(err, "Failed to create exchange")

	//winner auction queue
	n.winner_auction_queue, err = n.channel.QueueDeclare(
		"",    // name
		false, // durable
		true,  // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // argument
	)
	failOnError(err, "Failed to declare a queue")

	err = n.channel.QueueBind(
		n.winner_auction_queue.Name,  // name
		"winner_auction_routing_key", // routing key
		"winner_auction_exchange",    // exchange
		false,                        // no-wait
		nil,                          // arguments
	)
	failOnError(err, "Failed to bind a queue")

	n.valid_bids_queue, err = n.channel.QueueDeclare(
		"",    // name
		false, // durable
		true,  // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // argument
	)
	failOnError(err, "Failed to declare a queue")

	err = n.channel.QueueBind(
		n.valid_bids_queue.Name, // name
		"valid_bid_routing_key", // routing key
		"valid_bids_exchange",   // exchange
		false,                   // no-wait
		nil,                     // arguments
	)
	failOnError(err, "Failed to bind a queue")

	n.context, n.cancel = context.WithTimeout(context.Background(), 10*time.Second)
}

func (n *NotificationManager) listen_to_winner_auction() {
	var err error
	n.winner_auction_messages, err = n.channel.Consume(
		n.winner_auction_queue.Name, // queue name
		"",                          // consumer
		true,                        // auto-ack
		false,                       // exclusive
		false,                       // no-local
		false,                       // no-wait
		nil,                         // args
	)
	failOnError(err, "Failed to register a consumer on the queue")
	go func() {
		for d := range n.winner_auction_messages {
			log.Printf("Winner auction: %s", d.Body)
			var body map[string]interface{}
			err := json.Unmarshal(d.Body, &body)
			failOnError(err, "Failed to unmarshal body")
			n.publish_notification(body["auction_id"].(string), body, "winner_bid")
		}
	}()
}

func (n *NotificationManager) listen_to_valid_bid() {
	var err error
	n.valid_bids_messages, err = n.channel.Consume(
		n.valid_bids_queue.Name, // queue name
		"",                      // consumer
		true,                    // auto-ack
		false,                   // exclusive
		false,                   // no-local
		false,                   // no-wait
		nil,                     // args
	)
	failOnError(err, "Failed to register a consumer on the queue")
	go func() {
		for d := range n.valid_bids_messages {
			log.Printf("Valid bid: %s", d.Body)
			var body map[string]interface{}
			err := json.Unmarshal(d.Body, &body)
			failOnError(err, "Failed to unmarshal body")
			n.publish_notification(body["auction_id"].(string), body, "new_valid_bid")
		}
	}()
}

func (n *NotificationManager) publish_notification(auction_id string, body map[string]interface{}, event_type string) {
	// add a new key to the body
	body["event_type"] = event_type

	json_body, err := json.Marshal(body)
	failOnError(err, "Failed to marshal body")

	err = n.channel.PublishWithContext(n.context,
		"notification_auction_exchange", // exchange
		auction_id,                      // routing key
		false,                           // mandatory
		false,                           // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        json_body,
		})
	failOnError(err, "Failed to publish notification")
}

func (n *NotificationManager) close() {}

func main() {
	var notification_manager = NotificationManager{}
	notification_manager.Init()
	defer notification_manager.close()
	notification_manager.listen_to_winner_auction()
	notification_manager.listen_to_valid_bid()
	select {}
}
