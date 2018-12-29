package gossiper

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	var fileSize = fileStat.Size()

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
	fmt.Printf("META file %s: %x\n", filepath.Base(filePath), metafileHash)

	// Publish a new tx and wait for it to be mined
	g.BroadcastNewTransaction(filepath.Base(filePath), fileSize, metafileHash[:])

	// Ping the Blockchain to see if the tx has been accepted
	ticker := time.NewTicker(500 * time.Millisecond)
	//after := time.After(20 * time.Second)
	for range ticker.C {
		blockchainMetaHash, ok := g.fileMiner.GetMetaFileFromBlockchain(filepath.Base(filePath))
		if !ok {
			// the tx has not been processed yet
			fmt.Println("Not yet in the blockchain")
			continue
		}
		if blockchainMetaHash != metafileHash {
			fmt.Println("File name already present in the blockchain")
			return errors.New("File name already present in the blockchain")
		}

		// File accepted by the blockchain
		//Save the file info
		fileInfo := &database.FileInfo{FileName: filepath.Base(filePath), FileSize: fileSize, MetaFile: metafile, MetaHash: metafileHash}
		g.dbFile.InsertMeta(metafileHash, fileInfo)

		//Save the chunk info
		for index, hash := range meta {
			g.dbFile.InsertChunk(hash, &database.ChunkInfo{MetaHash: metafileHash, Index: index})
		}
		fmt.Println("Handled file")
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

	g.fileMiner.ChGossiperToMiner <- packetToSend

	//g.BroadcastMessage(packetToSend, nil)

	// Add tx to the pool to be mined
	//g.miner.DbBlockchain.AddTxInPool(txPublish)
}

func (g *Gossiper) RequestFile(dest string, file string, request [32]byte) error {
	//Create _Download folder if it not exists
	if _, err := os.Stat(downloadFolder); os.IsNotExist(err) {
		os.Mkdir(downloadFolder, os.ModePerm)
	}

	// start a thread that handle the file request
	go g.handleFileRequest(dest, filepath.Base(file), request)
	//TODO return error from thread somehow
	return nil
}

func (g *Gossiper) handleFileRequest(dest string, file string, request [32]byte) {

	//Fist step: download the metafile at dest
	tempFileName, chunkHashes, err := g.requestMetafile(dest, file, request)

	if err != nil {
		fmt.Println(err)
		return
	}

	// request for the chunk hashes
	for index, chunkHash := range chunkHashes {
		//time.Sleep(500 * time.Millisecond)
		// Download the Chunk
		err := g.requestChunk(dest, file, tempFileName, request, chunkHash, index)
		if err != nil {
			// TODO remove the tempFile and return
			fmt.Println(err)
			return
		}
	}

	// After all the chunks are collected and the file is reconstructed, rename it
	er := os.Rename(downloadFolder+tempFileName, downloadFolder+file)
	if er != nil {
		fmt.Println(er)
		return
	}
	//Update the file name in the db
	g.dbFile.UpdateFileName(request, file)
	utils.PrintReconstructed(file)

}

func (g *Gossiper) requestMetafile(dest string, file string, request [32]byte) (string, [][]byte, error) {
	//Check if the destination is reachable
	nextHop, oki := g.routingTable.GetRoute(dest)
	if !oki {
		//TODO maybe we have to return the error somehow
		return "", nil, errors.New("Destination not present in the routing table")
	}
	peerNextHop := g.dbPeers.Get(nextHop)
	if peerNextHop == nil {
		return "", nil, errors.New("Destination not present in the routing table")
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

	// Nice print
	utils.PrintDownloadingMetafile(file, dest)

	// Create a DataRequest
	dataRequest := &message.DataRequest{
		Origin:      g.Name,
		Destination: dest,
		HopLimit:    10,
		HashValue:   request[:],
	}
	packetToSend := &message.GossipPacket{DataRequest: dataRequest}

	// Reduce the hop limit before sending
	packetToSend.DataRequest.HopLimit = packetToSend.DataRequest.HopLimit - 1
	g.SendToPeer(peerNextHop, packetToSend)

	var chunkHashes [][]byte
	var ok bool
	tempFileName := file + "." + hex.EncodeToString(request[:]) + ".temp"
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
				fmt.Print("metafile received is incorrect")
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
			//TODO maybe put a max number of tentatives
			utils.PrintDownloadingMetafile(file, dest)
			g.SendToPeer(peerNextHop, packetToSend)
			break
		}
		if flag {
			break
		}
	}
	return tempFileName, chunkHashes, nil
}

