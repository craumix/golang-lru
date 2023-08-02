// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package simplelru provides simple LRU implementation based on build-in container/list.
package simplelru

import "time"

// LRUCache is the interface for simple LRU cache.
type LRUCache[K comparable, V any] interface {
	// Adds a value to the cache, returns true if an eviction occurred and
	// updates the "recently used"-ness of the key.
	Add(key K, value V) bool

	// Add adds a value to the cache allows for specific time to expire value.
	// If provided time IsZero() the caches own TTL will be used (if available).
	// Returns true if an eviction occurred.
	AddWithExp(key K, value V, expiry time.Time) (evicted bool)

	// Returns key's value from the cache and
	// updates the "recently used"-ness of the key. #value, isFound
	Get(key K) (value V, ok bool)

	// Checks if a key exists in cache without updating the recent-ness.
	Contains(key K) (ok bool)

	// Returns key's value without updating the "recently used"-ness of the key.
	Peek(key K) (value V, ok bool)

	// Removes a key from the cache.
	Remove(key K) bool

	// Removes the oldest entry from cache.
	RemoveOldest() (K, V, bool)

	// Returns the oldest entry from the cache. #key, value, isFound
	GetOldest() (K, V, bool)

	// Returns a slice of the keys in the cache, from oldest to newest.
	Keys() []K

	// Values returns a slice of the values in the cache, from oldest to newest.
	Values() []V

	// Len returns the physical number of items in the cache.
	// This may include items that are inaccessible due to having expired.
	Len() int

	// Returns the number of accessible items in the cache.
	ItemCount() int

	// Clears all cache entries.
	Purge()

	// Resizes cache, returning number evicted
	Resize(int) int

	// Checks if a given key has expired.
	KeyHasExpired(key K) (expired bool)

	// Returns the expiry for a given key.
	// If key is not found or does not expire a new instance of time.Time will be returned.
	// This will not return for a key that already has expired.
	ExpiryForKey(key K) (expiry time.Time)

	// Removes all expired entries from the cache.
	RemoveExpired() (evicted int)
}
