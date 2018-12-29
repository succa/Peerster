package gossiper

import (
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	protobuf "github.com/dedis/protobuf"
	message "github.com/succa/Peerster/pkg/message"
	peer "github.com/succa/Peerster/pkg/peer"
	utils "github.com/succa/Peerster/pkg/utils"
)

// ****** PEER ******
// ******************
func (g *Gossiper) servePeer(addr net.Addr, buf []byte) {
	//Decode the message received
	var packetReceived message.GossipPacket
	protobuf.Decode(buf, &packetReceived)

	if packetReceived.Simple != nil {
		//Add the peer if necessary
		peer, err := peer.New(packetReceived.Simple.RelayPeerAddr)
		if err != nil {
			return
		}
		g.dbPeers.Insert(peer)

		utils.PrintSimpleMessage(packetReceived.Simple)
		utils.PrintPeers(g.dbPeers.GetKeys())

		var toAvoid = make(map[string]struct{})
		toAvoid[packetReceived.Simple.RelayPeerAddr] = struct{}{}

		// change the RelayPeerAddr field
		packetReceived.Simple.RelayPeerAddr = g.address.String()

		// broadcast
		g.BroadcastMessage(&packetReceived, toAvoid)

	} else if packetReceived.Rumor != nil {
		//Add the peer if necessary
		peer, err := peer.New(addr.String())
		if err != nil {
			return
		}
		g.dbPeers.Insert(peer)

		peer = g.dbPeers.Get(addr.String())

		// if the message is empty, this is a route rumor
		if packetReceived.Rumor.Text != "" {
			utils.PrintRumorMessage(packetReceived.Rumor, addr.String())
			utils.PrintPeers(g.dbPeers.GetKeys())
		}

		// Save the message in the database
		err = g.dbMessages.InsertMessage(packetReceived.Rumor.Origin, packetReceived.Rumor)

		// Send back status packet
		statusPacket := &message.StatusPacket{Want: g.dbMessages.GetVectorClock()}
		packetToSend := &message.GossipPacket{Status: statusPacket}

		//Test
		//time.Sleep(3 * time.Second)
		//Send back status packet
		g.SendToPeer(peer, packetToSend)

		//if InsertMessage returned an error, the message was already received or out of order. No need to send to another peer
		if err != nil {
			return
		}

		//if InsertMessage was ok, a new message is inserted in the database. Update the routing table
		ok := g.routingTable.UpdateRoute(packetReceived.Rumor.Origin, peer.Address.String())
		if ok {
			utils.PrintDSDV(packetReceived.Rumor.Origin, peer.Address.String())
		}

		// Pick a random peer and rumoring
		var toAvoid = make(map[string]struct{})
		toAvoid[peer.Address.String()] = struct{}{}
		randomPeer, err := g.dbPeers.GetRandom(toAvoid)
		if err != nil {
			return
		}

		g.rumoring(randomPeer, &packetReceived, toAvoid)

	} else if packetReceived.Status != nil {
		//Add the peer if necessary
		peer, err := peer.New(addr.String())
		if err != nil {
			return
		}
		g.dbPeers.Insert(peer)

		utils.PrintStatusMessage(packetReceived.Status, addr.String())
		utils.PrintPeers(g.dbPeers.GetKeys())

		ch, ok := g.dbCh.Get(addr.String())

		if !ok {
			//status message from a new peer
			rumorToSend, vectorClock := g.dbMessages.CompareVectors(packetReceived.Status.Want)
			if rumorToSend != nil {
				packetToSend := &message.GossipPacket{Rumor: rumorToSend}
				g.rumoring(peer, packetToSend, nil)
				return
			} else if vectorClock != nil {
				statusPacket := &message.StatusPacket{Want: vectorClock}
				packetToSend := &message.GossipPacket{Status: statusPacket}
				g.SendToPeer(peer, packetToSend)
				return
			} else {
				utils.PrintInSyncWith(addr.String())
			}
		} else {
			//status message to forward to the thread
			ch <- packetReceived
			g.dbCh.Delete(addr.String())
		}
	} else if packetReceived.Private != nil {
		//Add the peer if necessary
		peer, err := peer.New(addr.String())
		if err != nil {
			return
		}
		g.dbPeers.Insert(peer)

		// If i'm the destination, I print the message and return
		if packetReceived.Private.Destination == g.Name {
			utils.PrintPrivateMessage(packetReceived.Private)
			g.dbPrivateMessages.InsertMessage(packetReceived.Private.Origin, packetReceived.Private)
			return
		}

		// Forward the message to next hop
		nextHop, ok := g.routingTable.GetRoute(packetReceived.Private.Destination)
		if !ok {
			fmt.Println("Destination not present in the routing table")
			return
		}
		destinationPeer := g.dbPeers.Get(nextHop)
		if peer == nil {
			fmt.Println("Destination not present in the routing table")
			return
		}

		//reduce the hop limit
		packetReceived.Private.HopLimit = packetReceived.Private.HopLimit - 1
		// discard if reched limit
		if packetReceived.Private.HopLimit == 0 {
			fmt.Println("Reached 0")
			return
		}

		g.SendToPeer(destinationPeer, &packetReceived)
	} else if packetReceived.DataRequest != nil {
		//fmt.Println("RECEIVED DATA REQUEST")
		//Add the peer if necessary
		peer, err := peer.New(addr.String())
		if err != nil {
			return
		}
		g.dbPeers.Insert(peer)

		// If i'm the destination, prepare the DataReplay message and send it back
		if packetReceived.DataRequest.Destination == g.Name {
			var HashValue32 [32]byte
			copy(HashValue32[:], packetReceived.DataRequest.HashValue)
			data, ok := g.dbFile.GetHashValue(HashValue32)
			if !ok {
				return
			}
			dataReply := &message.DataReply{
				Origin:      g.Name,
				Destination: packetReceived.DataRequest.Origin,
				HopLimit:    10,
				HashValue:   packetReceived.DataRequest.HashValue,
				Data:        data,
			}
			packetToSend := &message.GossipPacket{DataReply: dataReply}
			//time.Sleep(500 * time.Millisecond)
			// Reduce the hop limit before send
			packetToSend.DataReply.HopLimit = packetToSend.DataReply.HopLimit - 1
			g.SendToPeer(peer, packetToSend)
			return
		}

		// Otherwise we have to forward the packet
		nextHop, ok := g.routingTable.GetRoute(packetReceived.DataRequest.Destination)
		if !ok {
			fmt.Println("Destination not present in the routing table")
			return
		}
		destinationPeer := g.dbPeers.Get(nextHop)
		if destinationPeer == nil {
			fmt.Println("Destination not present in the routing table")
			return
		}

		//reduce the hop limit
		packetReceived.DataRequest.HopLimit = packetReceived.DataRequest.HopLimit - 1
		// discard if reached limit
		if packetReceived.DataRequest.HopLimit == 0 {
			fmt.Println("Reached 0")
			return
		}

		g.SendToPeer(destinationPeer, &packetReceived)

	} else if packetReceived.DataReply != nil {
		//Add the peer if necessary
		peer, err := peer.New(addr.String())
		if err != nil {
			return
		}
		g.dbPeers.Insert(peer)

		// If I am the destination, forward the message to the thread is handling it
		if packetReceived.DataReply.Destination == g.Name {
			ch, ok := g.dbFileCh.Get(packetReceived.DataReply.Origin)
			if !ok {
				fmt.Println("Received a DataReply message, but no thread to handle it")
				return
			}
			ch <- packetReceived
			return
		}

		// Otherwise we have to forward the packet
		nextHop, ok := g.routingTable.GetRoute(packetReceived.DataReply.Destination)
		if !ok {
			fmt.Println("Destination not present in the routing table")
			return
		}
		destinationPeer := g.dbPeers.Get(nextHop)
		if destinationPeer == nil {
			fmt.Println("Destination not present in the routing table")
			return
		}

		//reduce the hop limit
		packetReceived.DataReply.HopLimit = packetReceived.DataReply.HopLimit - 1
		// discard if reached limit
		if packetReceived.DataReply.HopLimit == 0 {
			fmt.Println("Reached 0")
			return
		}

		g.SendToPeer(destinationPeer, &packetReceived)
	} else if packetReceived.SearchRequest != nil {

		//Add the peer if necessary
		peer, err := peer.New(addr.String())
		if err != nil {
			return
		}
		g.dbPeers.Insert(peer)
		fmt.Println("RECEIVED SEARCH REQUEST from " + peer.Address.String())

		//check if already received
		if g.searchDuplicates.IsDuplicate(packetReceived.SearchRequest.Origin, strings.Join(packetReceived.SearchRequest.Keywords, ",")) {
			fmt.Println("Search request is duplicate")
			return
		}

		// Insert search request to the db
		g.searchDuplicates.Insert(packetReceived.SearchRequest.Origin, strings.Join(packetReceived.SearchRequest.Keywords, ","))

		for _, file := range packetReceived.SearchRequest.Keywords {
			filesFound := g.dbFile.GetFileRegex(file)
			for _, fileFound := range filesFound {
				fmt.Print("File Found ")
				fmt.Println(fileFound.FileName)
			}
			if len(filesFound) != 0 {
				// Send back the result
				searchReply := &message.SearchReply{
					Origin:      g.Name,
					Destination: packetReceived.SearchRequest.Origin,
					HopLimit:    10, //TODO ask
					Results:     filesFound,
				}
				packetToSend := &message.GossipPacket{
					SearchReply: searchReply,
				}
				// Reduce the hop limit before send
				packetToSend.SearchReply.HopLimit = packetToSend.SearchReply.HopLimit - 1
				g.SendToPeer(peer, packetToSend)
				return
			}
		}

		// Reduce the budget and forward the packet to peers if needed
		budget := packetReceived.SearchRequest.Budget - 1
		var toAvoid = make(map[string]struct{})
		fmt.Println(peer.Address.String())
		toAvoid[peer.Address.String()] = struct{}{}
		peersAndBudget, err := g.dbPeers.GetRandomWithBudget(budget, toAvoid)
		if err != nil {
			//TODO decide what to do
			fmt.Println("No Peers")
			return
		}
		for _, p := range peersAndBudget {
			fmt.Print("Sending to ")
			fmt.Println(p.Peer.Address.String())
			searchRequest := &message.SearchRequest{
				Origin:   packetReceived.SearchRequest.Origin,
				Budget:   p.Budget,
				Keywords: packetReceived.SearchRequest.Keywords,
			}
			packetToSend := &message.GossipPacket{SearchRequest: searchRequest}

			g.SendToPeer(p.Peer, packetToSend)

		}
	} else if packetReceived.SearchReply != nil {
		fmt.Println("RECEIVED SEARCH REPLY origin " + packetReceived.SearchReply.Origin + " from " + addr.String())
		//Add the peer if necessary
		peer, err := peer.New(addr.String())
		if err != nil {
			return
		}
		g.dbPeers.Insert(peer)

		// If I am the destination, insert the results in the db if necessary
		if packetReceived.SearchReply.Destination == g.Name {
			g.dbFile.HandleSearchReply(packetReceived.SearchReply)
			return
		}

		// Otherwise we have to forward the packet
		// TODO TODO TODO send directly to the Destination or to the hop?????
		nextHop, ok := g.routingTable.GetRoute(packetReceived.SearchReply.Destination)
		if !ok {
			fmt.Println("Destination not present in the routing table")
			return
		}
		destinationPeer := g.dbPeers.Get(nextHop)
		if destinationPeer == nil {
			fmt.Println("Destination not present in the routing table")
			return
		}

		//reduce the hop limit
		packetReceived.SearchReply.HopLimit = packetReceived.SearchReply.HopLimit - 1
		// discard if reached limit
		if packetReceived.SearchReply.HopLimit == 0 {
			fmt.Println("Reached 0")
			return
		}
		g.SendToPeer(destinationPeer, &packetReceived)
	} else if packetReceived.TxPublish != nil {
		g.fileMiner.ChGossiperToMiner <- &packetReceived
	} else if packetReceived.BlockPublish != nil {
		g.fileMiner.ChGossiperToMiner <- &packetReceived
	}
}

