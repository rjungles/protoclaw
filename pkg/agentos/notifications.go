package agentos

import (
	"sync"
	"time"
)

type Notification struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	FromActor string                 `json:"from_actor"`
	ToActor   string                 `json:"to_actor"`
	Type      string                 `json:"type"`
	Title     string                 `json:"title"`
	Body      string                 `json:"body"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Read      bool                   `json:"read"`
}

type NotificationHandler func(n Notification)

type NotificationBus struct {
	mu       sync.RWMutex
	handlers map[string][]NotificationHandler
	log      []Notification
	maxLog   int
	seq      int64
}

func NewNotificationBus(maxLog int) *NotificationBus {
	if maxLog <= 0 {
		maxLog = 1000
	}
	return &NotificationBus{
		handlers: make(map[string][]NotificationHandler),
		log:      make([]Notification, 0, 64),
		maxLog:   maxLog,
	}
}

func (b *NotificationBus) Subscribe(actorID string, handler NotificationHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[actorID] = append(b.handlers[actorID], handler)
}

func (b *NotificationBus) Notify(n Notification) {
	b.mu.Lock()
	b.seq++
	if n.ID == "" {
		n.ID = time.Now().Format("20060102150405") + "-" + itoa(b.seq)
	}
	if n.Timestamp.IsZero() {
		n.Timestamp = time.Now()
	}
	b.log = append(b.log, n)
	if len(b.log) > b.maxLog {
		b.log = b.log[len(b.log)-b.maxLog:]
	}
	handlers := make([]NotificationHandler, len(b.handlers[n.ToActor]))
	copy(handlers, b.handlers[n.ToActor])
	allHandlers := make([]NotificationHandler, len(b.handlers["*"]))
	copy(allHandlers, b.handlers["*"])
	b.mu.Unlock()

	for _, h := range handlers {
		h(n)
	}
	for _, h := range allHandlers {
		h(n)
	}
}

func (b *NotificationBus) GetUnread(actorID string) []Notification {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var result []Notification
	for _, n := range b.log {
		if n.ToActor == actorID && !n.Read {
			result = append(result, n)
		}
	}
	return result
}

func (b *NotificationBus) MarkRead(notificationID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.log {
		if b.log[i].ID == notificationID {
			b.log[i].Read = true
			return
		}
	}
}

func (b *NotificationBus) GetLog() []Notification {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cp := make([]Notification, len(b.log))
	copy(cp, b.log)
	return cp
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
