package gossiper

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	database "github.com/succa/Peerster/pkg/database"
	message "github.com/succa/Peerster/pkg/message"
	peer "github.com/succa/Peerster/pkg/peer"
	utils "github.com/succa/Peerster/pkg/utils"
)

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

func (g *Gossiper) SendOnionPrivateMessage(msg string, destination string) error {
	if !g.tor {
		return errors.New("Onion encription not enabled")
	}

	// Private Message
	privateMessage := &message.PrivateMessage{
		Origin:      g.Name,
		ID:          0,
		Text:        msg,
		Destination: destination,
		HopLimit:    10,
	}
	gossipPacket := &message.GossipPacket{Private: privateMessage}

	// Onion encrypt
	// Ask the miner for 3 nodes from the pk blockchain
	onionNodes, err := g.onionMiner.GetRoute(destination)
	if err != nil {
		return err
	}
	packetToSend, err := g.onionAgent.OnionEncrypt(gossipPacket, onionNodes[0], onionNodes[1], onionNodes[2], onionNodes[3])
	if err != nil {
		return err
	}

	// Take the next hop from routing table
	nextHop, ok := g.routingTable.GetRoute(packetToSend.OnionMessage.Destination)
	if !ok {
		return errors.New("Destination not present in the routing table")
	}
	peer := g.dbPeers.Get(nextHop)
	if peer == nil {
		return errors.New("Destination not present in the routing table")
	}

	// Reduce the hop limit before sending
	packetToSend.OnionMessage.HopLimit = packetToSend.OnionMessage.HopLimit - 1
	err = g.SendToPeer(peer, packetToSend)
	if err != nil {
		fmt.Println("Error in sending")
		return err
	}
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
    ///////////////////////
	// creating tmp dirs //
	///////////////////////

	if _, err := os.Stat(utils.SharedFilesPath); os.IsNotExist(err) {
		os.Mkdir(utils.SharedFilesPath, utils.FileCommonMode)
	}
	if _, err := os.Stat(utils.SharedChunksPath); os.IsNotExist(err) {
		os.Mkdir(utils.SharedChunksPath, utils.FileCommonMode)
	}

	sharedFile := &database.FileInfo{FileName:path.Base(filePath)}

	chunksPath := filepath.Join(utils.SharedChunksPath, sharedFile.FileName)
	if _, err := os.Stat(chunksPath); !os.IsNotExist(err) {
		fmt.Println("such file is already shared, no more")
		return nil // it seems like this sharedFile is already being shared
	}

	os.Mkdir(chunksPath, utils.FileCommonMode)

	//////////////////////////////////
	// building tree level by level //
	//////////////////////////////////

	curLevel := make([]*database.MerkleNode, 0)
	f, err := os.Open(filePath)
	defer f.Close()
	if err != nil {
		fmt.Println(err)
		return nil
	}

	fs, err := f.Stat()
	if err != nil {
		return nil
	}
	sharedFile.FileSize = fs.Size()
	nodeset := make(map[[32]byte]*database.MerkleNode)

	// build 0th level
	offset := int64(0)
	buffer := make([]byte, utils.FileChunkSize)
	for {
		n, err := f.ReadAt(buffer, offset)
		offset += int64(n)
		if n <= 0 && err != nil {
			break // file read succesfully
		}

		curChunk := buffer[0:n]

		chunkHash := sha256.Sum256(curChunk)
		ioutil.WriteFile(filepath.Join(chunksPath, database.GetMerkleChunkFileName(chunkHash)), curChunk, utils.FileCommonMode)
		node := &database.MerkleNode{HashValue: chunkHash, Children: nil, Height: 0}
		curLevel = append(curLevel, node)
		nodeset[chunkHash] = node
	}

	// build upper levels one-by-one
	childrenNumber := utils.FileChunkSize / 32
	for len(curLevel) > 1 || curLevel[0].Height == 0 {
		newLevel := make([]*database.MerkleNode, 0)
		curChildren := make([]*database.MerkleNode, 0, childrenNumber)
		for i := 0; i < len(curLevel); i++ {
			curChildren = append(curChildren, curLevel[i])
			if (i+1)%childrenNumber == 0 || (i+1) == len(curLevel) {
				// children collected for a parent creation
				parentNode := database.ConstructInnerMerkleTree(curChildren, chunksPath)
				newLevel = append(newLevel, parentNode)
				nodeset[parentNode.HashValue] = parentNode

				curChildren = make([]*database.MerkleNode, 0, childrenNumber)
			}
		}

		curLevel = newLevel
	}

	if len(curLevel) == 0 {
		return errors.New("levels built wrongly")
	}

	sharedFile.RootNode = curLevel[0]
	sharedFile.ChunkDb = nodeset

	// Publish a new tx and wait for it to be mined
	g.BroadcastNewTransaction(filepath.Base(filePath), sharedFile.FileSize, sharedFile.RootNode.HashValue[:])

	// Ping the Blockchain to see if the tx has been accepted
	ticker := time.NewTicker(500 * time.Millisecond)
	//after := time.After(20 * time.Second)
	for range ticker.C {
		blockchainMetaHash, ok := g.miner.GetMetaFileFromBlockchain(filepath.Base(filePath))
		if !ok {
			// the tx has not been processed yet
			fmt.Println("Not yet in the blockchain")
			continue
		}
		if blockchainMetaHash != sharedFile.RootNode.HashValue {
			fmt.Println("File name already present in the blockchain")
			return errors.New("File name already present in the blockchain")
		}

		// File accepted by the blockchain
		// Save the file info
		g.dbFile.InsertFileinfo(sharedFile)

		// info about chunks is stored inside fileinfo
		//Save the chunk info
		//for index, hash := range meta {
		//	//g.dbFile.InsertChunk(hash, &ChunkInfo{MetaHash: metafileHash, Index: index})
		//}
		fmt.Println("Handled file " + filepath.Base(filePath) + ", got hash: " + hex.EncodeToString(sharedFile.RootNode.HashValue[:]))
		return nil
	}
	return nil
}