func (g *Gossiper) requestChunk(dest string, file string, tempFileName string, request [32]byte, chunkHash []byte, index int) error {
	//Check if the destination is reachable
	nextHop, oki := g.routingTable.GetRoute(dest)
	if !oki {
		//TODO maybe we have to return the error somehow
		return errors.New("Destination not present in the routing table")
	}
	peerNextHop := g.dbPeers.Get(nextHop)
	if peerNextHop == nil {
		return errors.New("Destination not present in the routing table")
	}

	// Nice print
	utils.PrintDownloadingChunk(file, index, dest)

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
			file, err := os.OpenFile(downloadFolder+tempFileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0660)
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
			// resend //TODO max number of tentatives??
			utils.PrintDownloadingChunk(file, index, dest)
			g.SendToPeer(peerNextHop, packetToSend)
			break
		}
		if flag {
			break
		}
	}
	return nil
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

func (g *Gossiper) SearchFiles(keywords []string, budget uint64) error {
	if len(keywords) == 0 {
		return errors.New("No keywords defined")
	}

	// Reset the searches that matches the keywords, we will search for them again
	g.dbFile.ResetRequestDbKeywords(keywords)

	// Start a thread to do the search
	go g.handleSearchFiles(keywords, budget)

	return nil

}

func (g *Gossiper) handleSearchFiles(keywords []string, budget uint64) {
	go g.downloadMetafileSearched(keywords)

	//this function sends the messages to the peers, and a thread handle the replies
	//a channel makes the two communicate, so that if the timout accours the thread is killed, or if the
	//thread reach the threashold communicate it to this. The event is the closing channel

	// Budget specified //TODO but we reach 32 anyway if it is lower????
	if budget != 0 {

		//communication := make(chan bool)
		//go g.listenForSearchReplies(2, communication)

		peersAndBudget, err := g.dbPeers.GetRandomWithBudget(budget, nil)
		if err != nil {
			//TODO decide what to do
			//close(communication)
			fmt.Println("No Peers")
			return
		}

		for _, p := range peersAndBudget {
			searchRequest := &message.SearchRequest{
				Origin:   g.Name,
				Budget:   p.Budget,
				Keywords: keywords,
			}
			packetToSend := &message.GossipPacket{SearchRequest: searchRequest}

			fmt.Println("SENDING SEARCH REQUEST to " + p.Peer.Address.String())
			g.SendToPeer(p.Peer, packetToSend)

		}
	} else {

		budget = 1
		threshold := 2
		// wait for the replay //TODO timeout? resend??
		//TODO cancellarli dal db alla fine?
		for {
			select {
			//case _, ok := <-communication:
			//	if !ok {
			//		return
			//	}
			case <-time.Tick(1 * time.Second):
				// Look at the file Database if there are 2 completed matches
				if g.dbFile.CheckCompletedSearch(keywords, threshold) {
					fmt.Println("SEARCH FINISHED")
					return
				}

				budget = budget * 2
				if budget > 32 {
					//close(communication)
					return
				}
				fmt.Printf("Budget %d\n", int(budget))

				peersAndBudget, err := g.dbPeers.GetRandomWithBudget(budget, nil)
				if err != nil {
					//TODO decide what to do
					fmt.Println("No Peers")
					return
				}
				for _, p := range peersAndBudget {
					searchRequest := &message.SearchRequest{
						Origin:   g.Name,
						Budget:   p.Budget,
						Keywords: keywords,
					}
					packetToSend := &message.GossipPacket{SearchRequest: searchRequest}

					//fmt.Println("SENDING SEARCH REQUEST to " + p.Peer.Address.String())
					g.SendToPeer(p.Peer, packetToSend)
				}
			}
		}
	}

}

func (g *Gossiper) downloadMetafileSearched(keywords []string) {
	ticker := time.NewTicker(500 * time.Millisecond)
	after := time.After(20 * time.Second)
	for {
		select {
		case <-after:
			return
		case <-ticker.C:
			requestInfos := g.dbFile.GetCompletedSearches(keywords)
			for _, requestInfo := range requestInfos {
				g.requestMetafile(
					requestInfo.ChunkMap[0],
					requestInfo.FileName,
					requestInfo.MetaHash,
				)
			}
		}
	}
}

