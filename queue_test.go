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
	"math"
	"runtime"
	"sync"
	"testing"
	"time"

	queue "gitee.com/ivfzhou/safe-queue"
)

func TestPutGet(t *testing.T) {
	q := queue.New[int](1 << 3)
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
	if err != queue.ErrQueueIsFull {
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
	if err != queue.ErrQueueIsEmpty {
		t.Fatal("err != ErrQueueIsEmpty")
	}
}

func TestEnough(t *testing.T) {
	q := queue.New[int](8)
	size, left := q.PutEnough(1, 2, 3, 4, 5, 6, 7, 8)
	if size != 8 {
		t.Fatal("size != 8")
	}
	if left != 0 {
		t.Fatal("left != 0")
	}
	size, left = q.PutEnough(9)
	vals, size, used := q.GetEnough(8)
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
	vals, size, used = q.GetEnough(1)

	size, left = q.PutEnough(1, 2, 3, 4, 5, 6, 7, 8, 9)
	if size != 8 {
		t.Fatal("size != 8")
	}
	if left != 0 {
		t.Fatal("left != 0")
	}
	vals, size, used = q.GetEnough(9)
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

	size, left = q.PutEnough(1, 2, 3, 4, 5)
	if size != 5 {
		t.Fatal("size != 5")
	}
	if left != 3 {
		t.Fatal("left != 3")
	}
	vals, size, used = q.GetEnough(4)
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
	q := queue.New[int](8)
	for i := 0; i < 8; i++ {
		left := q.MustPut(i)
		if left != uint32(7-i) {
			t.Fatal("left != 7-i")
		}
	}
	for i := 0; i < 8; i++ {
		val, used := q.MustGet()
		if used != uint32(7-i) {
			t.Fatal("used != 7-i")
		}
		if val != i {
			t.Fatal("val != i")
		}
	}
}

func TestConcurrent(t *testing.T) {
	const capacity = 1 << 8
	q := queue.New[int](capacity)
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
			if preVal > val {
				t.Errorf("preVal%d > val%d", preVal, val)
			}
		}()
	}
	wg.Wait()
	if q.Len() != 0 {
		t.Fatal("Len != 0")
	}
}

func TestUint32Overflow(t *testing.T) {
	capacity := uint32(1 << 8)
	q := queue.New[uint32](capacity)
	ticker := time.NewTicker(time.Second * 3)
	go func() {
		for range ticker.C {
			// t.Log(q)
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
	// t.Log(q)
}
