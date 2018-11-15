package database

import (
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/succa/Peerster/pkg/message"
)

//var StartTime = time.Now().UnixNano()
var StartTime = int64(0)

type DatabaseEntry struct {
	Rumor    *message.RumorMessage
	TimeNano int64
}

type DatabaseMessages struct {
	db  map[string][]*DatabaseEntry
	mux sync.RWMutex
}

func NewDatabaseMessages() *DatabaseMessages {
	return &DatabaseMessages{db: make(map[string][]*DatabaseEntry)}
}

func convertRumorToEntry(rumor *message.RumorMessage) *DatabaseEntry {
	return &DatabaseEntry{Rumor: rumor, TimeNano: time.Now().UnixNano()}
}

func (d *DatabaseMessages) InsertMessage(identifier string, msg *message.RumorMessage) error {
	d.mux.Lock()
	defer d.mux.Unlock()

	dbEntry := convertRumorToEntry(msg)

	list, ok := d.db[identifier]
	if !ok {
		d.db[identifier] = []*DatabaseEntry{dbEntry}
	} else {
		if int(msg.ID) == len(list)+1 {
			d.db[identifier] = append(list, dbEntry)
		} else {
			return errors.New("Already in database")
		}
	}

	return nil
}

func (d *DatabaseMessages) PrintDb() {
	d.mux.RLock()
	defer d.mux.RUnlock()

	for key, val := range d.db {
		println("Identifier: " + key + " " + strconv.Itoa(len(val)))
	}
}

func Map(vs []*DatabaseEntry, f func(*DatabaseEntry) *message.RumorMessage) []*message.RumorMessage {
	vsm := make([]*message.RumorMessage, len(vs))
	for i, v := range vs {
		vsm[i] = f(v)
	}
	return vsm
}
func Filter(vs []*DatabaseEntry, f func(*DatabaseEntry) bool) []*DatabaseEntry {
	vsf := make([]*DatabaseEntry, 0)
	for _, v := range vs {
		if f(v) {
			vsf = append(vsf, v)
		}
	}
	return vsf
}
func (d *DatabaseMessages) GetMessages() []*message.RumorMessage {

	list := make([]*DatabaseEntry, 0)
	d.mux.RLock()
	defer d.mux.RUnlock()
	for _, value := range d.db {
		list = append(list, value...)
	}
	filtered := Filter(list, func(entry *DatabaseEntry) bool {
		if entry.Rumor.Text != "" && entry.TimeNano > StartTime {
			return true
		}
		return false
	})
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].TimeNano < filtered[j].TimeNano
	})
	if len(filtered) != 0 {
		StartTime = filtered[len(filtered)-1].TimeNano
	}
	ret := Map(filtered, func(entry *DatabaseEntry) *message.RumorMessage {
		return entry.Rumor
	})
	return ret
}

func (d *DatabaseMessages) GetVectorClock() []message.PeerStatus {
	d.mux.RLock()
	defer d.mux.RUnlock()

	want := []message.PeerStatus{}
	for key, val := range d.db {
		want = append(want, message.PeerStatus{Identifier: key, NexId: uint32(len(val) + 1)})
	}
	return want
}

func (d *DatabaseMessages) CompareVectors(vectorClock []message.PeerStatus) (*message.RumorMessage, []message.PeerStatus) {
	// the rumor message to send win
	myVectorClock := d.GetVectorClock()
	for _, myPeerStatusMsg := range myVectorClock {
		flag := false
		for _, peerStatusMsg := range vectorClock {
			if peerStatusMsg.Identifier == myPeerStatusMsg.Identifier {
				flag = true
				// check if the clock is the lower
				if peerStatusMsg.NexId < myPeerStatusMsg.NexId {
					return d.db[myPeerStatusMsg.Identifier][peerStatusMsg.NexId-1].Rumor, nil
				}
			}
		}
		// the vectorClock doesnt have this entry, send the first
		if !flag {
			return d.db[myPeerStatusMsg.Identifier][0].Rumor, nil
		}
	}
	// we dont have to send rumor messages, check if I need something
	for _, peerStatusMsg := range vectorClock {
		flag := false
		for _, myPeerStatusMsg := range myVectorClock {
			if peerStatusMsg.Identifier == myPeerStatusMsg.Identifier {
				flag = true
				// check if the clock is the higher
				if peerStatusMsg.NexId > myPeerStatusMsg.NexId {
					return nil, myVectorClock
				}
			}
		}
		if !flag {
			return nil, myVectorClock
		}
	}
	return nil, nil
}
