# Distributed Auction System

## Architecture

<img width="5659" height="2541" alt="trabalho_1_sistemas_distribuidos" src="https://github.com/user-attachments/assets/f10cfa18-4b15-4052-a64d-0cdeb9c00b94" />


## Overview

This is a distributed auction system built with Go that demonstrates microservice architecture using RabbitMQ as the communication middleware. The system provides a secure, real-time auction platform where multiple clients can participate in auctions with cryptographic verification of bids.

## Components

### **Auction Microservice**
- Manages auction lifecycle (creation, start, and end)
- Broadcasts auction events to other services
- Handles auction timing and status management

### **Bids Microservice** 
- Validates incoming bids using digital signatures
- Maintains highest bid tracking per auction
- Verifies client authenticity through public key verification
- Publishes valid bids and winner announcements

### **Client Microservice**
- Allows users to participate in auctions
- Generates RSA key pairs for secure bid signing
- Receives real-time notifications about auction updates
- Handles bid submission with cryptographic signatures

### **Notification Microservice**
- Routes notifications to interested clients
- Manages auction event broadcasting
- Handles winner announcements and bid updates

## Encryption & Security

The system implements **RSA 2048-bit asymmetric encryption** for bid security:

- **Key Generation**: Each client generates a unique RSA key pair
- **Digital Signatures**: All bids are signed with the client's private key using SHA-256 hashing
- **Signature Verification**: The bids microservice verifies signatures using clients' public keys
- **Message Integrity**: Prevents bid tampering and ensures authentic participation

### Security Flow:
1. Client generates RSA 2048-bit key pair
2. Public key is shared with the bids microservice
3. Each bid is signed with client's private key
4. Bids microservice verifies signature before processing
5. Only valid, authenticated bids are accepted

## RabbitMQ Middleware

The system uses **RabbitMQ** with multiple exchange patterns:

- **Fanout Exchanges**: For broadcasting auction start events
- **Direct Exchanges**: For targeted bid processing and notifications  
- **Topic Exchanges**: For selective auction notifications to subscribed clients

## Getting Started

### Prerequisites
- Go 1.25.0 or higher
- RabbitMQ server running on localhost:5672

### Running the System

1. **Start RabbitMQ server**
2. **Launch each microservice:**
   ```bash
   # Terminal 1 - Bids Service  
   cd bids_microservice
   go run .

   # Terminal 2 - Notification Service
   cd notification_microservice
   go run .

   # Terminal 3+ - Clients
   cd client_microservice
   go run . <client_id> <client_name>

   # Terminal 4 - Auction Service
   cd auction_microservice
   go run .
   
   ```

### Usage

1. Clients automatically receive notifications when auctions start
2. Place bids using the format: `{bid_amount} {auction_id}`
3. Receive real-time updates on new valid bids and auction winners
4. All communications are secured with RSA digital signatures

---

*Built by Gustavo Adame*
