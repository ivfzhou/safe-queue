package safe_queue_test

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

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

	t.Log(q)

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

	res, remain, err = q.Get()
	if err != safe_queue.ErrQueueIsEmpty {
		t.Fatal("err != ErrQueueIsEmpty")
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

func TestMore(t *testing.T) {
	q := safe_queue.New(8)
	leave, err := q.PutMore(context.Background(), 1, 2, 3, 4, 5, 6, 7, 8)
	if err != nil {
		t.Fatal(err)
	}
	if leave != 0 {
		t.Fatal("leave != 0")
	}
	values, leave, err := q.GetMore(context.Background(), 8)
	if err != nil {
		t.Fatal(err)
	}
	if leave != 0 {
		t.Fatal("leave != 0")
	}
	for i, value := range values {
		if i+1 != value.(int) {
			t.Fatal("i != value")
		}
	}

	leave, err = q.PutMore(context.Background(), 1, 2, 3, 4, 5, 6, 7, 8, 9)
	if err != safe_queue.ErrTooMoreValues {
		t.Fatal("err != ErrTooMoreValues")
	}
	values, leave, err = q.GetMore(context.Background(), 9)
	if err != safe_queue.ErrNotEnoughValues {
		t.Fatal("err != ErrNotEnoughValues")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	values, leave, err = q.GetMore(ctx, 8)
	if err != ctx.Err() {
		t.Fatal("err != ctx.Err()")
	}

	_, _ = q.PutMore(context.Background(), 1, 2, 3, 4, 5, 6)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	leave, err = q.PutMore(ctx, 7, 8, 9)
	if err != ctx.Err() {
		t.Fatal("err != ctx.Err()")
	}
}

func TestConcurrent(t *testing.T) {
	q := safe_queue.New(24)
	wg := sync.WaitGroup{}

	for i := 0; i < 48; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
		L1:
			_, err := q.Put(i)
			if err != nil {
				time.Sleep(time.Millisecond)
				goto L1
			}
		}(i)
	}

	for i := 0; i < 48; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
		L1:
			_, _, err := q.Get()
			if err != nil {
				time.Sleep(time.Millisecond)
				goto L1
			}
		}(i)
	}

	for i := 0; i < 48; i += 4 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
		L1:
			_, err := q.PutMore(context.Background(), i, i+1, i+2, i+3, i+4, i+5, i+6, i+7, i+8, i+9)
			if err != nil {
				time.Sleep(time.Millisecond)
				goto L1
			}
		}(i)
	}

	for i := 0; i < 48; i += 4 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
		L1:
			_, _, err := q.GetMore(context.Background(), 10)
			if err != nil {
				time.Sleep(time.Millisecond)
				goto L1
			}
		}(i)
	}

	wg.Wait()
	t.Log(q)
}
