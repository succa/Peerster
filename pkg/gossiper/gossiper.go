package gossiper

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	"time"

	protobuf "github.com/dedis/protobuf"
	database "github.com/succa/Peerster/pkg/database"
	message "github.com/succa/Peerster/pkg/message"
	peer "github.com/succa/Peerster/pkg/peer"
	utils "github.com/succa/Peerster/pkg/utils"
)

const clientDefaultAddress = "127.0.0.1"
const timeForTheAck = 1
const timeForAntyEntropy = 1
const fileChunk = 1 * (1 << 13) //8Kb
const downloadFolder = "./_Downloads/"

type Gossiper struct {
	address           *net.UDPAddr
	conn              *net.UDPConn
	Name              string
	dbPeers           *database.DatabasePeers
	dbMessages        *database.DatabaseMessages
	dbPrivateMessages *database.DatabasePrivateMessages
	counter           *database.Counter
	dbCh              *database.DatabaseChannels
	routingTable      *database.RoutingTable
	dbFile            *database.FileDatabase
	dbFileCh          *database.RequestFileDatabase
	rtimer            int
	mode              bool
}

// Start the connections to client and peers
func NewGossiper(UIport, address, name string, rtimer int, mode bool) *Gossiper {
	// Peers
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		panic(err)
	}
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		panic(err)
	}

	return &Gossiper{
		address:           udpAddr,
		conn:              udpConn,
		Name:              name,
		dbPeers:           database.NewDatabasePeers(),
		dbMessages:        database.NewDatabaseMessages(),
		dbPrivateMessages: database.NewDatabasePrivateMessages(),
		counter:           database.NewCounter(),
		dbCh:              database.NewDatabaseChannels(),
		routingTable:      database.NewRoutingTable(),
		dbFile:            database.NewFileDatabase(),
		dbFileCh:          database.NewRequestFileDatabase(),
		rtimer:            rtimer,
		mode:              mode,
	}
}

func (g *Gossiper) AddKnownPeers(peers []string) {
	for _, peerString := range peers {
		peer, err := peer.New(peerString)
		if err != nil {
			return
		}
		g.dbPeers.Insert(peer)
	}
}

func (g *Gossiper) Start() {
	// Handle peer connection
	go g.handleConn()

	// Route rumor if not disabled (0 value)
	if g.rtimer != 0 {
		go g.RouteRumor()
	}

	// Anri-entropy -- BLOCKING
	g.antiEntropy()
}

// ****** CLIENT ******
// ********************
func (g *Gossiper) SendClientMessage(msg string) error {
	utils.PrintClientMessage(msg)
	utils.PrintPeers(g.dbPeers.GetKeys())

	if g.mode {
		//Construct the SimpleMessage
		simplemessage := &message.SimpleMessage{
			OriginalName:  g.Name,
			RelayPeerAddr: g.address.String(),
			Contents:      msg,
		}
		packetToSend := &message.GossipPacket{Simple: simplemessage}

		// broadcast
		g.BroadcastMessage(packetToSend, nil)

	} else {
		//Construct the RumorMessage
		rumormessage := &message.RumorMessage{
			Origin: g.Name,
			ID:     g.counter.Next(),
			Text:   msg,
		}
		packetToSend := &message.GossipPacket{Rumor: rumormessage}

		// Save message in the database
		g.dbMessages.InsertMessage(g.Name, rumormessage)

		// Pick a random peer, create a thread to wait for the replay and send the rumor message
		randomPeer, err := g.dbPeers.GetRandom(nil)
		if err != nil {
			return err
		}
		var toAvoid = make(map[string]struct{})
		toAvoid[randomPeer.Address.String()] = struct{}{}

		err = g.rumoring(randomPeer, packetToSend, toAvoid)
		if err != nil {
			return err
		}

	}
	return nil
}

