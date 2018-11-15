package database

import (
	"reflect"
	"sync"
)

type RoutingTable struct {
	Db  map[string]string
	mux sync.RWMutex
}

func NewRoutingTable() *RoutingTable {
	return &RoutingTable{Db: make(map[string]string)}
}

func (r *RoutingTable) UpdateRoute(origin string, address string) bool {
	r.mux.Lock()
	defer r.mux.Unlock()

	_, ok := r.Db[origin]
	if ok {
		return false
	}
	r.Db[origin] = address
	return true
}

func (r *RoutingTable) GetRoute(origin string) (string, bool) {
	r.mux.RLock()
	defer r.mux.RUnlock()

	value, ok := r.Db[origin]
	return value, ok
}

func (r *RoutingTable) GetKeys() []string {
	r.mux.RLock()
	defer r.mux.RUnlock()

	keys := reflect.ValueOf(r.Db).MapKeys()
	strkeys := make([]string, len(keys))
	if len(keys) == 0 {
		return nil
	}
	for i := 0; i < len(keys); i++ {
		strkeys[i] = keys[i].String()
	}
	return strkeys
}
