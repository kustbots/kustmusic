package cache

import (
	"testing"
	"time"
)

func TestCacheBasicOperations(t *testing.T) {
	c := NewCache[string](100 * time.Millisecond)
	defer c.Close()

	// Test Set and Get
	c.Set("key1", "val1")
	val, ok := c.Get("key1")
	if !ok || val != "val1" {
		t.Fatalf("expected val1, got %v (ok=%v)", val, ok)
	}

	// Test Size
	if c.Size() != 1 {
		t.Fatalf("expected size 1, got %d", c.Size())
	}

	// Test Delete
	c.Delete("key1")
	_, ok = c.Get("key1")
	if ok {
		t.Fatalf("expected key1 to be deleted")
	}

	// Test Clear
	c.Set("k1", "v1")
	c.Set("k2", "v2")
	if c.Size() != 2 {
		t.Fatalf("expected size 2, got %d", c.Size())
	}
	c.Clear()
	if c.Size() != 0 {
		t.Fatalf("expected size 0 after Clear, got %d", c.Size())
	}
}

func TestCacheExpiration(t *testing.T) {
	c := NewCache[int](20 * time.Millisecond)
	defer c.Close()

	c.Set("short", 42)
	c.SetWithTTL("long", 100, 200*time.Millisecond)

	time.Sleep(35 * time.Millisecond)

	// 'short' should be expired
	_, ok := c.Get("short")
	if ok {
		t.Fatalf("expected 'short' to be expired")
	}

	// 'long' should still exist
	val, ok := c.Get("long")
	if !ok || val != 100 {
		t.Fatalf("expected 'long' to be 100, got %v (ok=%v)", val, ok)
	}
}

func TestCacheEvictExpired(t *testing.T) {
	c := NewCache[string](10 * time.Millisecond)
	defer c.Close()

	c.Set("item1", "val1")
	c.Set("item2", "val2")

	time.Sleep(20 * time.Millisecond)

	// Trigger evictExpired manually
	c.evictExpired()

	if c.Size() != 0 {
		t.Fatalf("expected size 0 after evictExpired, got %d", c.Size())
	}
}

func TestJanitorRegistration(t *testing.T) {
	c := NewCache[string](50 * time.Millisecond)

	j := getJanitor()
	if !j.has(c) {
		t.Fatalf("expected cache to be registered with janitor")
	}

	c.Close()
	if j.has(c) {
		t.Fatalf("expected cache to be unregistered from janitor after Close()")
	}
}
