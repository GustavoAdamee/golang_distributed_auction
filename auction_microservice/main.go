package main

import (
	"fmt"
	"time"
)

func main() {
	auction_1 := Auction{
		id:          "123",
		description: "auction 1",
		Start_time:  time.Now().Add(5 * time.Second),
		End_time:    time.Now().Add(60 * time.Second),
		Status:      StatusIdle,
	}

	auction_2 := Auction{
		id:          "321",
		description: "auction 2",
		Start_time:  time.Now().Add(6 * time.Second),
		End_time:    time.Now().Add(25 * time.Second),
		Status:      StatusIdle,
	}

	var auctionPool []Auction

	// Initialize auctions before adding to pool
	auction_1.Init()
	auction_2.Init()

	auctionPool = append(auctionPool, auction_1)
	auctionPool = append(auctionPool, auction_2)

	// Ensure cleanup when program exits
	defer auction_1.Close()
	defer auction_2.Close()

	auctionPooling(auctionPool)
}

func auctionPooling(pool []Auction) {
	var is_all_auctions_finished bool = false
	for !is_all_auctions_finished {
		for i := 0; i < len(pool); i++ {
			if pool[i].Status == StatusIdle || pool[i].Status == StatusActive {
				checkAuctionStarted(&pool[i])
				checkAuctionEnded(&pool[i])
			} else {
				pool = append(pool[:i], pool[i+1:]...)
			}
		}
		if len(pool) == 0 {
			is_all_auctions_finished = true
		}
	}
	fmt.Printf("All auctions finished!\n")
}

func checkAuctionStarted(auction *Auction) {
	if (time.Now().After(auction.Start_time) || time.Now().Equal(auction.Start_time)) && auction.Status == StatusIdle {
		auction.Status = StatusActive
		auction.StartAuction()
		fmt.Printf("Auction %v started --- Auction status set to: %v\n", auction.id, auction.Status)
	}
}

func checkAuctionEnded(auction *Auction) {
	if time.Now().After(auction.End_time) && auction.Status == StatusActive {
		auction.Status = StatusFinished
		auction.FinishAuction()
		fmt.Printf("Auction %v ended --- Auction status set to: %v\n", auction.id, auction.Status)
		auction.Close()
	}
}