func (g *Gossiper) rumoring(peer *peer.Peer, packetToSend *message.GossipPacket, toAvoid map[string]struct{}) error {
	channel := make(chan message.GossipPacket)
	g.dbCh.Add(peer.Address.String(), channel)
	if toAvoid == nil {
		toAvoid = make(map[string]struct{})
	}
	toAvoid[peer.Address.String()] = struct{}{}
	go g.waitForRumorAck(channel, peer.Address, packetToSend, toAvoid)

	utils.PrintMongering(peer.Address.String())
	err := g.SendToPeer(peer, packetToSend)
	return err
}

func (g *Gossiper) waitForRumorAck(channel chan message.GossipPacket, relayAddr *net.UDPAddr, packetToSend *message.GossipPacket, peerToAvoid map[string]struct{}) {
	ticker := time.NewTicker(timeForTheAck * time.Second)
	defer g.dbCh.Delete(relayAddr.String())
	for {
		select {
		case msg := <-channel:
			rumorToSend, vectorClock := g.dbMessages.CompareVectors(msg.Status.Want)
			if rumorToSend != nil {
				packetToSend := &message.GossipPacket{Rumor: rumorToSend}
				peer := g.dbPeers.Db[relayAddr.String()]
				g.rumoring(peer, packetToSend, nil)
				return
			} else if vectorClock != nil {
				statusPacket := &message.StatusPacket{Want: vectorClock}
				packetToSend := &message.GossipPacket{Status: statusPacket}
				peer := g.dbPeers.Db[relayAddr.String()]
				g.SendToPeer(peer, packetToSend)
				return
			} else {
				utils.PrintInSyncWith(relayAddr.String())
				//Flip the coin
				switch choose := rand.Int() % 2; choose {
				case 0:
					//Ceases the rumor
					return
				case 1:
					//Start new rumormongering process
					randomPeer, err := g.dbPeers.GetRandom(peerToAvoid)
					if err != nil {
						return
					}
					channel := make(chan message.GossipPacket)
					g.dbCh.Add(randomPeer.Address.String(), channel)
					peerToAvoid[randomPeer.Address.String()] = struct{}{}
					//delete(peerToAvoid, relayAddr.String())
					go g.waitForRumorAck(channel, randomPeer.Address, packetToSend, peerToAvoid)

					utils.PrintFlippedCoin(randomPeer.Address.String())
					g.SendToPeer(randomPeer, packetToSend)
				}
				return
			}
		case <-ticker.C:
			ticker.Stop()
			// flip a coin, A) choose a new peer and start rumoring B) stop
			switch choose := rand.Int() % 2; choose {
			case 0:
				//Ceases the rumor
				return
			case 1:
				//Start new rumormongering process
				randomPeer, err := g.dbPeers.GetRandom(peerToAvoid)
				if err != nil {
					return
				}
				channel := make(chan message.GossipPacket)
				g.dbCh.Add(randomPeer.Address.String(), channel)
				peerToAvoid[randomPeer.Address.String()] = struct{}{}
				go g.waitForRumorAck(channel, randomPeer.Address, packetToSend, peerToAvoid)

				utils.PrintFlippedCoin(randomPeer.Address.String())
				g.SendToPeer(randomPeer, packetToSend)
			}
			return
		}
	}
}

