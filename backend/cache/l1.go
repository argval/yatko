package cache

import (
	"container/list"
	"sync"
)

// l1MaxEntries caps the in-process cache so a burst of unique keys can't
// unbounded-grow backend memory. Hot /dl release keys dominate.
const l1MaxEntries = 512

// l1Cache is a small process-local LRU of marshaled cache entries. It sits in
// front of Redis so repeated hits (especially /dl) skip an Upstash round-trip.
type l1Cache struct {
	mu       sync.Mutex
	max      int
	items    map[string]*list.Element
	eviction *list.List // front = most recently used
}

type l1Node struct {
	key  string
	data []byte
}

func newL1(max int) *l1Cache {
	if max < 1 {
		max = l1MaxEntries
	}
	return &l1Cache{
		max:      max,
		items:    make(map[string]*list.Element, max),
		eviction: list.New(),
	}
}

func (l *l1Cache) get(key string) ([]byte, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	el, ok := l.items[key]
	if !ok {
		return nil, false
	}
	l.eviction.MoveToFront(el)
	// Copy so callers can't mutate the cached slice.
	src := el.Value.(*l1Node).data
	out := make([]byte, len(src))
	copy(out, src)
	return out, true
}

func (l *l1Cache) set(key string, data []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	stored := make([]byte, len(data))
	copy(stored, data)
	if el, ok := l.items[key]; ok {
		el.Value.(*l1Node).data = stored
		l.eviction.MoveToFront(el)
		return
	}
	for l.eviction.Len() >= l.max {
		oldest := l.eviction.Back()
		if oldest == nil {
			break
		}
		l.eviction.Remove(oldest)
		delete(l.items, oldest.Value.(*l1Node).key)
	}
	el := l.eviction.PushFront(&l1Node{key: key, data: stored})
	l.items[key] = el
}

func (l *l1Cache) delete(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	el, ok := l.items[key]
	if !ok {
		return
	}
	l.eviction.Remove(el)
	delete(l.items, key)
}
