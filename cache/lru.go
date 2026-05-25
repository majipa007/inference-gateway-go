package cache

import (
	"sync"
	"time"
)

// Entry is a single cached response with TTL support.
type Entry struct {
	Value     []byte
	ExpiresAt time.Time
}

// IsExpired returns true if the entry has passed its TTL.
func (e *Entry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// LRU is a thread-safe least-recently-used cache with TTL.
type LRU struct {
	capacity int
	ttl      time.Duration
	items    map[string]*listNode
	order    *doublyLinked
	mu       sync.RWMutex
}

type listNode struct {
	key     string
	value   []byte
	expires time.Time
	prev    *listNode
	next    *listNode
}

func (n *listNode) IsExpired() bool {
	return time.Now().After(n.expires)
}

type doublyLinked struct {
	head *listNode
	tail *listNode
	size int
}

type Option func(*LRU)

// WithCapacity sets the max number of entries.
func WithCapacity(cap int) Option {
	return func(l *LRU) { l.capacity = cap }
}

// WithTTL sets the time-to-live for each entry.
func WithTTL(ttl time.Duration) Option {
	return func(l *LRU) { l.ttl = ttl }
}

// New creates a new LRU cache with the given options.
func New(opts ...Option) *LRU {
	l := &LRU{
		capacity: 1000,
		ttl:      5 * time.Minute,
	}
	for _, opt := range opts {
		opt(l)
	}
	l.order = &doublyLinked{}
	l.order.init()
	l.items = make(map[string]*listNode)
	return l
}

func (l *doublyLinked) init() {
	sentinel := &listNode{}
	sentinel.prev = sentinel
	sentinel.next = sentinel
	l.head = sentinel
	l.tail = sentinel
}

func (l *doublyLinked) addFront(n *listNode) {
	n.next = l.head.next
	n.prev = l.head
	l.head.next.prev = n
	l.head.next = n
	l.size++
}

func (l *doublyLinked) remove(n *listNode) {
	n.prev.next = n.next
	n.next.prev = n.prev
	l.size--
}

func (l *doublyLinked) moveToFront(n *listNode) {
	l.remove(n)
	l.addFront(n)
}

func (l *doublyLinked) removeLast() (*listNode, bool) {
	if l.size == 0 {
		return nil, false
	}
	n := l.tail.prev
	l.remove(n)
	return n, true
}

// Get retrieves a cached value. Returns nil if not found or expired.
func (c *LRU) Get(key string) []byte {
	c.mu.RLock()
	node, ok := c.items[key]
	if !ok {
		c.mu.RUnlock()
		return nil
	}
	if node.IsExpired() {
		c.mu.RUnlock()
		c.mu.Lock()
		delete(c.items, key)
		c.order.remove(node)
		c.mu.Unlock()
		return nil
	}
	c.order.moveToFront(node)
	val := node.value
	c.mu.RUnlock()
	return val
}

// Set adds a value to the cache with default TTL.
func (c *LRU) Set(key string, value []byte) {
	c.SetWithTTL(key, value, c.ttl)
}

// SetWithTTL adds a value with a specific TTL.
func (c *LRU) SetWithTTL(key string, value []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing
	if node, ok := c.items[key]; ok {
		node.value = value
		node.expires = time.Now().Add(ttl)
		c.order.moveToFront(node)
		return
	}

	// Evict if at capacity
	for c.order.size >= c.capacity {
		if toEvict, ok := c.order.removeLast(); ok {
			delete(c.items, toEvict.key)
		} else {
			break
		}
	}

	// Create new node
	node := &listNode{
		key:     key,
		value:   value,
		expires: time.Now().Add(ttl),
	}
	c.items[key] = node
	c.order.addFront(node)
}

// Delete removes an entry from the cache.
func (c *LRU) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, ok := c.items[key]; ok {
		c.order.remove(node)
		delete(c.items, key)
	}
}

// Size returns the number of non-expired entries.
func (c *LRU) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.order.size
}

// Capacity returns the cache capacity.
func (c *LRU) Capacity() int {
	return c.capacity
}

// TTL returns the default TTL.
func (c *LRU) TTL() time.Duration {
	return c.ttl
}
