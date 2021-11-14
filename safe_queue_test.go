package safe_queue_test

import (
	"math"
	"testing"

	"gitee.com/ivfzhou/safe-queue"
)

var val = new(int)

func TestCommon(t *testing.T) {
	q := safe_queue.New(8)
	*val = 1

	if q == nil {
		t.Fatal("q == nil")
	}
	if q.Cap() != 8 {
		t.Fatal("Cap != 8")
	}

	remain, err := q.Put(val)
	if err != nil {
		t.Fatal(err)
	}
	if remain != 7 {
		t.Fatal("remain != 7")
	}

	if q.Len() != 1 {
		t.Fatal("Len != 1")
	}
	for i := 0; i < 7; i++ {
		_, _ = q.Put(val)
	}
	if q.Len() != 8 {
		t.Fatal("Len != 8")
	}

	remain, err = q.Put(val)
	if err != safe_queue.ErrQueueIsFull {
		t.Fatal("err != ErrQueueIsFull")
	}

	res, remain, err := q.Get()
	if err != nil {
		t.Fatal(err)
	}
	if res != val {
		t.Fatal("res != val")
	}
	if remain != 7 {
		t.Fatal("remain != 7")
	}

	if q.Len() != 7 {
		t.Fatal("Len != 7")
	}
	for i := 0; i < 7; i++ {
		_, _, _ = q.Get()
	}
	if q.Len() != 0 {
		t.Fatal("Len != 0")
	}
}

func TestExtreme(t *testing.T) {
	q := safe_queue.New(8)
	times := math.MaxUint32 / 8
	for i := 0; i < times; i++ {
		for j := 0; j < 8; j++ {
			_, _ = q.Put(val)
		}
		for j := 0; j < 8; j++ {
			_, _, _ = q.Get()
		}
	}

	for i := 0; i < 8; i++ {
		remain, err := q.Put(i)
		if err != nil {
			t.Fatal(err)
		}
		if remain != uint32(8-i-1) {
			t.Fatal("remain != uint32(8-i-1)")
		}
	}
	for i := 0; i < 8; i++ {
		res, remain, err := q.Get()
		if err != nil {
			t.Fatal(err)
		}
		if res != i {
			t.Fatal("res != i")
		}
		if remain != uint32(8-i-1) {
			t.Fatal("remain != uint32(8-i-1)")
		}
	}
}
