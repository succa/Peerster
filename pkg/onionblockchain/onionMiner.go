package onionblockchain

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/succa/Peerster/pkg/message"
	"github.com/succa/Peerster/pkg/onion"
	"github.com/succa/Peerster/pkg/utils"
)

type Miner struct {
	name              string
	ChGossiperToMiner chan *message.GossipPacket
	ChMinerToGossiper chan *message.GossipPacket
	DbBlockchain      *DatabaseBlockchain
}

func NewMiner(name string) *Miner {
	return &Miner{
		name:              name,
		ChGossiperToMiner: make(chan *message.GossipPacket),
		ChMinerToGossiper: make(chan *message.GossipPacket),
		DbBlockchain:      NewDatabaseBlockchain(),
	}
}

func (m *Miner) StartMining() {
	// Thread for the mining process
	go m.MiningProcess()

	//Handle the tx and block messages
	for {
		packetReceived := <-m.ChGossiperToMiner
		if packetReceived.TxOnionPeer != nil {

			// Check if I have already seen this tx
			alreadySeen := m.DbBlockchain.CheckTxAlreadySeen(packetReceived.TxOnionPeer)
			// Check if the file name has already been claimed
			areadyClaimed := m.DbBlockchain.CheckNameAlreadyClaimed(packetReceived.TxOnionPeer)

			if !alreadySeen && !areadyClaimed {
				// store the tx for the next block to mine
				m.DbBlockchain.AddTxInPool(packetReceived.TxOnionPeer)

				// decrement nexthop
				packetReceived.TxOnionPeer.HopLimit = packetReceived.TxOnionPeer.HopLimit - 1
				if packetReceived.TxOnionPeer.HopLimit == 0 {
					continue
				}

				//Broadcast to new peers
				m.ChMinerToGossiper <- packetReceived
			}
		} else if packetReceived.BlockOnionPublish != nil {
			// A new Block Publish message is arrived
			block := packetReceived.BlockOnionPublish.Block

			// Check if the block is valid
			// 1) Do I have the parent block??
			haveParent := m.DbBlockchain.CheckParentBlock(&block)
			// 2) Valid PoW
			validPoW := checkSuccesfullBlock(&block)
			// 3) TODO already in the chain
			_, haveAlready := m.DbBlockchain.GetBlock(block.Hash())

			haveInTemp := m.DbBlockchain.HaveInTemporary(block.PrevHash)

			if validPoW && !haveAlready && !haveInTemp {
				if haveParent {
					m.DbBlockchain.AddNewBlock(&block)
				} else {
					m.DbBlockchain.AddToTemporary(&block)
					go m.periodicRequest(block.PrevHash)
				}

				// decrement nexthop
				packetReceived.BlockOnionPublish.HopLimit = packetReceived.BlockOnionPublish.HopLimit - 1
				if packetReceived.BlockOnionPublish.HopLimit == 0 {
					continue
				}

				//Broadcast to new peers
				m.ChMinerToGossiper <- packetReceived
			} else {
				//fmt.Println("BLOCK NOT ACCEPTED")
			}
		} else if packetReceived.BlockRequest != nil {
			block, haveAlready := m.DbBlockchain.GetBlock(packetReceived.BlockRequest.BlockHash)
			if haveAlready {
				fmt.Printf("ONION SEND BLOCK WITH HASH %x\n", block.Hash())
				blockReply := &message.BlockReply{
					Origin:      m.name,
					Destination: packetReceived.BlockRequest.Origin,
					Block:       *block,
					BlockHash:   block.Hash(),
					HopLimit:    20,
				}
				packetToSend := &message.GossipPacket{BlockReply: blockReply}
				m.ChMinerToGossiper <- packetToSend
			} else {
				m.ChMinerToGossiper <- packetReceived
			}
		} else if packetReceived.BlockReply != nil {
			block := packetReceived.BlockReply.Block
			if block.Hash() == packetReceived.BlockReply.BlockHash {
				fmt.Printf("ONION RECEIVED BLOCK WITH HASH %x\n", block.Hash())
				// Check if the block is valid
				// 1) Do I have the parent block??
				haveParent := m.DbBlockchain.CheckParentBlock(&block)
				// 2) Valid PoW
				validPoW := checkSuccesfullBlock(&block)
				// 3) TODO already in the chain
				_, haveAlready := m.DbBlockchain.GetBlock(block.Hash())
				haveInTemp := m.DbBlockchain.HaveInTemporary(block.PrevHash)

				if validPoW && !haveAlready && !haveInTemp {
					if haveParent {
						m.DbBlockchain.AddNewBlock(&block)
					} else {
						m.DbBlockchain.AddToTemporary(&block)
						go m.periodicRequest(block.PrevHash)
					}
				}
			}
		}
	}
}

func (m *Miner) MiningProcess() {

	// Infinite cicle to mine new blocks
	time.Sleep(3 * time.Second)

	for {
		txs, prevHash := m.DbBlockchain.GetAvailableTxsAndLastBlockHash()

		// start Mining
		// Infinite loop that breaks when it finds the block or a new valid block arrives
		b := make([]byte, 32)
		var nonce [32]byte
		newBlock := &message.BlockOnion{
			MinerName:    m.name,
			Transactions: txs,
			PrevHash:     prevHash,
		}
		// mining process
		//time.Sleep(500 * time.Millisecond)
		//fmt.Println("Riccardo mining.......")
		rand.Read(b)
		copy(nonce[:], b)
		newBlock.Nonce = nonce
		if checkSuccesfullBlock(newBlock) {
			utils.PrintFoundOnionBlock(newBlock)
			// Add the block to the chain
			m.DbBlockchain.AddNewBlock(newBlock)
			// Send to everybody the good news
			//time.Sleep(1 * time.Second)
			m.BroadcastNewBlock(newBlock)
			// Remove mined txs from the pool and reset Currently mined pool
		}
	}
}

func (m *Miner) BroadcastNewBlock(block *message.BlockOnion) {
	blockPublish := &message.BlockOnionPublish{
		Block:    *block,
		HopLimit: 20,
	}
	packetToSend := &message.GossipPacket{BlockOnionPublish: blockPublish}

	m.ChMinerToGossiper <- packetToSend
}

func checkSuccesfullBlock(block *message.BlockOnion) bool {
	hash := block.Hash()
	if bytes.HasPrefix(hash[:], make([]byte, 2)) {
		return true
	}
	return false
}

func (m *Miner) GetRoute(destination string) ([]*onion.OnionRouter, error) {
	return m.DbBlockchain.GetRoute(destination, m.name)
}

func (m *Miner) periodicRequest(hash [32]byte) {
	// Send a status message to a random peer every x seconds
	ticker := time.NewTicker(time.Duration(10) * time.Second)
	for {
		_, haveAlready := m.DbBlockchain.GetBlock(hash)
		if haveAlready {
			return
		}
		fmt.Printf("ONION REQUEST BLOCK WITH HASH %x\n", hash)
		blockRequest := &message.BlockRequest{
			Origin:    m.name,
			BlockHash: hash,
			HopLimit:  20,
		}
		packetToSend := &message.GossipPacket{BlockRequest: blockRequest}
		m.ChMinerToGossiper <- packetToSend
		<-ticker.C
	}
}
