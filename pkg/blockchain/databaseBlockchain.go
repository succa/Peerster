package blockchain

import (
	"sync"

	"github.com/succa/Peerster/pkg/message"
	"github.com/succa/Peerster/pkg/utils"
)

type DatabaseBlockchain struct {
	TxPool           map[[32]byte]*message.TxPublish
	CurrentlyMined   map[[32]byte]*message.TxPublish
	Blockchain       map[[32]byte]*message.Block
	LastBlock        *message.Block
	LongestChainSize int
	FileNames        map[string][32]byte // this is the metahash
	FirstBlock       bool
	mux              sync.RWMutex
}

func NewDatabaseBlockchain() *DatabaseBlockchain {
	return &DatabaseBlockchain{
		TxPool:           make(map[[32]byte]*message.TxPublish),
		CurrentlyMined:   make(map[[32]byte]*message.TxPublish),
		Blockchain:       make(map[[32]byte]*message.Block),
		LongestChainSize: 0,
		FileNames:        make(map[string][32]byte),
		FirstBlock:       true,
	}
}

func (b *DatabaseBlockchain) AddNewBlock(block *message.Block) bool {
	b.mux.Lock()
	defer b.mux.Unlock()

	//utils.PrintFoundBlock(block)
	//fmt.Printf("Adding block %x\n", block.Hash())

	//FirstBlock
	if b.FirstBlock {
		b.FirstBlock = false
	}

	// Add the new block to the chain
	b.Blockchain[block.Hash()] = block
	chain := b.getChain(block)

	if b.LastBlock == nil || block.PrevHash == b.LastBlock.Hash() {
		//update last block
		b.LastBlock = block
		//update chain length
		b.LongestChainSize = b.LongestChainSize + 1
		//update file to metahash
		b.updateFileToMetahashDb(block)
		// Print the new chain
		utils.PrintChain(chain)
		return false
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

			utils.PrintForkLonger(n)
			utils.PrintChain(chain)
			return false
		} else {
			// shorter chain
			utils.PrintForkShorter(block)
			return true
		}
	}
}

func (b *DatabaseBlockchain) GetMetaFile(fileName string) ([32]byte, bool) {
	b.mux.Lock()
	defer b.mux.Unlock()

	meta, ok := b.FileNames[fileName]
	return meta, ok
}

func (b *DatabaseBlockchain) rollback(newLongestChain []*message.Block) int {
	//find the common uncestor with the current lastblock
	previousLongestChain := b.getChain(b.LastBlock)
	//fmt.Print("Previous longest chain: ")
	//utils.PrintChain(previousLongestChain)

	rollbackSize := len(previousLongestChain) - 1
	for i := len(newLongestChain) - 1; i >= 0; i-- {
		if b.contains(previousLongestChain, newLongestChain[i]) {
			rollbackSize = i
			break
		}
	}
	//fmt.Printf("rollbackSize %d\n", rollbackSize)

	//rollback the file-to-metahash mapping
	for i := len(previousLongestChain) - 1; i >= rollbackSize; i-- {
		block := previousLongestChain[i]
		//fmt.Printf("Eliminating block %x\n", block.Hash())
		for _, tx := range block.Transactions {
			delete(b.FileNames, tx.File.Name)
		}
	}

	//adding the txs of the newlongestchain blocks
	for i := rollbackSize; i < len(newLongestChain); i++ {
		//fmt.Printf("ADDING TXS OF THE BLOCK %x\n", newLongestChain[i].Hash())
		b.updateFileToMetahashDb(newLongestChain[i])
	}

	return rollbackSize + 1
}

func (b *DatabaseBlockchain) contains(chain []*message.Block, block *message.Block) bool {
	for i := len(chain) - 1; i >= 0; i-- {
		if chain[i].Hash() == block.Hash() {
			return true
		}
	}
	return false
}

func (b *DatabaseBlockchain) CheckTxAlreadySeen(tx *message.TxPublish) bool {
	b.mux.RLock()
	defer b.mux.RUnlock()

	_, ok := b.TxPool[tx.Hash()]
	return ok
}

func (b *DatabaseBlockchain) CheckFileNameAlreadyClaimed(tx *message.TxPublish) bool {
	b.mux.RLock()
	defer b.mux.RUnlock()

	_, ok := b.FileNames[tx.File.Name]
	return ok
}

func (b *DatabaseBlockchain) CheckFistBlock() bool {
	b.mux.RLock()
	defer b.mux.RUnlock()

	return b.FirstBlock
}

func (b *DatabaseBlockchain) CheckParentBlock(block *message.Block) bool {
	b.mux.Lock()
	defer b.mux.Unlock()

	if block.PrevHash == [32]byte{0} {
		return true
	}

	_, ok := b.Blockchain[block.PrevHash]
	return ok
}

func (b *DatabaseBlockchain) AddTxInPool(tx *message.TxPublish) {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.TxPool[tx.Hash()] = tx
}

func (b *DatabaseBlockchain) GetBlock(hash [32]byte) (*message.Block, bool) {
	b.mux.Lock()
	defer b.mux.Unlock()

	block, ok := b.Blockchain[hash]
	return block, ok
}

func (b *DatabaseBlockchain) FlushTxsPool() {
	b.mux.Lock()
	defer b.mux.Unlock()

	b.TxPool = make(map[[32]byte]*message.TxPublish)
}

func (b *DatabaseBlockchain) SetCurrentlyMined(txs []message.TxPublish) {
	b.mux.Lock()
	defer b.mux.Unlock()

	for _, tx := range txs {
		b.CurrentlyMined[tx.Hash()] = &tx
	}
}

func (b *DatabaseBlockchain) GetAvailableTxs() []message.TxPublish {
	b.mux.Lock()
	defer b.mux.Unlock()

	available := make([]message.TxPublish, 0, len(b.TxPool))
	for _, value := range b.TxPool {
		if !contains(available, value.File.Name) && !b.fileInBlockchain(value.File.Name) {
			available = append(available, *value)
		}
	}

	return available
}

func contains(l []message.TxPublish, fileName string) bool {
	for _, tx := range l {
		if tx.File.Name == fileName {
			return true
		}
	}
	return false
}
func (b *DatabaseBlockchain) fileInBlockchain(fileName string) bool {
	_, ok := b.FileNames[fileName]
	return ok
}

func (b *DatabaseBlockchain) RemoveMinedAndReset(block *message.Block) {
	b.mux.Lock()
	defer b.mux.Unlock()

	for _, tx := range block.Transactions {
		delete(b.TxPool, tx.Hash())
	}
	b.CurrentlyMined = make(map[[32]byte]*message.TxPublish)
}

func (b *DatabaseBlockchain) GetLastBlockHash() [32]byte {
	b.mux.Lock()
	defer b.mux.Unlock()

	if b.LastBlock != nil {
		return b.LastBlock.Hash()
	} else {
		return [32]byte{0}
	}
}

func (b *DatabaseBlockchain) getChain(block *message.Block) []*message.Block {
	if block == nil {
		return []*message.Block{}
	}
	l := []*message.Block{block}
	currentBlock := block
	for {
		if currentBlock.PrevHash == [32]byte{0} {
			return l
		}
		previousBlock, ok := b.Blockchain[currentBlock.PrevHash]
		if !ok {
			return l
		}
		l = append(l, previousBlock)
		currentBlock = previousBlock
	}
}

func (b *DatabaseBlockchain) updateFileToMetahashDb(block *message.Block) {
	for _, tx := range block.Transactions {
		var metahash [32]byte
		copy(metahash[:], tx.File.MetafileHash)
		b.FileNames[tx.File.Name] = metahash
	}
}