func (g *Gossiper) BroadcastNewTransaction(fileName string, size int64, metafileHash []byte) {
	file := message.File{
		Name:         fileName,
		Size:         size,
		MetafileHash: metafileHash,
	}
	txPublish := &message.TxPublish{
		File:     file,
		HopLimit: 10,
	}
	packetToSend := &message.GossipPacket{TxPublish: txPublish}
	//fmt.Println("Riccardo: broadcasting new transaction")

	g.miner.ChGossiperToMiner <- packetToSend

	//g.BroadcastMessage(packetToSend, nil)

	// Add tx to the pool to be mined
	//g.miner.DbBlockchain.AddTxInPool(txPublish)
}

func (g *Gossiper) RequestFile(dest string, file string, request [32]byte) error {
	//Create _Download folder if it not exists
	if _, err := os.Stat(utils.DownloadsFilesPath); os.IsNotExist(err) {
		os.Mkdir(utils.DownloadsFilesPath, utils.FileCommonMode)
	}

	// start a thread that handle the file request
	go g.handleFileRequest(dest, filepath.Base(file), request, false)
	return nil
}

func (g *Gossiper) RequestFileOnion(dest string, file string, request [32]byte) error {
	//Create _Download folder if it not exists
	if _, err := os.Stat(downloadFolder); os.IsNotExist(err) {
		os.Mkdir(downloadFolder, os.ModePerm)
	}

	// start a thread that handle the file request
	go g.handleFileRequest(dest, filepath.Base(file), request, true)
	return nil
}

