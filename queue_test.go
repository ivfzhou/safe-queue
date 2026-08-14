/*
 * Copyright (c) 2023 ivfzhou
 * safe-queue is licensed under Mulan PSL v2.
 * You can use this software according to the terms and conditions of the Mulan PSL v2.
 * You may obtain a copy of Mulan PSL v2 at:
 *          http://license.coscl.org.cn/MulanPSL2
 * THIS SOFTWARE IS PROVIDED ON AN "AS IS" BASIS, WITHOUT WARRANTIES OF ANY KIND,
 * EITHER EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO NON-INFRINGEMENT,
 * MERCHANTABILITY OR FIT FOR A PARTICULAR PURPOSE.
 * See the Mulan PSL v2 for more details.
 */

package safe_queue_test

import (
	"errors"
	"math"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	queue "gitee.com/ivfzhou/safe-queue"
)

func TestPutAndGet(t *testing.T) {
	q := queue.New[int](1 << 3)
	if q == nil {
		t.Fatal("queue is nil")
	}
	if q.Cap() != 8 {
		t.Fatal("capacity should be 8")
	}
	leftSize, err := q.Put(1)
	if err != nil {
		t.Fatal("put error", err)
	}
	if leftSize != 7 {
		t.Fatal("left size should be 7")
	}
	if q.Len() != 1 {
		t.Fatal("length should be 1")
	}
	for i := range 7 {
		leftSize, err = q.Put(i + 2)
		if err != nil {
			t.Fatal("put error", err)
		}
		if leftSize != uint32(6-i) {
			t.Fatal("left size should be", 6-i)
		}
	}
	if q.Len() != 8 {
		t.Fatal("length should be 8")
	}
	leftSize, err = q.Put(1)
	if !errors.Is(err, queue.ErrQueueIsFull) {
		t.Fatal("err is not ErrQueueIsFull")
	}
	value, usedSize, err := q.Get()
	if err != nil {
		t.Fatal("get error", err)
	}
	if value != 1 {
		t.Fatal("value should be 1")
	}
	if usedSize != 7 {
		t.Fatal("used size should be 7")
	}
	if q.Len() != 7 {
		t.Fatal("length should be 7")
	}
	for i := range 7 {
		value, usedSize, err = q.Get()
		if err != nil {
			t.Fatal("get error", err)
		}
		if value != i+2 {
			t.Fatal("value should be", i+2)
		}
		if usedSize != uint32(6-i) {
			t.Fatal("used size should be", 6-i)
		}
	}
	if q.Len() != 0 {
		t.Fatal("length should be 0")
	}
	value, usedSize, err = q.Get()
	if !errors.Is(err, queue.ErrQueueIsEmpty) {
		t.Fatal("err is not ErrQueueIsEmpty")
	}
}

func TestPutAndGetEnough(t *testing.T) {
	q := queue.New[int](8)

	actualInsertedSize, leftSize := q.PutEnough(1, 2, 3, 4, 5, 6, 7, 8)
	if actualInsertedSize != 8 {
		t.Fatal("actualInsertedSize should be 8")
	}
	if leftSize != 0 {
		t.Fatal("leftSize should be 0")
	}

	elements, actualGetSize, leftSize := q.GetEnough(8)
	if actualGetSize != 8 {
		t.Fatal("actualGetSize should be 8")
	}
	if leftSize != 0 {
		t.Fatal("leftSize should 0")
	}
	for i, v := range elements {
		if v != i+1 {
			t.Fatal("element should be", i+1)
		}
	}

	actualInsertedSize, leftSize = q.PutEnough(1, 2, 3, 4, 5, 6, 7, 8, 9)
	if actualInsertedSize != 8 {
		t.Fatal("actualInsertedSize should be 8")
	}
	if leftSize != 0 {
		t.Fatal("leftSize should be 0")
	}

	elements, actualGetSize, leftSize = q.GetEnough(9)
	if actualGetSize != 8 {
		t.Fatal("actualGetSize should be 8")
	}
	if leftSize != 0 {
		t.Fatal("leftSize should be 0")
	}
	for i, v := range elements {
		if v != i+1 {
			t.Fatal("element should be", i+1)
		}
	}

	actualInsertedSize, leftSize = q.PutEnough(1, 2, 3, 4, 5)
	if actualInsertedSize != 5 {
		t.Fatal("actualInsertedSize should be 5")
	}
	if leftSize != 3 {
		t.Fatal("leftSize should be 3")
	}
	elements, actualGetSize, leftSize = q.GetEnough(4)
	if actualGetSize != 4 {
		t.Fatal("actualInsertedSize should be 4")
	}
	if leftSize != 1 {
		t.Fatal("leftSize should be 1")
	}
	for i, v := range elements {
		if v != i+1 {
			t.Fatal("element should be", i+1)
		}
	}
}

func TestMustPutAndGet(t *testing.T) {
	q := queue.New[int](8)

	for i := range 8 {
		leftSize := q.MustPut(i + 1)
		if leftSize != uint32(7-i) {
			t.Fatal("leftSize should be", 7-i)
		}
	}

	for i := range 8 {
		value, leftSize := q.MustGet()
		if leftSize != uint32(7-i) {
			t.Fatal("leftSize should be", 7-i)
		}
		if value != i+1 {
			t.Fatal("value should be", i+1)
		}
	}
}