func (g *Gossiper) SendPrivateMessage(msg string, destination string) error {
	//construct the message
	privateMessage := &message.PrivateMessage{
		Origin:      g.Name,
		ID:          0,
		Text:        msg,
		Destination: destination,
		HopLimit:    10,
	}
	packetToSend := &message.GossipPacket{Private: privateMessage}

	// Take the next hop from routing table
	nextHop, ok := g.routingTable.GetRoute(destination)
	if !ok {
		return errors.New("Destination not present in the routing table")
	}
	peer := g.dbPeers.Get(nextHop)
	if peer == nil {
		return errors.New("Destination not present in the routing table")
	}

	// Reduce the hop limit before sending
	packetToSend.Private.HopLimit = packetToSend.Private.HopLimit - 1
	g.SendToPeer(peer, packetToSend)
	utils.PrintSendingPrivateMessage(privateMessage)
	g.dbPrivateMessages.InsertMessage(destination, privateMessage)
	return nil
}

func (g *Gossiper) AddNewPeer(peerAddress string) error {
	peer, err := peer.New(peerAddress)
	if err != nil {
		return err
	}
	g.dbPeers.Insert(peer)
	return nil
}

func (g *Gossiper) GetPeerList() []string {
	return g.dbPeers.GetKeys()
}

func (g *Gossiper) GetNodeList() []string {
	return g.routingTable.GetKeys()
}

func (g *Gossiper) GetMessages() []*message.RumorMessage {
	return g.dbMessages.GetMessages()
}

func (g *Gossiper) GetPrivateMessages(dest string) []*message.PrivateMessage {
	return g.dbPrivateMessages.GetPrivateMessages(dest)
}

func (g *Gossiper) ShareFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	fileStat, _ := file.Stat()
	var fileSize int64 = fileStat.Size()

	totalParts := uint64(math.Ceil(float64(fileSize) / float64(fileChunk)))

	meta := make([][32]byte, totalParts)
	metaByte := make([][]byte, totalParts)
	for i := uint64(0); i < totalParts; i++ {
		partSize := int(math.Min(fileChunk, float64(fileSize-int64(i*fileChunk))))
		partBuffer := make([]byte, partSize)
		file.Read(partBuffer)
		// compute the hash
		chunkHash := sha256.Sum256(partBuffer)
		meta[i] = chunkHash
		metaByte[i] = chunkHash[:]
	}

	metafile := bytes.Join(metaByte, []byte(""))
	metafileHash := sha256.Sum256(metafile)

	//Save the file info
	fileInfo := &database.FileInfo{FileName: filePath, FileSize: fileSize, MetaFile: metafile, MetaHash: metafileHash}
	g.dbFile.InsertMeta(metafileHash, fileInfo)

	//Save the chunk info
	for index, hash := range meta {
		g.dbFile.InsertChunk(hash, &database.ChunkInfo{MetaHash: metafileHash, Index: index})
	}
	fmt.Println("Handled file")
	return nil
}

func (g *Gossiper) RequestFile(dest string, file string, request [32]byte) error {
	//Create _Download folder if it not exists
	if _, err := os.Stat(downloadFolder); os.IsNotExist(err) {
		os.Mkdir(downloadFolder, os.ModePerm)
	}

	// Check if I know the destination and get the next hop
	nextHop, ok := g.routingTable.GetRoute(dest)
	if !ok {
		return errors.New("Destination not present in the routing table")
	}
	peerNextHop := g.dbPeers.Get(nextHop)
	if peerNextHop == nil {
		return errors.New("Destination not present in the routing table")
	}

	//g.SendToPeer(peer, packetToSend)
	//return nil

	// No concurrent request to the same server. Wait until the previous finishes
	for {
		_, err := g.dbFileCh.Get(dest)
		if !err {
			break
		}
		time.Sleep(5 * time.Second)
	}

	// create new channel and save it in the db
	channel := make(chan message.GossipPacket)
	g.dbFileCh.Add(dest, channel)

	// start a thread that handle the file request
	go g.handleFileRequest(peerNextHop, dest, file, request, channel)
	return nil
}

