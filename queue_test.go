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
	"math/rand"
	"slices"
	"sync"
	"testing"

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
	{
		q := queue.New[int](capacity)
		ch := make(chan int, 100)
		putFinished := false
		wg1 := sync.WaitGroup{}
		wg1.Add(3)
		go func() {
			defer wg1.Done()
			for i := range size {
				_ = q.MustPut(i + 1)
			}
		}()
		go func() {
			defer wg1.Done()
			for i := range size {
				_ = q.MustPut(i + size + 1)
			}
		}()
		go func() {
			defer wg1.Done()
			for i := range size {
				_ = q.MustPut(i + 2*size + 1)
			}
		}()
		go func() {
			wg1.Wait()
			putFinished = true
		}()

		wg2 := sync.WaitGroup{}
		wg2.Add(3)
		go func() {
			defer wg2.Done()
			for !putFinished {
				value, _ := q.MustGet()
				ch <- value
			}
		}()
		go func() {
			defer wg2.Done()
			for !putFinished {
				value, _ := q.MustGet()
				ch <- value
			}
		}()
		go func() {
			defer wg2.Done()
			for !putFinished {
				value, _ := q.MustGet()
				ch <- value
			}
		}()
		go func() {
			wg2.Wait()
			close(ch)
		}()

		var result []int
		for i := range ch {
			result = append(result, i)
		}
		slices.Sort(result)
		for i := 0; i < len(result)-1; i++ {
			if result[i] >= result[i+1] {
				t.Error("failed test", result[i], result[i+1])
			}
		}
	}
}

func TestOverflow(t *testing.T) {
	q := queue.New[int](1000)
	maximum := uint64(math.MaxUint32) + 10
	count := uint64(0)
	for count < maximum {
		value := rand.Intn(math.MaxInt)
		_, err := q.Put(value)
		if err != nil {
			t.Fatal("put error", err)
		}
		getValue, _, err := q.Get()
		if err != nil {
			t.Fatal("get error", err)
		}
		if value != getValue {
			t.Error("value is unexpected")
		}
		count++
	}
}
