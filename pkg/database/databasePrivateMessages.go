package database

import (
	"sort"
	"sync"
	"time"

	"github.com/succa/Peerster/pkg/message"
)

type DatabasePrivateEntry struct {
	Private *message.PrivateMessage
	Date    time.Time
}

type DatabasePrivateMessages struct {
	db  map[string][]*DatabasePrivateEntry
	mux sync.RWMutex
}

func NewDatabasePrivateMessages() *DatabasePrivateMessages {
	return &DatabasePrivateMessages{db: make(map[string][]*DatabasePrivateEntry)}
}

func convertPrivateToEntry(private *message.PrivateMessage) *DatabasePrivateEntry {
	return &DatabasePrivateEntry{Private: private, Date: time.Now()}
}

func (d *DatabasePrivateMessages) InsertMessage(dest string, msg *message.PrivateMessage) {
	d.mux.Lock()
	defer d.mux.Unlock()

	dbEntry := convertPrivateToEntry(msg)

	list, ok := d.db[dest]
	if !ok {
		d.db[dest] = []*DatabasePrivateEntry{dbEntry}
	} else {
		d.db[dest] = append(list, dbEntry)
	}
}

/*
func (d *DatabaseMessages) PrintDb() {
	d.mux.RLock()
	defer d.mux.RUnlock()

	for key, val := range d.db {
		println("Identifier: " + key + " " + strconv.Itoa(len(val)))
	}
}
*/

func PrivateMap(vs []*DatabasePrivateEntry, f func(*DatabasePrivateEntry) *message.PrivateMessage) []*message.PrivateMessage {
	vsm := make([]*message.PrivateMessage, len(vs))
	for i, v := range vs {
		vsm[i] = f(v)
	}
	return vsm
}
func (d *DatabasePrivateMessages) GetPrivateMessages(dest string) []*message.PrivateMessage {
	d.mux.RLock()
	defer d.mux.RUnlock()
	list, _ := d.db[dest]
	sort.Slice(list, func(i, j int) bool {
		return list[i].Date.Unix() < list[j].Date.Unix()
	})
	ret := PrivateMap(list, func(entry *DatabasePrivateEntry) *message.PrivateMessage {
		return entry.Private
	})
	return ret
}
