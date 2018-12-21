package gossiper

import (
	"fmt"
	"net"

	"github.com/succa/Peerster/pkg/blockchain"
	database "github.com/succa/Peerster/pkg/database"
	peer "github.com/succa/Peerster/pkg/peer"
	utils "github.com/succa/Peerster/pkg/utils"
)

const clientDefaultAddress = "127.0.0.1"
const timeForTheAck = 1
const timeForAntyEntropy = 20
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
	dbFileCh          *database.FileDatabaseChannels
	miner             *blockchain.Miner
	searchHelper      *utils.SearchHelper
	searchDuplicates  *utils.TTLSearchRequest
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
		dbFileCh:          database.NewFileDatabaseChannels(),
		miner:             blockchain.NewMiner(),
		searchHelper:      utils.NewSearchHelper(),
		searchDuplicates:  utils.NewTTLSearchRequest(int64(0.5e9)),
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

	// Blockchain Mining
	go g.Mining()

	// Anri-entropy -- BLOCKING
	g.antiEntropy()
}

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
