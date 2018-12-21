package utils

import (
	"sync"
	"time"
)

type couple struct {
	origin   string
	keywords string
}

type TTLSearchRequest struct {
	db  map[couple]time.Time
	mux sync.RWMutex
}

func NewTTLSearchRequest(maxNanoTTL int64) *TTLSearchRequest {
	m := &TTLSearchRequest{db: make(map[couple]time.Time)}
	go func() {
		for now := range time.Tick(10 * time.Millisecond) {
			m.mux.Lock()
			for k, v := range m.db {
				if now.UnixNano()-v.UnixNano() > maxNanoTTL {
					delete(m.db, k)
				}
			}
			m.mux.Unlock()
		}
	}()
	return m
}

func (t *TTLSearchRequest) Insert(origin, keywords string) {
	t.mux.Lock()
	defer t.mux.Unlock()

	_, ok := t.db[couple{origin, keywords}]
	if !ok {
		t.db[couple{origin, keywords}] = time.Now()
	}
}

func (t *TTLSearchRequest) IsDuplicate(origin, keywords string) bool {
	t.mux.RLock()
	defer t.mux.RUnlock()

	_, ok := t.db[couple{origin, keywords}]
	return ok
}