// ***** ANTI ENTROPY *****
// ************************
func (g *Gossiper) antiEntropy() {
	// Send a status message to a random peer every x seconds
	ticker := time.NewTicker(time.Duration(timeForAntyEntropy) * time.Second)
	for {
		<-ticker.C
		randomPeer, err := g.dbPeers.GetRandom(nil)
		if err != nil {
			continue
		}
		statusPacket := &message.StatusPacket{Want: g.dbMessages.GetVectorClock()}
		packetToSend := &message.GossipPacket{Status: statusPacket}

		g.SendToPeer(randomPeer, packetToSend)
	}
}

// ***** ROUTE RUMOR *****
// ***********************
func (g *Gossiper) RouteRumor() {
	//Fist route rumor to send immediately
	randomPeer, err := g.dbPeers.GetRandom(nil)
	if err == nil {
		//Construct the RumorMessage
		rumormessage := &message.RumorMessage{
			Origin: g.Name,
			ID:     g.counter.Next(),
			Text:   "",
		}
		packetToSend := &message.GossipPacket{Rumor: rumormessage}

		// Save message in the database
		g.dbMessages.InsertMessage(g.Name, rumormessage)

		g.SendToPeer(randomPeer, packetToSend)
	}

	// Periodic
	ticker := time.NewTicker(time.Duration(g.rtimer) * time.Second)
	for {
		<-ticker.C
		randomPeer, err := g.dbPeers.GetRandom(nil)
		if err != nil {
			continue
		}
		//Construct the RumorMessage
		rumormessage := &message.RumorMessage{
			Origin: g.Name,
			ID:     g.counter.Next(),
			Text:   "",
		}
		packetToSend := &message.GossipPacket{Rumor: rumormessage}
		g.dbMessages.InsertMessage(g.Name, rumormessage)

		g.SendToPeer(randomPeer, packetToSend)
	}
}

