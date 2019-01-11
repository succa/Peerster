package utils

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/succa/Peerster/pkg/message"
)

func PrintClientMessage(msg string) {
	fmt.Println("CLIENT MESSAGE " + msg)
}

func PrintPeers(peers []string) {
	fmt.Println("PEERS " + strings.Join(peers, ","))
}

func PrintSimpleMessage(msg *message.SimpleMessage) {
	fmt.Println("SIMPLE MESSAGE origin " + msg.OriginalName +
		" from " + msg.RelayPeerAddr + " contents " + msg.Contents)
}

func PrintRumorMessage(msg *message.RumorMessage, relayAddr string) {
	fmt.Println("RUMOR origin " + msg.Origin + " from " + relayAddr + " ID " + strconv.Itoa(int(msg.ID)) + " contents " + msg.Text)
}

func PrintStatusMessage(msg *message.StatusPacket, relayAddr string) {
	var buffer bytes.Buffer
	buffer.WriteString("STATUS from " + relayAddr + " ")
	for _, el := range msg.Want {
		buffer.WriteString("peer " + el.Identifier + " nextID " + strconv.Itoa(int(el.NexId)) + " ")
	}
	fmt.Println(buffer.String())
}

func PrintMongering(relayAddr string) {
	fmt.Println("MONGERING with " + relayAddr)
}

func PrintFlippedCoin(addr string) {
	fmt.Println("FLIPPED COIN sending rumor to " + addr)
}

func PrintInSyncWith(addr string) {
	fmt.Println("IN SYNC WITH " + addr)
}

func PrintDSDV(origin string, address string) {
	fmt.Println("DSDV " + origin + " " + address)
}

func PrintPrivateMessage(msg *message.PrivateMessage) {
	fmt.Println("PRIVATE origin " + msg.Origin + " hop-limit " + strconv.Itoa(int(msg.HopLimit)) + " contents " + msg.Text)
}

func PrintSendingPrivateMessage(msg *message.PrivateMessage) {
	fmt.Println("SENDING PRIVATE MESSAGE " + msg.Text + " TO " + msg.Destination)
}

func PrintRequestIndexing(filename string) {
	fmt.Println("REQUESTING INDEXING filename " + filename)
}

func PrintRequestFile(filename string, dest string, request string) {
	fmt.Println("REQUESTING filename " + filename + " from " + dest + " hash " + request)
}

func PrintDownloadingMetafile(file string, dest string) {
	fmt.Println("DOWNLOADING metafile of " + file + " from " + dest)
}

func PrintDownloadingChunk(file string, index int, dest string) {
	fmt.Println("DOWNLOADING " + file + " chunk " + strconv.Itoa(int(index+1)) + " from " + dest)
}

func PrintReconstructed(file string) {
	fmt.Println("RECONSTRUCTED file " + file)
}

func PrintFoundMatch(file string, dest string, metafilehash []byte, chunkMap []uint64) {
	//sort chunkMap (just to be sure)
	sort.Slice(chunkMap, func(i, j int) bool { return chunkMap[i] < chunkMap[j] })
	var s []string
	for _, chunk := range chunkMap {
		s = append(s, strconv.FormatUint(chunk+1, 10))
	}
	fmt.Printf("FOUND match %s at %s metafile=%x chunks=%s\n", file, dest, metafilehash, strings.Join(s, ","))
}

func PrintFoundBlock(block *message.Block) {
	fmt.Printf("FOUND-BLOCK [%x]\n", block.Hash())
}

func PrintChain(blocks []*message.Block) {
	fmt.Printf("CHAIN ")
	for i := len(blocks) - 1; i >= 0; i-- {
		fmt.Printf("[%x] ", blocks[i].Hash())
	}
	fmt.Printf("\n")
}

func PrintForkLonger(rewind int) {
	fmt.Printf("FORK-LONGER rewind %d blocks\n", rewind)
}

func PrintForkShorter(block *message.Block) {
	fmt.Printf("FORK-SHORTER %x\n", block.Hash())
}

func PrintFoundOnionBlock(block *message.BlockOnion) {
	fmt.Printf("ONION FOUND-BLOCK %x\n", block.Hash())
}

func PrintChainOnion(blocks []*message.BlockOnion) {
	fmt.Printf("ONION CHAIN ")
	for i := len(blocks) - 1; i >= 0; i-- {
		fmt.Printf("%x:", blocks[i].Hash())
		fmt.Printf("%x", blocks[i].PrevHash)
		num_tx := len(blocks[i].Transactions)
		if num_tx > 0 {
			fmt.Printf(":")
		}
		for i, tx := range blocks[i].Transactions {
			fmt.Printf("%s", tx.NodeName)
			if i != num_tx-1 {
				fmt.Printf(",")
			}
		}
		fmt.Printf(" ")
	}
	fmt.Printf("\n")
	/*
		for i := len(blocks) - 1; i >= 0; i-- {
			fmt.Printf("[%x] ", blocks[i].Hash())
		}
		fmt.Printf("\n")
	*/
}

func PrintForkLongerOnion(rewind int) {
	fmt.Printf("ONION FORK-LONGER rewind %d blocks\n", rewind)
}

func PrintForkShorterOnion(block *message.BlockOnion) {
	fmt.Printf("ONION FORK-SHORTER %x\n", block.Hash())
}

func PrintOnionMessage(onion *message.OnionMessage) {
	fmt.Print("ONION MESSAGE Destination %s Chiper %x\n", onion.Destination, onion.Cipher)
}
