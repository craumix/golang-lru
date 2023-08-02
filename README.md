# golang-lru

[![GoDoc](https://pkg.go.dev/badge/github.com/craumix/golang-lru?status.svg)](https://pkg.go.dev/github.com/craumix/golang-lru?tab=doc)

This provides the `lru` package which implements a fixed-size thread safe LRU cache.

This repository is based on [golang-lru by hashicorp](https://github.com/hashicorp/golang-lru), which is based on the cache in [groupcache](https://github.com/golang/groupcache).

This repository aims to add an optional TTL for cache items, to the existing cache types, with minimal overhead.
The expiry of cached items is stored in a separate map from the items, and items are lazily deleted (but can also be deleted manually).

## Example

Using the LRU is very simple:

```go
package main

import (
 "fmt"
 "github.com/craumix/golang-lru"
)

func main() {
  l, _ := lru.NewWithEvictTTL[int, any](128, nil, time.Minute)
  for i := 0; i < 256; i++ {
      l.Add(i, nil)
  }
  if l.Len() != 128 {
      panic(fmt.Sprintf("bad len: %v", l.Len()))
  }
}
```