// ****** MINER ******
// *******************
func (g *Gossiper) FileMining() {
	// Start the Miner (file Miner and pk miner)
	go g.fileMiner.StartMining()
	//go g.pkMiner.StartMining()

	// Handle messages from the file miner
	go func() {
		for {
			packetToSend := <-g.fileMiner.ChMinerToGossiper
			// Broadcast the message
			g.BroadcastMessage(packetToSend, nil)
		}
	}()

	// Handle messages from the pk miner
	/*
		go func() {
			for {
				packetToSend := <-g.pkMiner.ChMinerToGossiper
				// Broadcast the message
				g.BroadcastMessage(packetToSend, nil)
			}
		}()
	*/
}

func (g *Gossiper) PkMining() {
	fmt.Println("Starting the pk miner")
	// Start the Miner (file Miner and pk miner)
	//go g.pkMiner.StartMining()

	// Handle messages from the pk miner
	/*
		go func() {
			for {
				packetToSend := <-g.pkMiner.ChMinerToGossiper
				// Broadcast the message
				g.BroadcastMessage(packetToSend, nil)
			}
		}()
	*/
}

// ****** Functions to send ******
func (g *Gossiper) BroadcastMessage(packetToSend *message.GossipPacket, peerToAvoid map[string]struct{}) {

	for address, peer := range g.dbPeers.Db {
		if _, ok := peerToAvoid[address]; !ok {
			g.SendToPeer(peer, packetToSend)
		}
	}
}

func (g *Gossiper) SendToPeer(peer *peer.Peer, packet *message.GossipPacket) error {
	packetByte, err := protobuf.Encode(packet)
	if err != nil {
		return err
	}
	_, err = g.conn.WriteToUDP(packetByte, peer.Address)
	if err != nil {
		return err
	}
	return nil
}