func (g *Gossiper) handleFileRequest(peerNextHop *peer.Peer, dest string, file string, request [32]byte, channel chan message.GossipPacket) {

	defer g.dbFileCh.Delete(dest)

	// send the request in a DataRequest message
	dataRequest := &message.DataRequest{
		Origin:      g.Name,
		Destination: dest,
		HopLimit:    10,
		HashValue:   request[:],
	}
	packetToSend := &message.GossipPacket{DataRequest: dataRequest}

	// The first hash value requested must be the metahash
	// send the packet
	utils.PrintDownloadingMetafile(file, dest)
	// Reduce the hop limit before send
	packetToSend.DataRequest.HopLimit = packetToSend.DataRequest.HopLimit - 1
	g.SendToPeer(peerNextHop, packetToSend)

	var chunkHashes [][]byte
	var ok bool
	tempFileName := downloadFolder + file + "." + dest + ".temp"
	ticker := time.NewTicker(5 * time.Second)
	var flag bool
	for {
		select {
		case replay := <-channel:
			// control the hash received is the same as the one sent
			if !bytes.Equal(request[:], replay.DataReply.HashValue) {
				fmt.Println("hash received is different")
				continue
			}
			// the first replay must contain the metafile, otherwise the request didnt contain the metahash
			data := replay.DataReply.Data
			chunkHashes, ok = checkMetafile(data)
			if !ok {
				fmt.Print("metafile recevied is incorrect")
				continue
			}
			// create a new entry in the fileDb
			fileInfo := &database.FileInfo{
				FileName: tempFileName,
				FileSize: 0,
				MetaFile: data,
				MetaHash: request,
			}
			g.dbFile.InsertMeta(request, fileInfo)
			flag = true
			break
		case <-ticker.C:
			// resend
			utils.PrintDownloadingMetafile(file, dest)
			g.SendToPeer(peerNextHop, packetToSend)
			break
		}
		if flag {
			break
		}
	}

	// request for the chunk hashes
	for index, chunkHash := range chunkHashes {

		// send request
		dataRequest := &message.DataRequest{
			Origin:      g.Name,
			Destination: dest,
			HopLimit:    10,
			HashValue:   chunkHash,
		}
		packetToSend := &message.GossipPacket{DataRequest: dataRequest}

		utils.PrintDownloadingChunk(file, index, dest)
		// Reduce the hop limit before send
		packetToSend.DataRequest.HopLimit = packetToSend.DataRequest.HopLimit - 1
		g.SendToPeer(peerNextHop, packetToSend)

		ticker := time.NewTicker(5 * time.Second)
		var flag bool
		for {
			select {
			case replay := <-channel:
				// control the hash received is the same as the one sent
				if !bytes.Equal(chunkHash, replay.DataReply.HashValue) {
					fmt.Println("hash received is different")
					continue
				}
				// the data field contain the 8k chunk of the file
				dataChunk := replay.DataReply.Data
				// write the data in the temp file
				file, err := os.OpenFile(tempFileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0660)
				if err != nil {
					fmt.Println(err)
					file.Close()
					continue
				}
				n, err := file.Write(dataChunk)
				if err != nil {
					fmt.Println(err)
					file.Close()
					continue
				}
				// Update the FileSize in the metaDb
				g.dbFile.IncreaseFileSize(request, int64(n))

				// Insert hash in the chunkDb
				var chunkHash32 [32]byte
				copy(chunkHash32[:], chunkHash)
				chunkInfo := &database.ChunkInfo{
					MetaHash: request,
					Index:    index,
				}
				g.dbFile.InsertChunk(chunkHash32, chunkInfo)

				file.Close()

				flag = true
				break
			case <-ticker.C:
				// resend
				utils.PrintDownloadingChunk(file, index, dest)
				g.SendToPeer(peerNextHop, packetToSend)
				break
			}
			if flag {
				break
			}
		}
	}

	// After all the chunks are collected and the file is reconstructed, rename it
	er := os.Rename(tempFileName, downloadFolder+file)
	if er != nil {
		fmt.Println(er)
		return
	}
	//Update the file name in the db
	g.dbFile.UpdateFileName(request, downloadFolder+file)
	utils.PrintReconstructed(file)
}

func checkMetafile(data []byte) ([][]byte, bool) {
	// metafile has to be composed by multiple of 32
	if len(data)%32 != 0 {
		return nil, false
	}
	chunks := make([][]byte, len(data)/32)
	for i := 0; i < len(data)/32; i++ {
		chunks[i] = data[i*32 : i*32+32]
	}
	return chunks, true
}

// ****** PEER ******
// ******************
func (g *Gossiper) handleConn() {
	for {
		buf := make([]byte, 1*(1<<15))
		n, addr, err := g.conn.ReadFrom(buf)
		if err != nil {
			fmt.Println(err)
			continue
		}
		go g.servePeer(addr, buf[:n])
	}
}

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
