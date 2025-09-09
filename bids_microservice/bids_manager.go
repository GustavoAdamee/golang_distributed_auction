package main

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type BidsManager struct {
	client_keys_pool          map[string]interface{} // map[clientId]publicKey
	auction_pool              map[string]interface{} // map[auctionId]auction
	highest_bids_pool         map[string]interface{} // map[auctionId]highestBid
	conn                      *amqp.Connection
	channel                   *amqp.Channel
	public_keys_queue         amqp.Queue
	clients_public_keys       <-chan amqp.Delivery
	auction_started_queue     amqp.Queue
	auction_started_messages  <-chan amqp.Delivery
	auction_finished_queue    amqp.Queue
	auction_finished_messages <-chan amqp.Delivery
	bids_queue                amqp.Queue
	bids_messages             <-chan amqp.Delivery
	bids_context              context.Context
	bids_cancel               context.CancelFunc
}

// AuctionStatus represents the status of an auction
type AuctionStatus string

// Define the possible auction statuses
const (
	StatusIdle     AuctionStatus = "idle"
	StatusActive   AuctionStatus = "active"
	StatusFinished AuctionStatus = "finished"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Panicf("%s: %s", msg, err)
	}
}

func (b *BidsManager) Init() {
	// Initialize the maps
	b.client_keys_pool = make(map[string]interface{})
	b.auction_pool = make(map[string]interface{})
	b.highest_bids_pool = make(map[string]interface{})

	var err error
	b.conn, err = amqp.Dial("amqp://guest:guest@localhost:5672/")
	failOnError(err, "Failed to connect to RabbitM!")

	b.channel, err = b.conn.Channel()
	failOnError(err, "Failed to open a channel")

	err = b.channel.ExchangeDeclare(
		"public_keys_exchange", // name
		"direct",               // type
		true,                   // durable
		false,                  // auto-deleted
		false,                  // internal
		false,                  // no-wait
		nil,                    // arguments
	)
	failOnError(err, "Failed to create exchange")

	b.public_keys_queue, err = b.channel.QueueDeclare(
		"",    // name
		false, // durable
		true,  // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // argument
	)
	failOnError(err, "Failed to declare a queue")

	err = b.channel.QueueBind(
		b.public_keys_queue.Name, // name
		"public_key",             // routing key
		"public_keys_exchange",   // exchange
		false,                    // no-wait
		nil,                      // arguments
	)
	failOnError(err, "Failed to bind a queue")

	//started auction
	err = b.channel.ExchangeDeclare(
		"auction_started_exchange", // name
		"fanout",                   // type
		true,                       // durable
		false,                      // auto-deleted
		false,                      // internal
		false,                      // no-wait
		nil,                        // arguments
	)
	failOnError(err, "Failed to create exchange")

	b.auction_started_queue, err = b.channel.QueueDeclare(
		"",    // name (empty = auto-generated unique name)
		false, // durable
		true,  // delete when unused (auto-delete when client disconnects)
		true,  // exclusive (only this connection can access)
		false, // no-wait
		nil,   // argument
	)
	failOnError(err, "Failed to declare a queue")

	err = b.channel.QueueBind(
		b.auction_started_queue.Name, // name
		"",                           // routing key
		"auction_started_exchange",   // exchange
		false,                        // no-wait
		nil,                          // arguments
	)
	failOnError(err, "Failed to bind a queue")

	//finished auction
	err = b.channel.ExchangeDeclare(
		"auction_finished_exchange", // name
		"direct",                    // type
		true,                        // durable
		false,                       // auto-deleted
		false,                       // internal
		false,                       // no-wait
		nil,                         // arguments
	)
	failOnError(err, "Failed to create exchange")

	b.auction_finished_queue, err = b.channel.QueueDeclare(
		"",    // name
		false, // durable
		true,  // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // argument
	)
	failOnError(err, "Failed to declare a queue")

	err = b.channel.QueueBind(
		b.auction_finished_queue.Name,  // name
		"auction_finished_routing_key", // routing key
		"auction_finished_exchange",    // exchange
		false,                          // no-wait
		nil,                            // arguments
	)
	failOnError(err, "Failed to bind a queue")

	//bids exchange
	err = b.channel.ExchangeDeclare(
		"bids_exchange", // name
		"direct",        // type
		true,            // durable
		false,           // auto-deleted
		false,           // internal
		false,           // no-wait
		nil,             // arguments
	)
	failOnError(err, "Failed to create exchange")

	b.bids_queue, err = b.channel.QueueDeclare(
		"",    // name
		false, // durable
		true,  // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // argument
	)
	failOnError(err, "Failed to declare a queue")

	err = b.channel.QueueBind(
		b.bids_queue.Name, // name
		"bid_routing_key", // routing key
		"bids_exchange",   // exchange
		false,             // no-wait
		nil,               // arguments
	)
	failOnError(err, "Failed to bind a queue")

	//valid bids exchange
	err = b.channel.ExchangeDeclare(
		"valid_bids_exchange", // name
		"direct",              // type
		true,                  // durable
		false,                 // auto-deleted
		false,                 // internal
		false,                 // no-wait
		nil,                   // arguments
	)
	failOnError(err, "Failed to create exchange")

	//winner auction exchange
	err = b.channel.ExchangeDeclare(
		"winner_auction_exchange", // name
		"direct",                  // type
		true,                      // durable
		false,                     // auto-deleted
		false,                     // internal
		false,                     // no-wait
		nil,                       // arguments
	)
	failOnError(err, "Failed to create exchange")

	b.bids_context, b.bids_cancel = context.WithTimeout(context.Background(), 10*time.Second)
}

