package database

import (
	"sync"

	"github.com/succa/Peerster/pkg/message"
)

type DatabaseChannels struct {
	db  map[string](chan message.GossipPacket)
	mux sync.RWMutex
}

func NewDatabaseChannels() *DatabaseChannels {
	return &DatabaseChannels{db: make(map[string](chan message.GossipPacket))}
}

func (d *DatabaseChannels) Get(addr string) (chan message.GossipPacket, bool) {
	d.mux.RLock()
	defer d.mux.RUnlock()
	ch, err := d.db[addr]
	return ch, err
}

func (d *DatabaseChannels) Add(addr string, ch chan message.GossipPacket) {
	d.mux.Lock()
	defer d.mux.Unlock()
	d.db[addr] = ch
}

func (d *DatabaseChannels) Delete(addr string) {
	d.mux.Lock()
	defer d.mux.Unlock()
	delete(d.db, addr)
}