func (g *Gossiper) handleFileRequest(dest string, file string, request [32]byte, tor bool) {
	if _, err := os.Stat(utils.DownloadsFilesPath); os.IsNotExist(err) {
		os.Mkdir(utils.DownloadsFilesPath, utils.FileCommonMode)
	}
	if _, err := os.Stat(utils.DownloadsChunksPath); os.IsNotExist(err) {
		os.Mkdir(utils.DownloadsChunksPath, utils.FileCommonMode)
	}

	chunksPath := filepath.Join(utils.DownloadsChunksPath, file)
	if _, err := os.Stat(chunksPath); !os.IsNotExist(err) {
		return // it seems like this file was already downloaded
	}

	os.Mkdir(chunksPath, utils.FileCommonMode)

	//Fist step: download the root hash at dest
	chunkHashes, height, err := g.requestRootHash(dest, file, request, tor)
	if err != nil {
		fmt.Println(err)
		return
	}

	tempFileName := getTemporaryFilename(file)

	rootNode := &database.MerkleNode{Height:height, HashValue:request, Children:nil}

	nodeset := make(map[[32]byte]*database.MerkleNode)
	awaitedChildToParent := make(map[[32]byte]*database.MerkleNode)

	chunksToDownload := chunkHashes // queue for bfs
	nodeset[request] = rootNode
	for _, childHash := range chunkHashes {
		awaitedChildToParent[childHash] = rootNode
	}

	// request for the chunk hashes
	for len(chunksToDownload) != 0 {
		curHash := chunksToDownload[0]

		//time.Sleep(500 * time.Millisecond)
		// Download the Chunk
		mreply, err := g.requestChunk(dest, tempFileName, curHash[:], tor)
		if err != nil {
			// TODO remove the tempFile and return
			fmt.Println(err)
			return
		}
		if len(nodeset) % 1000 == 0 {
			fmt.Println("Downloaded chunk " + strconv.Itoa(len(nodeset)) + ", in queue left: " + strconv.Itoa(len(chunksToDownload)))
		}
		curNode := &database.MerkleNode{Height:mreply.Height, Children:make([]*database.MerkleNode, 0), HashValue:curHash}
		nodeset[curHash] = curNode
		awaitedChildToParent[curHash].Children = append(awaitedChildToParent[curHash].Children, curNode)
		ioutil.WriteFile(filepath.Join(chunksPath, database.GetMerkleChunkFileName(curHash)), mreply.Data, utils.FileCommonMode)

		if mreply.Height != 0 {
			chunkHashes, _ := checkMetafile(mreply.Data)
			for _, childHash := range chunkHashes {
				awaitedChildToParent[childHash] = curNode
				chunksToDownload = append(chunksToDownload, childHash)
			}
		} else {
			file, err := os.OpenFile(downloadFolder+tempFileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0660)
			if err != nil {
				fmt.Println(err)
				file.Close()
				continue
			}
			_, err = file.Write(mreply.Data)
			if err != nil {
				fmt.Println(err)
				file.Close()
				continue
			}
			file.Close()
		}

		chunksToDownload = chunksToDownload[1:]
	}

	// After all the chunks are collected and the file is reconstructed, rename it
	er := os.Rename(downloadFolder+tempFileName, downloadFolder+file)
	if er != nil {
		fmt.Println(er)
		return
	}

	// finally add file to db
	f, err := os.Open(downloadFolder + file)
	defer f.Close()
	if err != nil {
		fmt.Println(err)
		return
	}
	fs, err := f.Stat()
	if err != nil {
		fmt.Println(err)
		return
	}
	fileSize := fs.Size()
	downloadedFile := &database.FileInfo{ChunkDb:nodeset, FileName:file, RootNode:rootNode, FileSize:fileSize}
	g.dbFile.InsertFileinfo(downloadedFile)
	utils.PrintReconstructed(file)
}