func (b *BidsManager) listen_to_clients_public_keys() {
	var err error
	b.clients_public_keys, err = b.channel.Consume(
		b.public_keys_queue.Name, // queue name
		"",                       // consumer
		true,                     // auto-ack
		false,                    // exclusive
		false,                    // no-local
		false,                    // no-wait
		nil,                      // args
	)
	failOnError(err, "Failed to register a consumer on the queue")
	go func() {
		for d := range b.clients_public_keys {
			log.Printf("Received public key message: %s", d.Body)
			var body map[string]interface{}
			err := json.Unmarshal(d.Body, &body)
			if err != nil {
				log.Fatalf("%s", err)
			}
			clientId := body["id"].(string)
			b.client_keys_pool[clientId] = body["public_key"].(string)
		}
	}()
}

func (b *BidsManager) listen_to_started_auctions() {
	var err error
	b.auction_started_messages, err = b.channel.Consume(
		b.auction_started_queue.Name, // queue name
		"",                           // consumer
		true,                         // auto-ack
		false,                        // exclusive
		false,                        // no-local
		false,                        // no-wait
		nil,                          // args
	)
	failOnError(err, "Failed to register a consumer on the queue")
	go func() {
		for d := range b.auction_started_messages {
			log.Printf("Received a message: %s", d.Body)
			var body map[string]interface{}
			err := json.Unmarshal(d.Body, &body)
			if err != nil {
				log.Fatalf("%s", err)
			}
			var auction_body map[string]interface{} = make(map[string]interface{})
			auction_body["description"] = body["description"]
			auction_body["Start_time"] = body["Start_time"]
			auction_body["End_time"] = body["End_time"]
			auction_body["Status"] = body["Status"]
			b.auction_pool[body["id"].(string)] = auction_body
		}
	}()
}

func (b *BidsManager) listen_to_finished_auctions() {
	var err error
	b.auction_finished_messages, err = b.channel.Consume(
		b.auction_finished_queue.Name, // queue name
		"",                            // consumer
		true,                          // auto-ack
		false,                         // exclusive
		false,                         // no-local
		false,                         // no-wait
		nil,                           // args
	)
	failOnError(err, "Failed to register a consumer on the queue")
	go func() {
		for d := range b.auction_finished_messages {
			log.Printf("Received auction finished message")
			var body map[string]interface{}
			err := json.Unmarshal(d.Body, &body)
			failOnError(err, "Failed to unmarshal body")
			id, _ := body["id"].(string)
			auction, _ := b.auction_pool[id]
			auction.(map[string]interface{})["Status"] = StatusFinished
			b.send_winner_auction(id)
		}
	}()
}

func (b *BidsManager) listen_to_bids() {
	var err error
	b.bids_messages, err = b.channel.Consume(
		b.bids_queue.Name, // queue name
		"",                // consumer
		true,              // auto-ack
		false,             // exclusive
		false,             // no-local
		false,             // no-wait
		nil,               // args
	)
	failOnError(err, "Failed to register a consumer on the queue")
	go func() {
		for d := range b.bids_messages {
			// log.Printf("Received a message: %s", d.Body)
			var body map[string]interface{}
			err := json.Unmarshal(d.Body, &body)
			if err != nil {
				log.Fatalf("%s", err)
			}
			b.handle_bid(body)
		}
	}()
}

func parse_public_key_from_pem(pemData string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the public key")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("not an RSA public key")
	}

	return rsaPub, nil
}

func verify_signature(signature []byte, hashed_message []byte, public_key *rsa.PublicKey) bool {
	err := rsa.VerifyPKCS1v15(public_key, crypto.SHA256, hashed_message, signature)
	if err != nil {
		return false
	}
	return true
}

