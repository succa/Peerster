package utils

import (
	"bytes"
	"fmt"
	"path/filepath"
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
	fmt.Println("DOWNLOADING metafile of " + filepath.Base(file) + " from " + dest)
}

func PrintDownloadingChunk(file string, index int, dest string) {
	fmt.Println("DOWNLOADING " + filepath.Base(file) + " chunk " + strconv.Itoa(int(index)) + " from " + dest)
}

func PrintReconstructed(file string) {
	fmt.Println("RECONSTRUCTED file " + filepath.Base(file))
}