func (g *Gossiper) requestRootHash(dest string, file string, request [32]byte, tor bool) ([][32]byte, int, error) {
	//check tor is enabled
	if tor && !g.tor {
		return nil, -1, errors.New("Tor is not enabled")
	}

	// create new channel
	channel := make(chan message.GossipPacket)
	//No concurrent request to the server, wait until no other threads are downloading from it
	for {
		locked := g.dbFileCh.LockDest(dest, channel)
		if locked {
			break
		}
		time.Sleep(1 * time.Second)
	}
	defer g.dbFileCh.Delete(dest)

	// Create a DataRequest
	dataRequest := &message.DataRequest{
		Origin:      g.Name,
		Destination: dest,
		HopLimit:    10,
		HashValue:   request[:],
	}
	packetToSend := &message.GossipPacket{DataRequest: dataRequest}

	var peerNextHop *peer.Peer
	if tor {
		// Onion encrypt
		// Ask the miner for 3 nodes from the pk blockchain
		onionNodes, err := g.onionMiner.GetRoute(dest)
		if err != nil {
			return nil, -1, err
		}
		packetToSend, err = g.onionAgent.OnionEncrypt(packetToSend, onionNodes[0], onionNodes[1], onionNodes[2], onionNodes[3])
		if err != nil {
			return nil, -1, err
		}

		//Check if the destination is reachable
		nextHop, oki := g.routingTable.GetRoute(packetToSend.OnionMessage.Destination)
		if !oki {
			return nil, -1, errors.New("Destination not present in the routing table")
		}
		peerNextHop = g.dbPeers.Get(nextHop)
		if peerNextHop == nil {
			return nil, -1, errors.New("Destination not present in the routing table")
		}

		// Reduce the hop limit before sending
		packetToSend.OnionMessage.HopLimit = packetToSend.OnionMessage.HopLimit - 1
		//fmt.Println("Riccardo before sending onion message")
		g.SendToPeer(peerNextHop, packetToSend)
	} else {
		//Check if the destination is reachable
		nextHop, oki := g.routingTable.GetRoute(dest)
		if !oki {
			//TODO maybe we have to return the error somehow
			return nil, -1, errors.New("Destination not present in the routing table")
		}
		peerNextHop = g.dbPeers.Get(nextHop)
		if peerNextHop == nil {
			return nil, -1, errors.New("Destination not present in the routing table")
		}

		// Reduce the hop limit before sending
		packetToSend.DataRequest.HopLimit = packetToSend.DataRequest.HopLimit - 1
		g.SendToPeer(peerNextHop, packetToSend)
	}

	// Nice print
	utils.PrintDownloadingMetafile(file, dest)

	var chunkHashes [][32]byte
	var ok bool
	var height int
	//tempFileName := getTemporaryFilename(file)
	ticker := time.NewTicker(5 * time.Second)
	var flag bool
	for {
		select {
		case replay := <-channel:
			// control the hash received is the same as the one sent
			if !bytes.Equal(request[:], replay.MDataReply.HashValue) {
				fmt.Println("hash received is different")
				continue
			}
			// the first replay must contain the metafile, otherwise the request didnt contain the metahash
			data := replay.MDataReply.Data
			chunkHashes, ok = checkMetafile(data)
			height = replay.MDataReply.Height
			if !ok {
				fmt.Print("metafile received is incorrect")
				continue
			}

			// entry created only when downloading is completed in order to forbid partial downloading of merkle-shared files
			//fileInfo := &FileInfo{
			//	FileName: tempFileName,
			//	FileSize: 0,
			//	MetaFile: data,
			//	MetaHash: request,
			//}

			ioutil.WriteFile(path.Join(path.Join(utils.DownloadsChunksPath, file), database.GetMerkleChunkFileName(request)), data, utils.FileCommonMode)
			flag = true
			break
		case <-ticker.C:
			// resend
			//TODO maybe put a max number of tentatives
			utils.PrintDownloadingMetafile(file, dest)
			g.SendToPeer(peerNextHop, packetToSend)
			break
		}
		if flag {
			break
		}
	}
	return chunkHashes, height, nil
}

