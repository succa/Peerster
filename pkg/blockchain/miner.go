package blockchain

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/succa/Peerster/pkg/message"
	"github.com/succa/Peerster/pkg/utils"
)

type Miner struct {
	ChGossiperToMiner chan *message.GossipPacket
	ChMinerToGossiper chan *message.GossipPacket
	DbBlockchain      *DatabaseBlockchain
}

func NewMiner() *Miner {
	return &Miner{
		ChGossiperToMiner: make(chan *message.GossipPacket),
		ChMinerToGossiper: make(chan *message.GossipPacket),
		DbBlockchain:      NewDatabaseBlockchain(),
	}
}

func (m *Miner) StartMining() {
	// Thread for the mining process
	chMiningProcess := make(chan *message.Block)
	go m.MiningProcess(chMiningProcess)

	//Handle the tx and block messages
	for {
		packetReceived := <-m.ChGossiperToMiner
		if packetReceived.TxPublish != nil {

			// Check if I have already seen this tx
			alreadySeen := m.DbBlockchain.CheckTxAlreadySeen(packetReceived.TxPublish)
			// Check if the file name has already been claimed
			areadyClaimed := m.DbBlockchain.CheckFileNameAlreadyClaimed(packetReceived.TxPublish)

			if !alreadySeen && !areadyClaimed {
				// store the tx for the next block to mine
				m.DbBlockchain.AddTxInPool(packetReceived.TxPublish)

				// decrement nexthop
				packetReceived.TxPublish.HopLimit = packetReceived.TxPublish.HopLimit - 1
				if packetReceived.TxPublish.HopLimit == 0 {
					continue
				}

				//Broadcast to new peers
				m.ChMinerToGossiper <- packetReceived
			}
		} else if packetReceived.BlockPublish != nil {
			// A new Block Publish message is arrived
			block := packetReceived.BlockPublish.Block

			// Check if the block is valid
			// 1) Do I have the parent block??
			haveParent := m.DbBlockchain.CheckParentBlock(&block)
			// 2) Valid PoW
			validPoW := checkSuccesfullBlock(&block)
			// 3) TODO already in the chain
			_, haveAlready := m.DbBlockchain.GetBlock(block.Hash())
			// 4) TODO fist block
			firstBlock := m.DbBlockchain.CheckFistBlock()

			if (haveParent && validPoW && !haveAlready) || (validPoW && !haveAlready && firstBlock) {
				isOnForked := m.DbBlockchain.AddNewBlock(&block)

				// Now that we updated the last block, we have to restart the mining from this block
				// We send a message through the channel to the mining thread
				if !isOnForked {
					chMiningProcess <- &block
				}

				// decrement nexthop
				packetReceived.BlockPublish.HopLimit = packetReceived.BlockPublish.HopLimit - 1
				if packetReceived.BlockPublish.HopLimit == 0 {
					continue
				}

				//Broadcast to new peers
				m.ChMinerToGossiper <- packetReceived
			} else {
				fmt.Println("BLOCK NOT ACCEPTED")
			}
		}
	}
}

func (m *Miner) MiningProcess(chMiningProcess chan *message.Block) {

	// Infinite cicle to mine new blocks

	for {
		select {
		case block := <-chMiningProcess:
			// A mined block came from another peer
			// We have to stop mining this block and restart
			// Remove the mined txs from the pool
			m.DbBlockchain.RemoveMinedAndReset(block)

		default:
			// wait until there is at least one tx to mine
			txs := m.DbBlockchain.GetAvailableTxs()
			if len(txs) == 0 {
				time.Sleep(time.Second)
				continue
			}
			// Set the current mined txs and start mining
			m.DbBlockchain.SetCurrentlyMined(txs)

			// start Mining
			// Infinite loop that breaks when it finds the block or a new valid block arrives
			b := make([]byte, 32)
			var nonce [32]byte
			newBlock := &message.Block{
				Transactions: txs,
				PrevHash:     m.DbBlockchain.GetLastBlockHash(),
			}
			// mining process
			//time.Sleep(500 * time.Millisecond)
			rand.Read(b)
			copy(nonce[:], b)
			newBlock.Nonce = nonce
			if checkSuccesfullBlock(newBlock) {
				utils.PrintFoundBlock(newBlock)
				// Add the block to the chain
				m.DbBlockchain.AddNewBlock(newBlock)
				// Send to everybody the good news
				//time.Sleep(1 * time.Second)
				m.BroadcastNewBlock(newBlock)
				// Remove mined txs from the pool and reset Currently mined pool
				m.DbBlockchain.RemoveMinedAndReset(newBlock)

			}
		}
	}
}

func (m *Miner) BroadcastNewBlock(block *message.Block) {
	blockPublish := &message.BlockPublish{
		Block:    *block,
		HopLimit: 20,
	}
	packetToSend := &message.GossipPacket{BlockPublish: blockPublish}

	m.ChMinerToGossiper <- packetToSend
}

func (m *Miner) GetMetaFileFromBlockchain(fileName string) ([32]byte, bool) {
	return m.DbBlockchain.GetMetaFile(fileName)
}

func checkSuccesfullBlock(block *message.Block) bool {
	hash := block.Hash()
	if bytes.HasPrefix(hash[:], make([]byte, 2)) {
		return true
	}
	return false
}