func (b *BidsManager) handle_bid(bid map[string]interface{}) {
	log.Printf("Received a bid: %s", bid)
	// test if the bid is valid by checking the signature, checking the auction id and the client id
	bid_auction_id, ok := bid["auction_id"].(string)
	if !ok {
		log.Printf("Invalid auction id type in bid")
		return
	}
	bid_client_id, ok := bid["client_id"].(string)
	if !ok {
		log.Printf("Invalid client id type in bid")
		return
	}
	bid_signature_str, ok := bid["signature"].(string)
	if !ok {
		log.Printf("Invalid signature type in bid")
		return
	}

	// Decode base64 string back to bytes
	bid_signature, decodeErr := base64.StdEncoding.DecodeString(bid_signature_str)
	if decodeErr != nil {
		log.Printf("Error decoding signature from base64: %s", decodeErr)
		return
	}
	auction, exists := b.auction_pool[bid_auction_id]
	if !exists {
		log.Printf("Auction with id %s not found in pool", bid_auction_id)
		return
	}
	if auction.(map[string]interface{})["Status"] != "active" {
		log.Printf("Auction with id %s is not active", bid_auction_id)
		log.Printf("Auction status: %s", auction.(map[string]interface{})["Status"])
		return
	}
	publicKeyPEM, exists := b.client_keys_pool[bid_client_id]
	if !exists {
		log.Printf("Client with id %s not found in pool", bid_client_id)
		return
	}

	publicKey, err := parse_public_key_from_pem(publicKeyPEM.(string))
	if err != nil {
		log.Printf("Error parsing public key: %s", err)
		return
	}

	originalBid := map[string]interface{}{
		"bid":        bid["bid"],
		"client_id":  bid["client_id"],
		"auction_id": bid["auction_id"],
	}
	originalMessage, err := json.Marshal(originalBid)
	if err != nil {
		log.Printf("Error marshaling original message: %s", err)
		return
	}

	hash := sha256.New()
	hash.Write(originalMessage)
	hashedMessage := hash.Sum(nil)

	// Verify the signature
	if verify_signature(bid_signature, hashedMessage, publicKey) {
		highest_bid, exists := b.highest_bids_pool[bid_auction_id]
		bid["bid"], _ = strconv.Atoi(bid["bid"].(string))
		json_bid := map[string]interface{}{
			"bid":        bid["bid"],
			"client_id":  bid["client_id"],
			"auction_id": bid["auction_id"],
		}
		if !exists {
			b.highest_bids_pool[bid_auction_id] = json_bid
			b.send_valid_bid(json_bid)
		} else {
			if bid["bid"].(int) > highest_bid.(map[string]interface{})["bid"].(int) {
				b.highest_bids_pool[bid_auction_id] = json_bid
				b.send_valid_bid(json_bid)
			} else {
				log.Printf("Bid from client %s is not the highest bid for auction %s", bid_client_id, bid_auction_id)
				log.Printf("Highest bid: %s", highest_bid)
				return
			}
		}
	} else {
		log.Printf("Invalid signature for bid from client %s", bid_client_id)
	}
}

func (b *BidsManager) send_valid_bid(bid map[string]interface{}) {
	json_body, err := json.Marshal(bid)
	if err != nil {
		log.Printf("Error marshaling bid: %s", err)
		return
	}
	err = b.channel.PublishWithContext(b.bids_context,
		"valid_bids_exchange",   // exchange
		"valid_bid_routing_key", // routing key
		false,                   // mandatory
		false,                   // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        json_body,
		})
	failOnError(err, "Failed to publish valid bid")
}

func (b *BidsManager) send_winner_auction(auction_id string) {
	winner_bid, exists := b.highest_bids_pool[auction_id]
	if !exists {
		log.Printf("Winner bid not found for auction %s", auction_id)
		return
	}
	json_body, err := json.Marshal(winner_bid)
	if err != nil {
		log.Printf("Error marshaling winner bid: %s", err)
		return
	}
	err = b.channel.PublishWithContext(b.bids_context,
		"winner_auction_exchange",    // exchange
		"winner_auction_routing_key", // routing key
		false,                        // mandatory
		false,                        // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        json_body,
		})
	failOnError(err, "Failed to publish winner auction")
}

func (b *BidsManager) close() {
	if b.channel != nil {
		b.channel.Close()
	}
	if b.conn != nil {
		b.conn.Close()
	}
}

func main() {
	var bids_manager = BidsManager{}
	bids_manager.Init()
	defer bids_manager.close()
	bids_manager.listen_to_clients_public_keys()
	bids_manager.listen_to_started_auctions()
	bids_manager.listen_to_bids()
	bids_manager.listen_to_finished_auctions()
	select {}
}
