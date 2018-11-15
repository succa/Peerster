package database

import "sync"

type Counter struct {
	ID  uint32
	mux sync.RWMutex
}

func NewCounter() *Counter {
	return &Counter{ID: 0}
}

func (c *Counter) Next() uint32 {
	c.mux.Lock()
	c.ID = c.ID + 1
	c.mux.Unlock()
	return c.ID
}
