package database

import (
	"sync"

	"github.com/succa/Peerster/pkg/message"
)

type RequestFileDatabase struct {
	db  map[string](chan message.GossipPacket)
	mux sync.RWMutex
}

func NewRequestFileDatabase() *RequestFileDatabase {
	return &RequestFileDatabase{db: make(map[string](chan message.GossipPacket))}
}

func (d *RequestFileDatabase) Get(dest string) (chan message.GossipPacket, bool) {
	d.mux.RLock()
	defer d.mux.RUnlock()
	ch, err := d.db[dest]
	return ch, err
}

func (d *RequestFileDatabase) Add(dest string, ch chan message.GossipPacket) {
	d.mux.Lock()
	defer d.mux.Unlock()
	d.db[dest] = ch
}

func (d *RequestFileDatabase) Delete(dest string) {
	d.mux.Lock()
	defer d.mux.Unlock()
	delete(d.db, dest)
}