func getTemporaryFilename(filename string) string {
	return "tmp-" + filename
}

func (g *Gossiper) requestChunk(dest string, tempFileName string, chunkHash []byte, tor bool) (*message.MerkleDataReply, error) {
	//check tor is enabled
	if tor && !g.tor {
		return nil, errors.New("Tor is not enabled")
	}

	// create new channel
	channel := make(chan message.GossipPacket)
	//No concurrent request to the server, wait until no other threads are downloading from it
	for {
		locked := g.dbFileCh.LockDest(dest, channel)
		if locked {
			break
		}
		time.Sleep(1 * time.Second)
	}
	defer g.dbFileCh.Delete(dest)

	// send request
	dataRequest := &message.DataRequest{
		Origin:      g.Name,
		Destination: dest,
		HopLimit:    10,
		HashValue:   chunkHash,
	}
	packetToSend := &message.GossipPacket{DataRequest: dataRequest}

	var peerNextHop *peer.Peer
	if tor {
		// Onion encrypt
		// Ask the miner for 3 nodes from the pk blockchain
		onionNodes, err := g.onionMiner.GetRoute(dest)
		if err != nil {
			return nil, err
		}
		packetToSend, err = g.onionAgent.OnionEncrypt(packetToSend, onionNodes[0], onionNodes[1], onionNodes[2], onionNodes[3])
		if err != nil {
			return nil, err
		}

		//Check if the destination is reachable
		nextHop, oki := g.routingTable.GetRoute(packetToSend.OnionMessage.Destination)
		if !oki {
			//TODO maybe we have to return the error somehow
			return nil, errors.New("Destination not present in the routing table")
		}
		peerNextHop = g.dbPeers.Get(nextHop)
		if peerNextHop == nil {
			return nil, errors.New("Destination not present in the routing table")
		}

		// Reduce the hop limit before sending
		packetToSend.OnionMessage.HopLimit = packetToSend.OnionMessage.HopLimit - 1
		g.SendToPeer(peerNextHop, packetToSend)
	} else {
		//Check if the destination is reachable
		nextHop, oki := g.routingTable.GetRoute(dest)
		if !oki {
			//TODO maybe we have to return the error somehow
			return nil, errors.New("Destination not present in the routing table")
		}
		peerNextHop = g.dbPeers.Get(nextHop)
		if peerNextHop == nil {
			return nil, errors.New("Destination not present in the routing table")
		}

		// Reduce the hop limit before sending
		packetToSend.DataRequest.HopLimit = packetToSend.DataRequest.HopLimit - 1
		g.SendToPeer(peerNextHop, packetToSend)
	}

	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case replay := <-channel:
			// control the hash received is the same as the one sent
			if !bytes.Equal(chunkHash, replay.MDataReply.HashValue) {
				fmt.Println("hash received is different")
				continue
			}

			return replay.MDataReply, nil
		case <-ticker.C:
			// resend //TODO max number of tentatives??
			//PrintDownloadingChunk(file, index, dest)
			g.SendToPeer(peerNextHop, packetToSend)
			break
		}
	}
	return nil, nil
}

func checkMetafile(data []byte) ([][32]byte, bool) {
	// metafile has to be composed by multiple of 32
	if len(data)%32 != 0 {
		return nil, false
	}
	chunks := make([][32]byte, len(data)/32)
	for i := 0; i < len(data)/32; i++ {
		copy(chunks[i][:], data[i*32 : i*32+32])
	}
	return chunks, true
}

//func (g *Gossiper) SearchFiles(keywords []string, budget uint64) error {
//	if len(keywords) == 0 {
//		return errors.New("No keywords defined")
//	}
//
//	// Reset the searches that matches the keywords, we will search for them again
//	g.dbFile.ResetRequestDbKeywords(keywords)
//
//	// Start a thread to do the search
//	go g.handleSearchFiles(keywords, budget)
//
//	return nil
//
//}

