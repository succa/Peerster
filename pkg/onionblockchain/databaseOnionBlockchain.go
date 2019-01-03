package onionblockchain

import (
	"sync"
	"math/rand"
	"fmt"

	"github.com/succa/Peerster/pkg/message"
	"github.com/succa/Peerster/pkg/utils"
)

type DatabaseBlockchain struct {
	TxPool           map[string]*message.TxOnionPeer
	Blockchain       map[[32]byte]*message.BlockOnion
	TemporaryStorage map[[32]byte]*message.BlockOnion
	LastBlock        *message.BlockOnion
	LongestChainSize int
	Keys             map[string][]byte
	mux              sync.RWMutex
}

func NewDatabaseBlockchain() *DatabaseBlockchain {
	return &DatabaseBlockchain{
		TxPool:           make(map[string]*message.TxOnionPeer),
		Blockchain:       make(map[[32]byte]*message.BlockOnion),
		TemporaryStorage: make(map[[32]byte]*message.BlockOnion),
		LongestChainSize: 0,
		Keys:             make(map[string][]byte),
	}
}

func (b *DatabaseBlockchain) AddToTemporary(block *message.BlockOnion) {
	b.mux.Lock()
	defer b.mux.Unlock()
	fmt.Printf("ONION ADD TO TEMP %x WAIT FOR %x\n", block.Hash(), block.PrevHash)
	b.TemporaryStorage[block.PrevHash] = block
}

func (b *DatabaseBlockchain) HaveInTemporary(hash [32]byte) bool {
	b.mux.RLock()
	defer b.mux.RUnlock()

	_, ok := b.TemporaryStorage[hash]
	return ok
} 

func (b *DatabaseBlockchain) AddNewBlock(block *message.BlockOnion) {
	b.mux.Lock()
	defer b.mux.Unlock()

	// Add the new block to the chain
	for {
		b.Blockchain[block.Hash()] = block
		chain := b.getChain(block)

		if b.LastBlock == nil || block.PrevHash == b.LastBlock.Hash() {
			//update last block
			b.LastBlock = block
			//update chain length
			b.LongestChainSize = b.LongestChainSize + 1
			//update file to metahash
			b.updateKeysDb(block)
			// Print the new chain
			utils.PrintChainOnion(chain)
			//return false
		} else {
			if len(chain) > b.LongestChainSize {
				//fmt.Println("NEW LONGEST CHAIN")
				// new longest chain!
				// we have to roll back and add the txs of the new chain
				n := b.rollback(chain)
				//update last block
				b.LastBlock = block
				//update chain length
				b.LongestChainSize = len(chain)

				utils.PrintForkLongerOnion(n)
				utils.PrintChainOnion(chain)
				//return false
			} else {
				// shorter chain
				utils.PrintForkShorterOnion(block)
				//return true
			}
		}
		_, ok := b.TemporaryStorage[block.Hash()]
		if !ok {
			break
		}
		block = b.TemporaryStorage[block.Hash()]
		delete(b.TemporaryStorage, block.PrevHash)
	}
}


func (b *DatabaseBlockchain) rollback(newLongestChain []*message.BlockOnion) int {
	//find the common uncestor with the current lastblock
	previousLongestChain := b.getChain(b.LastBlock)
	//fmt.Print("Previous longest chain: ")
	//utils.PrintChain(previousLongestChain)

	rollbackUpTo := 0
	for i := len(newLongestChain) - 1; i >= 0; i-- {
		if b.contains(previousLongestChain, newLongestChain[i]) {
			rollbackUpTo = i + 1
			break
		}
	}
	//fmt.Printf("rollbackSize %d\n", rollbackSize)

	//rollback the file-to-metahash mapping
	for i := len(previousLongestChain) - 1; i >= rollbackUpTo; i-- {
		block := previousLongestChain[i]
		//fmt.Printf("Eliminating block %x\n", block.Hash())
		for _, tx := range block.Transactions {
			delete(b.Keys, tx.NodeName)
			b.TxPool[tx.NodeName] = &tx
		}
	}

	//adding the txs of the newlongestchain blocks
	for i := rollbackUpTo; i < len(newLongestChain); i++ {
		//fmt.Printf("ADDING TXS OF THE BLOCK %x\n", newLongestChain[i].Hash())
		b.updateKeysDb(newLongestChain[i])
	}

	return len(previousLongestChain) - rollbackUpTo
}

