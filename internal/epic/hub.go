package epic

import "sync"

const maxEpicEventBuffer = 5000

type EpicEventHub struct {
	mu    sync.RWMutex
	epics map[string]*epicConnection
}

type epicConnection struct {
	mu        sync.Mutex
	buffer    []EpicEvent
	listeners map[uint64]chan EpicEvent
	nextKey   uint64
	closed    bool
}

func NewEpicEventHub() *EpicEventHub {
	return &EpicEventHub{
		epics: make(map[string]*epicConnection),
	}
}

func (h *EpicEventHub) Publish(ev EpicEvent) {
	h.mu.RLock()
	conn, exists := h.epics[ev.EpicID]
	h.mu.RUnlock()

	if !exists {
		conn = h.getOrCreate(ev.EpicID)
	}

	conn.mu.Lock()
	if len(conn.buffer) < maxEpicEventBuffer {
		conn.buffer = append(conn.buffer, ev)
	}
	listeners := make([]chan EpicEvent, 0, len(conn.listeners))
	for _, ch := range conn.listeners {
		listeners = append(listeners, ch)
	}
	conn.mu.Unlock()

	for _, ch := range listeners {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (h *EpicEventHub) getOrCreate(epicID string) *epicConnection {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn, exists := h.epics[epicID]; exists {
		return conn
	}
	conn := &epicConnection{
		listeners: make(map[uint64]chan EpicEvent),
		buffer:    make([]EpicEvent, 0, maxEpicEventBuffer),
		nextKey:   1,
	}
	h.epics[epicID] = conn
	return conn
}

func (h *EpicEventHub) Subscribe(epicID string) (<-chan EpicEvent, func()) {
	conn := h.getOrCreate(epicID)

	conn.mu.Lock()
	defer conn.mu.Unlock()

	key := conn.nextKey
	conn.nextKey++
	ch := make(chan EpicEvent, 500)
	conn.listeners[key] = ch

	unsub := func() {
		conn.mu.Lock()
		_, exists := conn.listeners[key]
		delete(conn.listeners, key)
		conn.mu.Unlock()
		if exists {
			close(ch)
		}
	}
	return ch, unsub
}

func (h *EpicEventHub) GetBuffer(epicID string) []EpicEvent {
	h.mu.RLock()
	conn, exists := h.epics[epicID]
	h.mu.RUnlock()

	if !exists {
		return nil
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	buf := make([]EpicEvent, len(conn.buffer))
	copy(buf, conn.buffer)
	return buf
}

func (h *EpicEventHub) Close(epicID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conn, exists := h.epics[epicID]
	if !exists {
		return
	}

	conn.mu.Lock()
	for key, ch := range conn.listeners {
		delete(conn.listeners, key)
		close(ch)
	}
	conn.closed = true
	conn.mu.Unlock()

	delete(h.epics, epicID)
}
