package safe_queue_test

import (
	"math"
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

func TestUint32Overflow(t *testing.T) {
	capacity := uint32(1 << 8)
	q := safe_queue.New(capacity)
	ticker := time.NewTicker(time.Second * 3)
	go func() {
		for range ticker.C {
			t.Log(q)
		}
	}()
	times := math.MaxUint32 / capacity
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
	ticker.Stop()
}

/*func TestMore(t *testing.T) {
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
	const capacity = 1<<16 - 1
	q := safe_queue.New(capacity)
	wg := sync.WaitGroup{}

	for i := 0; i < capacity*2; i++ {
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

	for i := 0; i < capacity*2; i++ {
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

	for i := 0; i < capacity*2; i += 4 {
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

	for i := 0; i < capacity*2; i += 4 {
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
	t.Log(q.Len())
}*/