func (b *DatabaseBlockchain) contains(chain []*message.BlockOnion, block *message.BlockOnion) bool {
	for i := len(chain) - 1; i >= 0; i-- {
		if chain[i].Hash() == block.Hash() {
			return true
		}
	}
	return false
}

func (b *DatabaseBlockchain) CheckTxAlreadySeen(tx *message.TxOnionPeer) bool {
	b.mux.RLock()
	defer b.mux.RUnlock()

	_, ok := b.TxPool[tx.NodeName]
	return ok
}

func (b *DatabaseBlockchain) CheckNameAlreadyClaimed(tx *message.TxOnionPeer) bool {
	b.mux.RLock()
	defer b.mux.RUnlock()

	_, ok := b.Keys[tx.NodeName]
	return ok
}


func (b *DatabaseBlockchain) CheckParentBlock(block *message.BlockOnion) bool {
	b.mux.RLock()
	defer b.mux.RUnlock()

	if block.PrevHash == [32]byte{0} {
		return true
	}

	_, ok := b.Blockchain[block.PrevHash]
	return ok
}

func (b *DatabaseBlockchain) AddTxInPool(tx *message.TxOnionPeer) {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.TxPool[tx.NodeName] = tx
}

func (b *DatabaseBlockchain) GetBlock(hash [32]byte) (*message.BlockOnion, bool) {
	b.mux.RLock()
	defer b.mux.RUnlock()

	block, ok := b.Blockchain[hash]
	return block, ok
}

func (b *DatabaseBlockchain) GetAvailableTxsAndLastBlockHash() ([]message.TxOnionPeer, [32]byte) {
	b.mux.RLock()
	defer b.mux.RUnlock()

	hash := [32]byte{0}

	if b.LastBlock != nil {
		hash = b.LastBlock.Hash()
	}

	available := make([]message.TxOnionPeer, 0, len(b.TxPool))
	for _, value := range b.TxPool {
		available = append(available, *value)
	}

	return available, hash
}

func (b *DatabaseBlockchain) getChain(block *message.BlockOnion) []*message.BlockOnion {
	if block == nil {
		return []*message.BlockOnion{}
	}
	l := []*message.BlockOnion{block}
	currentBlock := block
	for {
		if currentBlock.PrevHash == [32]byte{0} {
			return l
		}
		previousBlock, ok := b.Blockchain[currentBlock.PrevHash]
		if !ok {
			return l
		}
		l = append([]*message.BlockOnion{previousBlock}, l...)
		currentBlock = previousBlock
	}
}

func (b *DatabaseBlockchain) updateKeysDb(block *message.BlockOnion) {
	for _, tx := range block.Transactions {
		b.Keys[tx.NodeName] = tx.PublicKey
		delete(b.TxPool, tx.NodeName)
	}
}

func (b *DatabaseBlockchain) GetRoute(destination string, my string) map[string][]byte {
	b.mux.RLock()
	defer b.mux.RUnlock()
	result := make(map[string][]byte)
	result[destination] = b.Keys[destination]
	keys := make([]string,0)
	for k, _ := range b.Keys {
		if k != destination && k != my {
			keys = append(keys, k)
		}
	}
	rand.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	peers := keys[:3]
	for _, v := range peers {
		result[v] = b.Keys[v]
	}
	return result
}

/*func (b *DatabaseBlockchain) GetAvailableTxs() []message.TxOnionPeer {
	b.mux.RLock()
	defer b.mux.RUnlock()

	available := make([]message.TxOnionPeer, 0, len(b.TxPool))
	for _, value := range b.TxPool {
		available = append(available, *value)
	}

	return available
}*/

/*func (b *DatabaseBlockchain) nodeInBlockchain(name string) bool {
	_, ok := b.Keys[name]
	return ok
}

func contains(l []message.TxOnionPeer, name string) bool {
	for _, tx := range l {
		if tx.NodeName == name {
			return true
		}
	}
	return false
}*/


/*func (b *DatabaseBlockchain) GetLastBlockHash() [32]byte {
	b.mux.RLock()
	defer b.mux.RUnlock()

	if b.LastBlock != nil {
		return b.LastBlock.Hash()
	} else {
		return [32]byte{0}
	}
}*/
