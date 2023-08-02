// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package simplelru

import (
	"errors"
	"time"
)

// EvictCallback is used to get a callback when a cache entry is evicted
type EvictCallback[K comparable, V any] func(key K, value V)

// LRU implements a non-thread safe fixed size LRU cache
type LRU[K comparable, V any] struct {
	size             int
	evictList        *lruList[K, V]
	items            map[K]*entry[K, V]
	onEvict          EvictCallback[K, V]
	itemTTL          time.Duration
	itemExpiries     map[K]time.Time
	expiryBasedEvict bool
}

// NewLRU constructs an LRU of the given size
func NewLRU[K comparable, V any](size int, onEvict EvictCallback[K, V]) (*LRU[K, V], error) {
	return NewLRUWithEvictTTL[K, V](size, onEvict, 0, false)
}

func NewLRUWithEvictTTL[K comparable, V any](size int, onEvict EvictCallback[K, V], itemTTL time.Duration, expiryBasedEvict bool) (*LRU[K, V], error) {
	if size <= 0 {
		return nil, errors.New("must provide a positive size")
	}

	c := &LRU[K, V]{
		size:             size,
		evictList:        newList[K, V](),
		items:            make(map[K]*entry[K, V]),
		onEvict:          onEvict,
		itemTTL:          itemTTL,
		itemExpiries:     make(map[K]time.Time),
		expiryBasedEvict: expiryBasedEvict,
	}
	return c, nil
}

// Purge is used to completely clear the cache.
func (c *LRU[K, V]) Purge() {
	for k, v := range c.items {
		if c.onEvict != nil {
			c.onEvict(k, v.value)
		}
		delete(c.items, k)
		delete(c.itemExpiries, k)
	}
	c.evictList.init()
}

// Add adds a value to the cache.  Returns true if an eviction occurred.
func (c *LRU[K, V]) Add(key K, value V) (evicted bool) {
	// Check for existing item
	if ent, ok := c.items[key]; ok {
		c.evictList.moveToFront(ent)
		if c.onEvict != nil {
			c.onEvict(key, ent.value)
		}
		ent.value = value
		return false
	}

	// Add new item
	ent := c.evictList.pushFront(key, value)
	c.items[key] = ent
	if c.itemTTL > 0 {
		c.itemExpiries[key] = time.Now().Add(c.itemTTL)
	}

	evict := c.evictList.length() > c.size
	// Verify size not exceeded
	if evict {
		c.removeOldest()
	}
	return evict
}

// Get looks up a key's value from the cache.
func (c *LRU[K, V]) Get(key K) (value V, ok bool) {
	if ent, ok := c.items[key]; ok && !c.keyHasExpired(key) {
		c.evictList.moveToFront(ent)
		return ent.value, true
	}
	return
}

// Contains checks if a key is in the cache, without updating the recent-ness
// or deleting it for being stale.
func (c *LRU[K, V]) Contains(key K) (ok bool) {
	if _, ok = c.items[key]; ok && !c.keyHasExpired(key) {
		return true
	}

	return
}

// Peek returns the key value (or undefined if not found) without updating
// the "recently used"-ness of the key.
func (c *LRU[K, V]) Peek(key K) (value V, ok bool) {
	var ent *entry[K, V]
	if ent, ok = c.items[key]; ok && !c.keyHasExpired(key) {
		return ent.value, true
	}
	return
}

// Remove removes the provided key from the cache, returning if the
// key was contained.
func (c *LRU[K, V]) Remove(key K) (present bool) {
	if ent, ok := c.items[key]; ok {
		if !c.keyHasExpired(key) {
			c.removeElement(ent)
			return true
		} else {
			c.removeElement(ent)
			return
		}
	}
	return
}

// RemoveOldest removes the oldest item from the cache.
func (c *LRU[K, V]) RemoveOldest() (key K, value V, ok bool) {
	if ent := c.evictList.back(); ent != nil {
		if !c.keyHasExpired(key) {
			c.removeElement(ent)
			return ent.key, ent.value, true
		} else {
			c.removeElement(ent)
			return
		}
	}
	return
}

// GetOldest returns the oldest entry
func (c *LRU[K, V]) GetOldest() (key K, value V, ok bool) {
	if ent := c.evictList.back(); ent != nil && !c.keyHasExpired(key) {
		return ent.key, ent.value, true
	}
	return
}

// Keys returns a slice of the keys in the cache, from oldest to newest.
func (c *LRU[K, V]) Keys() []K {
	keys := make([]K, c.evictList.length())
	i := 0
	for ent := c.evictList.back(); ent != nil; ent = ent.prevEntry() {
		if !c.keyHasExpired(ent.key) {
			keys[i] = ent.key
			i++
		}
	}
	return keys[:i]
}

// Values returns a slice of the values in the cache, from oldest to newest.
func (c *LRU[K, V]) Values() []V {
	values := make([]V, len(c.items))
	i := 0
	for ent := c.evictList.back(); ent != nil; ent = ent.prevEntry() {
		if !c.keyHasExpired(ent.key) {
			values[i] = ent.value
			i++
		}
	}
	return values[:i]
}

// Len returns the number of items in the cache.
func (c *LRU[K, V]) Len() int {
	return len(c.Keys())
}

// Len returns the number of actual items in the cache.
// This may include items that are inaccessible due to expiry.
func (c *LRU[K, V]) ActualLen() int {
	return len(c.Keys())
}

// Resize changes the cache size.
func (c *LRU[K, V]) Resize(size int) (evicted int) {
	diff := c.Len() - size
	if diff < 0 {
		diff = 0
	}
	for i := 0; i < diff; i++ {
		c.removeOldest()
	}
	c.size = size
	return diff
}

// removeOldest removes the oldest item from the cache.
func (c *LRU[K, V]) removeOldest() {
	if c.expiryBasedEvict {
		if ent, ok := c.findExpired(); ok {
			c.removeElement(ent)
			return
		}
	}

	if ent := c.evictList.back(); ent != nil {
		c.removeElement(ent)
	}
}

// removeElement is used to remove a given list element from the cache
func (c *LRU[K, V]) removeElement(e *entry[K, V]) {
	c.evictList.remove(e)
	delete(c.items, e.key)
	delete(c.itemExpiries, e.key)
	if c.onEvict != nil {
		c.onEvict(e.key, e.value)
	}
}

// Checks if a given key has expired.
func (c *LRU[K, V]) keyHasExpired(key K) (expired bool) {
	expiry, ok := c.itemExpiries[key]
	return ok && expiry.Before(time.Now())
}

// Finds the first entry that has expired.
func (c *LRU[K, V]) findExpired() (entry *entry[K, V], ok bool) {
	for ent := c.evictList.back(); ent != nil; ent = ent.prevEntry() {
		if c.keyHasExpired(ent.key) {
			return ent, true
		}
	}

	return
}

// Removes all expired entries from the cache.
func (c *LRU[K, V]) RemoveExpired() (evicted int) {
	var next *entry[K, V]

	for ent := c.evictList.back(); ent != nil; {
		next = ent.prevEntry()
		if c.keyHasExpired(ent.key) {
			c.removeElement(ent)
			evicted++
		}
		ent = next
	}

	return
}
