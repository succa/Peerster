package database

import (
	"errors"
	"math/rand"
	"reflect"
	"sync"

	"github.com/succa/Peerster/pkg/peer"
)

type DatabasePeers struct {
	Db  map[string]*peer.Peer
	mux sync.RWMutex
}

func NewDatabasePeers() *DatabasePeers {
	return &DatabasePeers{Db: make(map[string]*peer.Peer)}
}

func (d *DatabasePeers) Insert(peer *peer.Peer) {
	d.mux.Lock()
	defer d.mux.Unlock()
	if _, ok := d.Db[peer.Address.String()]; !ok {
		d.Db[peer.Address.String()] = peer
	}
}

func (d *DatabasePeers) Get(addr string) *peer.Peer {
	return d.Db[addr]
}

func (d *DatabasePeers) GetKeys() []string {
	d.mux.RLock()
	defer d.mux.RUnlock()
	keys := reflect.ValueOf(d.Db).MapKeys()
	strkeys := make([]string, len(keys))
	if len(keys) == 0 {
		return nil
	}
	for i := 0; i < len(keys); i++ {
		strkeys[i] = keys[i].String()
	}
	return strkeys
}

func (d *DatabasePeers) GetValues() []*peer.Peer {
	list := make([]*peer.Peer, len(d.Db))
	d.mux.RLock()
	defer d.mux.RUnlock()
	for _, value := range d.Db {
		list = append(list, value)
	}
	return list
}

func (d *DatabasePeers) GetRandom(toAvoid map[string]struct{}) (*peer.Peer, error) {
	keys := d.GetKeys()
	if len(toAvoid) == len(keys) {
		return nil, errors.New("No peer available")
	}
	randomAccess := rand.Perm(len(keys))
	for _, i := range randomAccess {
		if _, ok := toAvoid[keys[i]]; !ok {
			return d.Db[keys[i]], nil
		}
	}
	return nil, errors.New("No peer available")
}
