package database

import (
	"sync"

	"github.com/succa/Peerster/pkg/message"
)

type FileDatabaseChannels struct {
	db  map[string](chan message.GossipPacket)
	mux sync.RWMutex
}

func NewFileDatabaseChannels() *FileDatabaseChannels {
	return &FileDatabaseChannels{db: make(map[string](chan message.GossipPacket))}
}

func (d *FileDatabaseChannels) LockDest(dest string, channel chan message.GossipPacket) bool {
	d.mux.Lock()
	defer d.mux.Unlock()
	_, ok := d.db[dest]
	if ok {
		return false
	} else {
		d.db[dest] = channel
		return true
	}
}

func (d *FileDatabaseChannels) Add(dest string, ch chan message.GossipPacket) {
	d.mux.Lock()
	defer d.mux.Unlock()
	d.db[dest] = ch
}

func (d *FileDatabaseChannels) Get(dest string) (chan message.GossipPacket, bool) {
	d.mux.RLock()
	defer d.mux.RUnlock()
	ch, ok := d.db[dest]
	return ch, ok
}

func (d *FileDatabaseChannels) Delete(dest string) {
	d.mux.Lock()
	defer d.mux.Unlock()
	delete(d.db, dest)
}
