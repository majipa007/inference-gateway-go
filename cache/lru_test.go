package cache

import (
	"testing"
	"time"
)

func TestLRUCache_SetAndGet(t *testing.T) {
	c := New(WithCapacity(10))

	c.Set("key1", []byte("value1"))
	got := c.Get("key1")
	if string(got) != "value1" {
		t.Errorf("expected 'value1', got %q", string(got))
	}
}

func TestLRUCache_CacheMiss(t *testing.T) {
	c := New(WithCapacity(10))

	got := c.Get("nonexistent")
	if got != nil {
		t.Errorf("expected nil for miss, got %q", string(got))
	}
}

func TestLRUCache_Expiration(t *testing.T) {
	c := New(WithTTL(50 * time.Millisecond))

	c.Set("key1", []byte("value1"))
	got := c.Get("key1")
	if string(got) != "value1" {
		t.Errorf("expected 'value1', got %q", string(got))
	}

	time.Sleep(60 * time.Millisecond)

	got = c.Get("key1")
	if got != nil {
		t.Errorf("expected nil after expiration, got %q", string(got))
	}
}

func TestLRUCache_Overwrite(t *testing.T) {
	c := New(WithCapacity(10))

	c.Set("key1", []byte("value1"))
	c.Set("key1", []byte("value2"))

	got := c.Get("key1")
	if string(got) != "value2" {
		t.Errorf("expected 'value2', got %q", string(got))
	}
}

func TestLRUCache_CapacityEviction(t *testing.T) {
	c := New(WithCapacity(3))

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))
	c.Set("d", []byte("4"))

	// 'a' should have been evicted (LRU)
	if got := c.Get("a"); got != nil {
		t.Errorf("expected 'a' to be evicted, got %q", string(got))
	}

	// 'b' should still be present
	if got := c.Get("b"); string(got) != "2" {
		t.Errorf("expected 'b'=2, got %q", string(got))
	}

	// 'd' should be present (most recent)
	if got := c.Get("d"); string(got) != "4" {
		t.Errorf("expected 'd'=4, got %q", string(got))
	}
}

func TestLRUCache_MRUBehavior(t *testing.T) {
	c := New(WithCapacity(3))

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))

	// Access 'a' to make it MRU
	_ = c.Get("a")

	// Add 'd' - should evict 'b' (LRU), not 'a' (MRU)
	c.Set("d", []byte("4"))

	if got := c.Get("a"); string(got) != "1" {
		t.Errorf("expected 'a' to still exist (was accessed), got %q", string(got))
	}

	if got := c.Get("b"); got != nil {
		t.Errorf("expected 'b' to be evicted, got %q", string(got))
	}
}

func TestLRUCache_ConcurrentAccess(t *testing.T) {
	c := New(WithCapacity(1000))

	done := make(chan struct{}, 2)
	go func() {
		for i := 0; i < 10000; i++ {
			key := string(rune('a' + i%26))
			c.Set(key, []byte(string(rune('0'+i%10))))
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 10000; i++ {
			_ = c.Get(string(rune('a' + i%26)))
		}
		done <- struct{}{}
	}()

	<-done
	<-done
}

func TestLRUCache_SetWithTTL(t *testing.T) {
	c := New(WithCapacity(10), WithTTL(1*time.Hour))

	// This key should expire quickly despite the 1-hour default
	c.SetWithTTL("key1", []byte("value1"), 50*time.Millisecond)

	got := c.Get("key1")
	if string(got) != "value1" {
		t.Errorf("expected 'value1', got %q", string(got))
	}

	time.Sleep(60 * time.Millisecond)

	got = c.Get("key1")
	if got != nil {
		t.Errorf("expected nil after 50ms TTL, got %q", string(got))
	}
}

func TestLRUCache_Delete(t *testing.T) {
	c := New(WithCapacity(10))

	c.Set("key1", []byte("value1"))
	c.Delete("key1")

	got := c.Get("key1")
	if got != nil {
		t.Errorf("expected nil after delete, got %q", string(got))
	}
}

func TestLRUCache_Size(t *testing.T) {
	c := New(WithCapacity(10))

	if c.Size() != 0 {
		t.Errorf("expected size 0, got %d", c.Size())
	}

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3"))

	if c.Size() != 3 {
		t.Errorf("expected size 3, got %d", c.Size())
	}
}

func TestLRUCache_CapacityAndTTL(t *testing.T) {
	c := New(WithCapacity(3), WithTTL(5*time.Minute))

	if c.Capacity() != 3 {
		t.Errorf("expected capacity 3, got %d", c.Capacity())
	}

	if c.TTL() != 5*time.Minute {
		t.Errorf("expected TTL 5m, got %v", c.TTL())
	}
}

func TestLRUCache_CustomCapacity(t *testing.T) {
	c := New(WithCapacity(1))

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))

	if got := c.Get("a"); got != nil {
		t.Errorf("expected 'a' evicted with capacity 1, got %q", string(got))
	}

	if got := c.Get("b"); string(got) != "2" {
		t.Errorf("expected 'b'=2, got %q", string(got))
	}
}
