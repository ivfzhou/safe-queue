package safe_queue_test

import (
	"math"
	"runtime"
	"sync"
	"testing"
	"time"

	"gitee.com/ivfzhou/safe-queue"
)

func TestPutGet(t *testing.T) {
	q := safe_queue.New(8)
	if q == nil {
		t.Fatal("q == nil")
	}
	if q.Cap() != 8 {
		t.Fatal("Cap != 8")
	}
	left, err := q.Put(1)
	if err != nil {
		t.Fatal(err)
	}
	if left != 7 {
		t.Fatal("remain != 7")
	}
	if q.Len() != 1 {
		t.Fatal("Len != 1")
	}
	for i := 0; i < 7; i++ {
		left, err = q.Put(i + 2)
		if err != nil {
			t.Fatal(err)
		}
		if left != uint32(6-i) {
			t.Fatal("left != 7-i")
		}
	}
	if q.Len() != 8 {
		t.Fatal("Len != 8")
	}
	left, err = q.Put(1)
	if err != safe_queue.ErrQueueIsFull {
		t.Fatal("err != ErrQueueIsFull")
	}
	val, used, err := q.Get()
	if err != nil {
		t.Fatal(err)
	}
	if val != 1 {
		t.Fatal("res != 1")
	}
	if used != 7 {
		t.Fatal("remain != 7")
	}
	if q.Len() != 7 {
		t.Fatal("Len != 7")
	}
	for i := 0; i < 7; i++ {
		val, used, err = q.Get()
		if err != nil {
			t.Fatal(err)
		}
		if val != i+2 {
			t.Fatal("val != i+2")
		}
		if used != uint32(6-i) {
			t.Fatal("used != 6-i")
		}
	}
	if q.Len() != 0 {
		t.Fatal("Len != 0")
	}
	val, used, err = q.Get()
	if err != safe_queue.ErrQueueIsEmpty {
		t.Fatal("err != ErrQueueIsEmpty")
	}
}

func TestEnough(t *testing.T) {
	q := safe_queue.New(8)
	size, left, err := q.PutEnough(1, 2, 3, 4, 5, 6, 7, 8)
	if err != nil {
		t.Fatal(err)
	}
	if size != 8 {
		t.Fatal("size != 8")
	}
	if left != 0 {
		t.Fatal("left != 0")
	}
	size, left, err = q.PutEnough(9)
	if err != safe_queue.ErrQueueIsFull {
		t.Fatal("err != ErrQueueIsFull")
	}
	vals, size, used, err := q.GetEnough(8)
	if err != nil {
		t.Fatal(err)
	}
	if size != 8 {
		t.Fatal("size != 8")
	}
	if used != 0 {
		t.Fatal("used != 0")
	}
	for i, v := range vals {
		if v != i+1 {
			t.Fatal("v != i+1")
		}
	}
	vals, size, used, err = q.GetEnough(1)
	if err != safe_queue.ErrQueueIsEmpty {
		t.Fatal("err != ErrQueueIsEmpty")
	}

	size, left, err = q.PutEnough(1, 2, 3, 4, 5, 6, 7, 8, 9)
	if err != nil {
		t.Fatal(err)
	}
	if size != 8 {
		t.Fatal("size != 8")
	}
	if left != 0 {
		t.Fatal("left != 0")
	}
	vals, size, used, err = q.GetEnough(9)
	if err != nil {
		t.Fatal(err)
	}
	if size != 8 {
		t.Fatal("size != 8")
	}
	if used != 0 {
		t.Fatal("used != 0")
	}
	for i, v := range vals {
		if v != i+1 {
			t.Fatal("v != i+1")
		}
	}

	size, left, err = q.PutEnough(1, 2, 3, 4, 5)
	if err != nil {
		t.Fatal(err)
	}
	if size != 5 {
		t.Fatal("size != 5")
	}
	if left != 3 {
		t.Fatal("left != 3")
	}
	vals, size, used, err = q.GetEnough(4)
	if err != nil {
		t.Fatal(err)
	}
	if size != 4 {
		t.Fatal("size != 4")
	}
	if used != 1 {
		t.Fatal("used != 1")
	}
	for i, v := range vals {
		if v != i+1 {
			t.Fatal("v != i+1")
		}
	}
}

func TestMust(t *testing.T) {
	q := safe_queue.New(8)
	for i := 0; i < 8; i++ {
		left := q.PutMust(i)
		if left != uint32(7-i) {
			t.Fatal("left != 7-i")
		}
	}
	for i := 0; i < 8; i++ {
		val, used := q.GetMust()
		if used != uint32(7-i) {
			t.Fatal("used != 7-i")
		}
		if val != i {
			t.Fatal("val != i")
		}
	}
}

func TestUint32Overflow(t *testing.T) {
	capacity := uint32(1 << 8)
	q := safe_queue.New(capacity)
	ticker := time.NewTicker(time.Second * 3)
	go func() {
		for range ticker.C {
			t.Log(q)
		}
	}()
	times := uint32(1.1 * float64(math.MaxUint32/capacity))
	for i := uint32(0); i < times; i++ {
		for j := uint32(0); j < capacity; j++ {
			left, err := q.Put(j + 1)
			if err != nil {
				t.Fatal(err)
			}
			if left != capacity-j-1 {
				t.Fatal("left != capacity-j-1")
			}
		}
		for j := uint32(0); j < capacity; j++ {
			val, used, err := q.Get()
			if err != nil {
				t.Fatal(err)
			}
			if used != capacity-j-1 {
				t.Fatal("used != capacity-j-1")
			}
			if val != j+1 {
				t.Fatal("val != j+1")
			}
		}
	}
	ticker.Stop()
	if q.Len() != 0 {
		t.Fatal("Len != 0")
	}
	t.Log(q)
}

func TestConcurrent(t *testing.T) {
	const capacity = 1 << 8
	q := safe_queue.New(capacity)
	wg := sync.WaitGroup{}

	for i := 0; i < capacity; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
		L1:
			_, err := q.Put(i)
			if err != nil {
				runtime.Gosched()
				time.Sleep(time.Millisecond * 50)
				goto L1
			}
		}(i)
	}

	for i := 0; i < capacity; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			preVal := 0
		L1:
			val, _, err := q.Get()
			if err != nil {
				runtime.Gosched()
				time.Sleep(time.Millisecond * 50)
				goto L1
			}
			v := val.(int)
			if preVal > v {
				t.Errorf("preVal%d > val%d", preVal, v)
			}
		}()
	}
	wg.Wait()
	if q.Len() != 0 {
		t.Fatal("Len != 0")
	}
}