func (g *Gossiper) GetCompletedSearches(keywords []string) (ret []string) {
	requestInfos := g.dbFile.GetCompletedSearchesClient(keywords)
	for _, requestInfo := range requestInfos {
		ret = append(ret, requestInfo.FileName)
	}
	return ret
}

func (g *Gossiper) DownloadSearchedFile(destFileName string, metaHash [32]byte) error {
	//Create _Download folder if it not exists
	if _, err := os.Stat(downloadFolder); os.IsNotExist(err) {
		os.Mkdir(downloadFolder, os.ModePerm)
	}

	// get the requestInfo
	requestInfo, ok := g.dbFile.GetRequestInfo(metaHash)
	if !ok {
		return errors.New("Failed to retrieve request info")
	}
	fileInfo, ok := g.dbFile.GetMeta(requestInfo.MetaHash)
	if !ok {
		return errors.New("Failed to retrieve file info")
	}

	chunkHashes, ok := checkMetafile(fileInfo.MetaFile)
	if !ok {
		return errors.New("Failed to parse chunk hashes")
	}

	tempFileName := fileInfo.FileName

	for i := 0; i < requestInfo.ChunkNum; i++ {
		dest := requestInfo.ChunkMap[i]
		request := requestInfo.MetaHash
		// Download the Chunk
		err := g.requestChunk(dest, destFileName, tempFileName, request, chunkHashes[i], i)
		if err != nil {
			// TODO remove the tempFile and return
			fmt.Println(err)
			return err
		}
	}

	// After all the chunks are collected and the file is reconstructed, rename it
	er := os.Rename(downloadFolder+tempFileName, downloadFolder+destFileName)
	if er != nil {
		fmt.Println(er)
		return er
	}
	//Update the file name in the db
	g.dbFile.UpdateFileName(fileInfo.MetaHash, downloadFolder+destFileName)
	utils.PrintReconstructed(destFileName)

	return nil
}

func (g *Gossiper) DownloadSearchedFileFromName(destFileName string) error {
	//Create _Download folder if it not exists
	if _, err := os.Stat(downloadFolder); os.IsNotExist(err) {
		os.Mkdir(downloadFolder, os.ModePerm)
	}

	// get the requestInfo
	metaHash, ok := g.dbFile.GetMetaFromRequestName(destFileName)
	if !ok {
		return errors.New("Failed to retrieve meta hash")
	}
	requestInfo, ok := g.dbFile.GetRequestInfo(metaHash)
	if !ok {
		return errors.New("Failed to retrieve request info")
	}
	fileInfo, ok := g.dbFile.GetMeta(requestInfo.MetaHash)
	if !ok {
		return errors.New("Failed to retrieve file info")
	}

	chunkHashes, ok := checkMetafile(fileInfo.MetaFile)
	if !ok {
		return errors.New("Failed to parse chunk hashes")
	}

	tempFileName := fileInfo.FileName
	//fmt.Println("DestFileName " + destFileName)
	//fmt.Println("tempFileName " + tempFileName)
	// if the file was already downloaded, the tempfilename is equal to the file name (it was aready retitled)
	if destFileName == filepath.Base(tempFileName) {
		fmt.Println("Already downloaded")
		return nil
	}

	for i := 0; i < requestInfo.ChunkNum; i++ {
		dest := requestInfo.ChunkMap[i]
		request := requestInfo.MetaHash
		// Download the Chunk
		err := g.requestChunk(dest, destFileName, tempFileName, request, chunkHashes[i], i)
		if err != nil {
			// TODO remove the tempFile and return
			fmt.Println(err)
			return err
		}
	}

	// After all the chunks are collected and the file is reconstructed, rename it
	er := os.Rename(downloadFolder+tempFileName, downloadFolder+destFileName)
	if er != nil {
		fmt.Println(er)
		return er
	}
	//Update the file name in the db
	g.dbFile.UpdateFileName(fileInfo.MetaHash, downloadFolder+destFileName)
	utils.PrintReconstructed(destFileName)

	return nil
}
