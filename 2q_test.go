// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lru

import (
	"testing"
	"time"
)

func Benchmark2Q_Rand(b *testing.B) {
	l, err := New2Q[int64, int64](8192)
	if err != nil {
		b.Fatalf("err: %v", err)
	}

	trace := make([]int64, b.N*2)
	for i := 0; i < b.N*2; i++ {
		trace[i] = getRand(b) % 32768
	}

	b.ResetTimer()

	var hit, miss int
	for i := 0; i < 2*b.N; i++ {
		if i%2 == 0 {
			l.Add(trace[i], trace[i])
		} else {
			if _, ok := l.Get(trace[i]); ok {
				hit++
			} else {
				miss++
			}
		}
	}
	b.Logf("hit: %d miss: %d ratio: %f", hit, miss, float64(hit)/float64(hit+miss))
}

func Benchmark2Q_Freq(b *testing.B) {
	l, err := New2Q[int64, int64](8192)
	if err != nil {
		b.Fatalf("err: %v", err)
	}

	trace := make([]int64, b.N*2)
	for i := 0; i < b.N*2; i++ {
		if i%2 == 0 {
			trace[i] = getRand(b) % 16384
		} else {
			trace[i] = getRand(b) % 32768
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		l.Add(trace[i], trace[i])
	}
	var hit, miss int
	for i := 0; i < b.N; i++ {
		if _, ok := l.Get(trace[i]); ok {
			hit++
		} else {
			miss++
		}
	}
	b.Logf("hit: %d miss: %d ratio: %f", hit, miss, float64(hit)/float64(hit+miss))
}

func Test2Q_RandomOps(t *testing.T) {
	size := 128
	l, err := New2Q[int64, int64](128)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	n := 200000
	for i := 0; i < n; i++ {
		key := getRand(t) % 512
		r := getRand(t)
		switch r % 3 {
		case 0:
			l.Add(key, key)
		case 1:
			l.Get(key)
		case 2:
			l.Remove(key)
		}

		if l.recent.Len()+l.frequent.Len() > size {
			t.Fatalf("bad: recent: %d freq: %d",
				l.recent.Len(), l.frequent.Len())
		}
	}
}

func Test2Q_Get_RecentToFrequent(t *testing.T) {
	l, err := New2Q[int, int](128)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Touch all the entries, should be in t1
	for i := 0; i < 128; i++ {
		l.Add(i, i)
	}
	if n := l.recent.Len(); n != 128 {
		t.Fatalf("bad: %d", n)
	}
	if n := l.frequent.Len(); n != 0 {
		t.Fatalf("bad: %d", n)
	}

	// Get should upgrade to t2
	for i := 0; i < 128; i++ {
		if _, ok := l.Get(i); !ok {
			t.Fatalf("missing: %d", i)
		}
	}
	if n := l.recent.Len(); n != 0 {
		t.Fatalf("bad: %d", n)
	}
	if n := l.frequent.Len(); n != 128 {
		t.Fatalf("bad: %d", n)
	}

	// Get be from t2
	for i := 0; i < 128; i++ {
		if _, ok := l.Get(i); !ok {
			t.Fatalf("missing: %d", i)
		}
	}
	if n := l.recent.Len(); n != 0 {
		t.Fatalf("bad: %d", n)
	}
	if n := l.frequent.Len(); n != 128 {
		t.Fatalf("bad: %d", n)
	}
}

func Test2Q_Add_RecentToFrequent(t *testing.T) {
	l, err := New2Q[int, int](128)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Add initially to recent
	l.Add(1, 1)
	if n := l.recent.Len(); n != 1 {
		t.Fatalf("bad: %d", n)
	}
	if n := l.frequent.Len(); n != 0 {
		t.Fatalf("bad: %d", n)
	}

	// Add should upgrade to frequent
	l.Add(1, 1)
	if n := l.recent.Len(); n != 0 {
		t.Fatalf("bad: %d", n)
	}
	if n := l.frequent.Len(); n != 1 {
		t.Fatalf("bad: %d", n)
	}

	// Add should remain in frequent
	l.Add(1, 1)
	if n := l.recent.Len(); n != 0 {
		t.Fatalf("bad: %d", n)
	}
	if n := l.frequent.Len(); n != 1 {
		t.Fatalf("bad: %d", n)
	}
}

func Test2Q_Add_RecentEvict(t *testing.T) {
	l, err := New2Q[int, int](4)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Add 1,2,3,4,5 -> Evict 1
	l.Add(1, 1)
	l.Add(2, 2)
	l.Add(3, 3)
	l.Add(4, 4)
	l.Add(5, 5)
	if n := l.recent.Len(); n != 4 {
		t.Fatalf("bad: %d", n)
	}
	if n := l.recentEvict.Len(); n != 1 {
		t.Fatalf("bad: %d", n)
	}
	if n := l.frequent.Len(); n != 0 {
		t.Fatalf("bad: %d", n)
	}

	// Pull in the recently evicted
	l.Add(1, 1)
	if n := l.recent.Len(); n != 3 {
		t.Fatalf("bad: %d", n)
	}
	if n := l.recentEvict.Len(); n != 1 {
		t.Fatalf("bad: %d", n)
	}
	if n := l.frequent.Len(); n != 1 {
		t.Fatalf("bad: %d", n)
	}

	// Add 6, should cause another recent evict
	l.Add(6, 6)
	if n := l.recent.Len(); n != 3 {
		t.Fatalf("bad: %d", n)
	}
	if n := l.recentEvict.Len(); n != 2 {
		t.Fatalf("bad: %d", n)
	}
	if n := l.frequent.Len(); n != 1 {
		t.Fatalf("bad: %d", n)
	}
}

func Test2Q(t *testing.T) {
	l, err := New2Q[int, int](128)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	for i := 0; i < 256; i++ {
		l.Add(i, i)
	}
	if l.Len() != 128 {
		t.Fatalf("bad len: %v", l.Len())
	}

	for i, k := range l.Keys() {
		if v, ok := l.Get(k); !ok || v != k || v != i+128 {
			t.Fatalf("bad key: %v", k)
		}
	}
	for i, v := range l.Values() {
		if v != i+128 {
			t.Fatalf("bad key: %v", v)
		}
	}
	for i := 0; i < 128; i++ {
		if _, ok := l.Get(i); ok {
			t.Fatalf("should be evicted")
		}
	}
	for i := 128; i < 256; i++ {
		if _, ok := l.Get(i); !ok {
			t.Fatalf("should not be evicted")
		}
	}
	for i := 128; i < 192; i++ {
		l.Remove(i)
		if _, ok := l.Get(i); ok {
			t.Fatalf("should be deleted")
		}
	}

	l.Purge()
	if l.Len() != 0 {
		t.Fatalf("bad len: %v", l.Len())
	}
	if _, ok := l.Get(200); ok {
		t.Fatalf("should contain nothing")
	}
}

// Test that Contains doesn't update recent-ness
func Test2Q_Contains(t *testing.T) {
	l, err := New2Q[int, int](2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	l.Add(1, 1)
	l.Add(2, 2)
	if !l.Contains(1) {
		t.Errorf("1 should be contained")
	}

	l.Add(3, 3)
	if l.Contains(1) {
		t.Errorf("Contains should not have updated recent-ness of 1")
	}
}

// Test that Peek doesn't update recent-ness
func Test2Q_Peek(t *testing.T) {
	l, err := New2Q[int, int](2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	l.Add(1, 1)
	l.Add(2, 2)
	if v, ok := l.Peek(1); !ok || v != 1 {
		t.Errorf("1 should be set to 1: %v, %v", v, ok)
	}

	l.Add(3, 3)
	if l.Contains(1) {
		t.Errorf("should not have updated recent-ness of 1")
	}
}

func Test2Q_TTL(t *testing.T) {
	l, err := New2QParamsWithEvictTTL[int, int](16, Default2QRecentRatio, Default2QGhostEntries, nil, time.Millisecond*50)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	l.Add(1, 1)

	if l.Len() != 1 {
		t.Errorf("Cache Len() should be 1")
	}

	if l.ItemCount() != 1 {
		t.Errorf("ItemCount() should be 1, since element 1 should not have already expired")
	}

	time.Sleep(time.Millisecond * 50)

	if l.Len() != 1 {
		t.Errorf("Cache Len() should be 1, since 1 is expired but not removed from the cache")
	}

	if l.ItemCount() != 0 {
		t.Errorf("ItemCount() should be 0, since element 1 should have already expired")
	}

	if l.Len() != 0 {
		t.Errorf("Cache Len() should be 0, since item should have been removed")
	}
}