//func (g *Gossiper) handleSearchFiles(keywords []string, budget uint64) {
//	//go g.downloadMetafileSearched(keywords)
//
//	//this function sends the messages to the peers, and a thread handle the replies
//	//a channel makes the two communicate, so that if the timout accours the thread is killed, or if the
//	//thread reach the threashold communicate it to this. The event is the closing channel
//
//	// Budget specified //TODO but we reach 32 anyway if it is lower????
//	if budget != 0 {
//
//		//communication := make(chan bool)
//		//go g.listenForSearchReplies(2, communication)
//
//		peersAndBudget, err := g.dbPeers.GetRandomWithBudget(budget, nil)
//		if err != nil {
//			//TODO decide what to do
//			//close(communication)
//			fmt.Println("No Peers")
//			return
//		}
//
//		for _, p := range peersAndBudget {
//			searchRequest := &message.SearchRequest{
//				Origin:   g.Name,
//				Budget:   p.Budget,
//				Keywords: keywords,
//			}
//			packetToSend := &message.GossipPacket{SearchRequest: searchRequest}
//
//			fmt.Println("SENDING SEARCH REQUEST to " + p.Peer.Address.String())
//			g.SendToPeer(p.Peer, packetToSend)
//
//		}
//	} else {
//
//		budget = 1
//		threshold := 2
//		// wait for the replay //TODO timeout? resend??
//		//TODO cancellarli dal db alla fine?
//		for {
//			select {
//			//case _, ok := <-communication:
//			//	if !ok {
//			//		return
//			//	}
//			case <-time.Tick(1 * time.Second):
//				// Look at the file Database if there are 2 completed matches
//				if g.dbFile.CheckCompletedSearch(keywords, threshold) {
//					fmt.Println("SEARCH FINISHED")
//					return
//				}
//
//				budget = budget * 2
//				if budget > 32 {
//					//close(communication)
//					return
//				}
//				fmt.Printf("Budget %d\n", int(budget))
//
//				peersAndBudget, err := g.dbPeers.GetRandomWithBudget(budget, nil)
//				if err != nil {
//					//TODO decide what to do
//					fmt.Println("No Peers")
//					return
//				}
//				for _, p := range peersAndBudget {
//					searchRequest := &message.SearchRequest{
//						Origin:   g.Name,
//						Budget:   p.Budget,
//						Keywords: keywords,
//					}
//					packetToSend := &message.GossipPacket{SearchRequest: searchRequest}
//
//					//fmt.Println("SENDING SEARCH REQUEST to " + p.Peer.Address.String())
//					g.SendToPeer(p.Peer, packetToSend)
//				}
//			}
//		}
//	}
//
//}

//func (g *Gossiper) downloadMetafileSearched(keywords []string) {
//	ticker := time.NewTicker(500 * time.Millisecond)
//	after := time.After(20 * time.Second)
//	for {
//		select {
//		case <-after:
//			return
//		case <-ticker.C:
//			requestInfos := g.dbFile.GetCompletedSearches(keywords)
//			for _, requestInfo := range requestInfos {
//				g.requestMetafile(
//					requestInfo.ChunkMap[0],
//					requestInfo.FileName,
//					requestInfo.MetaHash,
//				)
//			}
//		}
//	}
//}

