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
	size         int
	evictList    *lruList[K, V]
	items        map[K]*entry[K, V]
	onEvict      EvictCallback[K, V]
	itemTTL      time.Duration
	itemExpiries map[K]time.Time
}

// NewLRU constructs an LRU of the given size
func NewLRU[K comparable, V any](size int, onEvict EvictCallback[K, V]) (*LRU[K, V], error) {
	return NewLRUWithEvictTTL[K, V](size, onEvict, 0)
}

func NewLRUWithEvictTTL[K comparable, V any](size int, onEvict EvictCallback[K, V], itemTTL time.Duration) (*LRU[K, V], error) {
	if size <= 0 {
		return nil, errors.New("must provide a positive size")
	}

	c := &LRU[K, V]{
		size:         size,
		evictList:    newList[K, V](),
		items:        make(map[K]*entry[K, V]),
		onEvict:      onEvict,
		itemTTL:      itemTTL,
		itemExpiries: make(map[K]time.Time),
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
	return c.AddWithExp(key, value, time.Time{})
}

// Add adds a value to the cache allows for specific time to expire value.
// If provided time IsZero() the caches own TTL will be used (if available).
// Returns true if an eviction occurred.
func (c *LRU[K, V]) AddWithExp(key K, value V, expiry time.Time) (evicted bool) {
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
	if !expiry.IsZero() {
		c.itemExpiries[key] = expiry
	} else if c.itemTTL > 0 {
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
	if ent, ok := c.items[key]; ok && !c.KeyHasExpired(key) {
		if !c.KeyHasExpired(key) {
			c.evictList.moveToFront(ent)
			return ent.value, true
		}
		c.removeElement(ent)
	}
	return
}

// Contains checks if a key is in the cache, without updating the recent-ness
// or deleting it for being stale.
func (c *LRU[K, V]) Contains(key K) (ok bool) {
	if ent, ok := c.items[key]; ok {
		if !c.KeyHasExpired(key) {
			return true
		}
		c.removeElement(ent)
	}

	return
}

// Peek returns the key value (or undefined if not found) without updating
// the "recently used"-ness of the key.
func (c *LRU[K, V]) Peek(key K) (value V, ok bool) {
	var ent *entry[K, V]
	if ent, ok = c.items[key]; ok {
		if !c.KeyHasExpired(key) {
			return ent.value, true
		}
		c.removeElement(ent)
	}
	return
}

// Remove removes the provided key from the cache, returning if the
// key was contained.
func (c *LRU[K, V]) Remove(key K) (present bool) {
	if ent, ok := c.items[key]; ok {
		defer c.removeElement(ent)
		if !c.KeyHasExpired(key) {
			return true
		}
	}
	return
}

// RemoveOldest removes the oldest item from the cache.
func (c *LRU[K, V]) RemoveOldest() (key K, value V, ok bool) {
	if ent, ok := c.getOldest(false); ok {
		c.removeElement(ent)
		return ent.key, ent.value, true
	}

	return
}

// GetOldest returns the oldest entry
func (c *LRU[K, V]) GetOldest() (key K, value V, ok bool) {
	if ent, ok := c.getOldest(false); ok {
		return ent.key, ent.value, true
	}

	return
}

// Keys returns a slice of the keys in the cache, from oldest to newest.
func (c *LRU[K, V]) Keys() []K {
	keys := make([]K, c.evictList.length())
	i := 0
	for ent := c.evictList.back(); ent != nil; ent = ent.prevEntry() {
		if !c.KeyHasExpired(ent.key) {
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
		if !c.KeyHasExpired(ent.key) {
			values[i] = ent.value
			i++
		}
	}
	return values[:i]
}

// Len returns the physical number of items in the cache.
// This may include items that are inaccessible due to having expired.
func (c *LRU[K, V]) Len() int {
	return len(c.Keys())
}

// Len returns the number of actual items in the cache.
func (c *LRU[K, V]) ItemCount() int {
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
	if ent, ok := c.getOldest(true); ok {
		c.removeElement(ent)
	}
}

func (c *LRU[K, V]) getOldest(includeExpired bool) (oldest *entry[K, V], ok bool) {
	var next *entry[K, V]

	if c.itemTTL > 0 && includeExpired {
		if ent, ok := c.findExpired(); ok {
			return ent, true
		}
	}

	for ent := c.evictList.back(); ent != nil; {
		if !c.KeyHasExpired(ent.key) {
			return ent, true
		}

		next = ent.prev
		c.removeElement(ent)
		ent = next
	}

	return
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
func (c *LRU[K, V]) KeyHasExpired(key K) (expired bool) {
	expiry, ok := c.itemExpiries[key]
	return ok && expiry.Before(time.Now())
}

// Returns the expiry for a given key.
// If key is not found or does not expire a new instance of time.Time will be returned.
// This will not return for a key that already has expired.
func (c *LRU[K, V]) ExpiryForKey(key K) (expiry time.Time) {
	return c.itemExpiries[key]
}

// Finds the first entry that has expired.
func (c *LRU[K, V]) findExpired() (entry *entry[K, V], ok bool) {
	for ent := c.evictList.back(); ent != nil; ent = ent.prevEntry() {
		if c.KeyHasExpired(ent.key) {
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
		if c.KeyHasExpired(ent.key) {
			c.removeElement(ent)
			evicted++
		}
		ent = next
	}

	return
}

// Change the expiry for an item in the cache.
// The expiry of already expired items cannot be changed.
func (c *LRU[K, V]) ChangeExpiry(key K, expiry time.Time) (ok bool) {
	if _, ok := c.Peek(key); ok {
		c.itemExpiries[key] = expiry
		return true
	}

	return
}

func MoveItem[K comparable, V any](key K, dest, src LRUCache[K, V]) (value V, moved bool) {
	if val, ok := src.Peek(key); ok {
		if !src.KeyHasExpired(key) {
			src.Remove(key)
			dest.AddWithExp(key, val, src.ExpiryForKey(key))
			return val, true
		}
	}

	return
}