func TestConcurrent(t *testing.T) {
	const capacity = 1000
	const size = 1000
	const total = 3 * size

	q := queue.New[int](capacity)
	ch := make(chan int, 100)

	// 3 个生产者，各写入 size 个元素，共 total 个。
	var producers sync.WaitGroup
	producers.Add(3)
	for g := range 3 {
		go func(base int) {
			defer producers.Done()
			for i := range size {
				q.MustPut(base + i + 1)
			}
		}(g * size)
	}

	// 3 个消费者，各读取 size 个元素，与生产者总数精确匹配，避免竞态与数据残留。
	var consumers sync.WaitGroup
	consumers.Add(3)
	for range 3 {
		go func() {
			defer consumers.Done()
			for range size {
				value, _ := q.MustGet()
				ch <- value
			}
		}()
	}

	go func() {
		producers.Wait()
		consumers.Wait()
		close(ch)
	}()

	result := make([]int, 0, total)
	for v := range ch {
		result = append(result, v)
	}
	if len(result) != total {
		t.Fatalf("expected %d elements, got %d", total, len(result))
	}
	slices.Sort(result)
	for i := 0; i < len(result)-1; i++ {
		if result[i] >= result[i+1] {
			t.Error("failed test", result[i], result[i+1])
		}
	}
}

func TestOverflow(t *testing.T) {
	if raceEnabled {
		t.Skip("race detector makes the 2^32 overflow test too slow")
	}
	const capacity = 1000
	q := queue.New[int](capacity)
	maximum := uint64(math.MaxUint32) + 10
	ints := [capacity]int{}
	wg := sync.WaitGroup{}
	count := uint64(0)
	for range runtime.NumCPU() {
		wg.Go(func() {
			for range maximum / capacity / uint64(runtime.NumCPU()) {
				values := ints[:]
				actualInsertedSize, _ := q.PutEnough(values...)
				for actualInsertedSize < uint32(len(values)) {
					values = values[actualInsertedSize:]
					actualInsertedSize, _ = q.PutEnough(values...)
				}
				_, actualGetSize, _ := q.GetEnough(capacity)
				tmp := uint32(capacity)
				for actualGetSize < tmp {
					tmp = capacity - actualGetSize
					_, actualGetSize, _ = q.GetEnough(tmp)
				}
				atomic.AddUint64(&count, capacity)
			}
		})
	}
	wg.Wait()
	for count < maximum {
		_, _ = q.Put(1)
		_, _, _ = q.Get()
		count++
	}
	t.Log(q.String())
}

func TestCapacityAlignment(t *testing.T) {
	cases := []struct {
		requested uint32
		expected  uint32
	}{
		{0, 2},
		{1, 2},
		{2, 2},
		{3, 4},
		{5, 8},
		{256, 256},
		{1000, 1024},
	}
	for _, c := range cases {
		q := queue.New[int](c.requested)
		if q.Cap() != c.expected {
			t.Fatalf("New(%d).Cap() = %d, want %d", c.requested, q.Cap(), c.expected)
		}
	}
}

func TestEmptyAndFull(t *testing.T) {
	q := queue.New[int](2)
	if !q.IsEmpty() {
		t.Fatal("queue should be empty")
	}
	if q.IsFull() {
		t.Fatal("queue should not be full")
	}

	_, err := q.Put(1)
	if err != nil {
		t.Fatal(err)
	}
	if q.IsEmpty() {
		t.Fatal("queue should not be empty")
	}

	_, err = q.Put(2)
	if err != nil {
		t.Fatal(err)
	}
	if !q.IsFull() {
		t.Fatal("queue should be full")
	}

	if _, _, err := q.Get(); err != nil {
		t.Fatal(err)
	}
	if q.IsFull() {
		t.Fatal("queue should not be full after get")
	}
}

func TestString(t *testing.T) {
	q := queue.New[int](2)
	s := q.String()
	if s == "" {
		t.Fatal("String() should not be empty")
	}
}

func TestGenericTypes(t *testing.T) {
	q1 := queue.New[string](4)
	q1.MustPut("hello")
	if v, _ := q1.MustGet(); v != "hello" {
		t.Fatalf("got %q, want %q", v, "hello")
	}

	type payload struct {
		name string
		n    int
	}
	q2 := queue.New[*payload](4)
	p := &payload{name: "x", n: 1}
	q2.MustPut(p)
	if v, _ := q2.MustGet(); v != p {
		t.Fatal("pointer value mismatch")
	}
}

func TestMustPutBlocking(t *testing.T) {
	q := queue.New[int](2)
	if _, err := q.Put(1); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Put(2); err != nil {
		t.Fatal(err)
	}
	if !q.IsFull() {
		t.Fatal("queue should be full")
	}

	done := make(chan struct{})
	go func() {
		q.MustPut(3)
		close(done)
	}()

	// 给 goroutine 时间进入自旋等待，确认其处于阻塞状态。
	time.Sleep(10 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("MustPut should block when queue is full")
	default:
	}

	if _, _, err := q.Get(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("MustPut did not complete after space freed")
	}
	if q.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", q.Len())
	}
}

func TestMustGetBlocking(t *testing.T) {
	q := queue.New[int](2)
	if !q.IsEmpty() {
		t.Fatal("queue should be empty")
	}

	done := make(chan int)
	go func() {
		v, _ := q.MustGet()
		done <- v
	}()

	time.Sleep(10 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("MustGet should block when queue is empty")
	default:
	}

	q.MustPut(42)

	select {
	case v := <-done:
		if v != 42 {
			t.Fatalf("got %d, want 42", v)
		}
	case <-time.After(time.Second):
		t.Fatal("MustGet did not complete")
	}
}