//func (g *Gossiper) GetCompletedSearches(keywords []string) (ret []string) {
//	requestInfos := g.dbFile.GetCompletedSearchesClient(keywords)
//	for _, requestInfo := range requestInfos {
//		ret = append(ret, requestInfo.FileName)
//	}
//	return ret
//}
//
//func (g *Gossiper) DownloadSearchedFile(destFileName string, metaHash [32]byte) error {
//	//Create _Download folder if it not exists
//	if _, err := os.Stat(downloadFolder); os.IsNotExist(err) {
//		os.Mkdir(downloadFolder, os.ModePerm)
//	}
//
//	// get the requestInfo
//	requestInfo, ok := g.dbFile.GetRequestInfo(metaHash)
//	if !ok {
//		return errors.New("Failed to retrieve request info")
//	}
//
//	g.handleFileRequest(requestInfo.ChunkMap[1], destFileName, metaHash)
//	return nil
//
//	fileInfo, ok := g.dbFile.GetMeta(requestInfo.MetaHash)
//	if !ok {
//		return errors.New("Failed to retrieve file info")
//	}
//
//	chunkHashes, ok := checkMetafile(fileInfo.MetaFile)
//	if !ok {
//		return errors.New("Failed to parse chunk hashes")
//	}
//
//	tempFileName := fileInfo.FileName
//
//	for i := 0; i < requestInfo.ChunkNum; i++ {
//		dest := requestInfo.ChunkMap[i]
//		request := requestInfo.MetaHash
//		// Download the Chunk
//		err := g.requestChunk(dest, destFileName, tempFileName, request, chunkHashes[i], i)
//		if err != nil {
//			// TODO remove the tempFile and return
//			fmt.Println(err)
//			return err
//		}
//	}
//
//	// After all the chunks are collected and the file is reconstructed, rename it
//	er := os.Rename(downloadFolder+tempFileName, downloadFolder+destFileName)
//	if er != nil {
//		fmt.Println(er)
//		return er
//	}
//	//Update the file name in the db
//	g.dbFile.UpdateFileName(fileInfo.MetaHash, downloadFolder+destFileName)
//	PrintReconstructed(destFileName)
//
//	return nil
//}

//func (g *Gossiper) DownloadSearchedFileFromName(destFileName string) error {
//	//Create _Download folder if it not exists
//	if _, err := os.Stat(downloadFolder); os.IsNotExist(err) {
//		os.Mkdir(downloadFolder, os.ModePerm)
//	}
//
//	// get the requestInfo
//	metaHash, ok := g.dbFile.GetMetaFromRequestName(destFileName)
//	if !ok {
//		return errors.New("Failed to retrieve meta hash")
//	}
//	requestInfo, ok := g.dbFile.GetRequestInfo(metaHash)
//	if !ok {
//		return errors.New("Failed to retrieve request info")
//	}
//
//	g.handleFileRequest(requestInfo.ChunkMap[1], destFileName, metaHash)
//	return nil
//
//	fileInfo, ok := g.dbFile.GetMeta(requestInfo.MetaHash)
//	if !ok {
//		return errors.New("Failed to retrieve file info")
//	}
//
//	chunkHashes, ok := checkMetafile(fileInfo.MetaFile)
//	if !ok {
//		return errors.New("Failed to parse chunk hashes")
//	}
//
//	tempFileName := fileInfo.FileName
//	//fmt.Println("DestFileName " + destFileName)
//	//fmt.Println("tempFileName " + tempFileName)
//	// if the file was already downloaded, the tempfilename is equal to the file name (it was aready retitled)
//	if destFileName == filepath.Base(tempFileName) {
//		fmt.Println("Already downloaded")
//		return nil
//	}
//
//	for i := 0; i < requestInfo.ChunkNum; i++ {
//		dest := requestInfo.ChunkMap[i]
//		request := requestInfo.MetaHash
//		// Download the Chunk
//		err := g.requestChunk(dest, destFileName, tempFileName, request, chunkHashes[i], i)
//		if err != nil {
//			// TODO remove the tempFile and return
//			fmt.Println(err)
//			return err
//		}
//	}
//
//	// After all the chunks are collected and the file is reconstructed, rename it
//	er := os.Rename(downloadFolder+tempFileName, downloadFolder+destFileName)
//	if er != nil {
//		fmt.Println(er)
//		return er
//	}
//	//Update the file name in the db
//	g.dbFile.UpdateFileName(fileInfo.MetaHash, downloadFolder+destFileName)
//	PrintReconstructed(destFileName)
//
//	return nil
//}
