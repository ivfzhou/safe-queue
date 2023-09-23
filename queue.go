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

package safe_queue

import (
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/cpu"
)

const cacheLinePadSize = unsafe.Sizeof(cpu.CacheLinePad{})

var (
	// ErrQueueIsFull 表明队列已满。
	ErrQueueIsFull = errors.New("队列已满")
	// ErrQueueIsEmpty 表明队列为空。
	ErrQueueIsEmpty = errors.New("队列为空")
)

type (
	// Queue 队列结构体。使用 New 创建变量。
	Queue[E any] struct {
		capacity, mask uint32
		_              [cacheLinePadSize - 8]byte
		head           uint32
		_              [cacheLinePadSize - 4]byte
		tail           uint32
		_              [cacheLinePadSize - 4]byte
		elements       []element[E]
		_              [cacheLinePadSize - unsafe.Sizeof([]element[E]{})]byte
	}
	element[E any] struct {
		getSeq, putSeq uint32
		value          E
		_              [cacheLinePadSize - 8 - 16]byte
	}
)

// New 创建队列。capacity 队列长度。值将调整为以2为底的幂数，最小值为2，最大值为2^31。
func New[E any](capacity uint32) *Queue[E] {
	capacity--
	capacity |= capacity >> 1
	capacity |= capacity >> 2
	capacity |= capacity >> 4
	capacity |= capacity >> 8
	capacity |= capacity >> 16
	capacity++

	if capacity < 2 {
		capacity = 2
	}

	instance := &Queue[E]{
		capacity: capacity,
		elements: make([]element[E], capacity),
		mask:     capacity - 1,
	}
	for i := range instance.elements {
		instance.elements[i].putSeq = uint32(i)
		instance.elements[i].getSeq = uint32(i)
	}
	instance.elements[0].putSeq = capacity
	instance.elements[0].getSeq = capacity

	return instance
}

// Put 向队列尾部填充数据。返回剩余可填充数据个数。若队列已满返回错误 ErrQueueIsFull。
func (q *Queue[E]) Put(value E) (uint32, error) {
	position, _, left, err := q.acquirePut(1)
	if err != nil {
		return 0, err
	}
	q.put(position, value)
	return left, nil
}

// Get 取出队列头部数据。返回队列数据，队列剩余可取个数。当无数据可取时返回错误 ErrQueueIsEmpty。
func (q *Queue[E]) Get() (E, uint32, error) {
	var val E
	position, _, used, err := q.acquireGet(1)
	if err != nil {
		return val, 0, err
	}
	val = q.get(position)
	return val, used, nil
}

// PutEnough 向队列填充多个数据。返回实际填充数据个数，剩余可填充数据个数。
func (q *Queue[E]) PutEnough(values ...E) (uint32, uint32) {
	size := uint32(len(values))
	if size == 0 {
		return 0, q.Cap() - q.Len()
	}
	position, actualSize, left, err := q.acquirePut(size)
	if err != nil {
		return 0, 0
	}

	for i, j := position, 0; i < position+actualSize; i, j = i+1, j+1 {
		q.put(i, values[j])
	}

	return actualSize, left
}

// GetEnough 从队列取出多个数据。返回队列队列数据，实际取出数据个数，剩余可取数据个数。
func (q *Queue[E]) GetEnough(size uint32) ([]E, uint32, uint32) {
	if size == 0 {
		return []E{}, 0, q.Cap() - q.Len()
	}

	position, actualSize, used, err := q.acquireGet(size)
	if err != nil {
		return nil, 0, 0
	}

	res := make([]E, 0, actualSize)
	for i := position; i < position+actualSize; i++ {
		res = append(res, q.get(i))
	}

	return res, actualSize, used
}

// MustPut 向队列中塞数据，若队列已满将等待。返回剩余可填充数据个数。
func (q *Queue[E]) MustPut(value E) uint32 {
	var (
		position, left uint32
		err            error
	)
	for {
		position, _, left, err = q.acquirePut(1)
		if err == nil {
			break
		}
	}
	q.put(position, value)
	return left
}

// MustGet 取出队列头部数据。，若队列无数据将等待。返回队列数据，队列剩余可取个数。
func (q *Queue[E]) MustGet() (E, uint32) {
	var (
		position, used uint32
		err            error
	)
	for {
		position, _, used, err = q.acquireGet(1)
		if err == nil {
			break
		}
	}
	val := q.get(position)
	return val, used
}

// Cap 返回队列长度。
func (q *Queue[E]) Cap() uint32 {
	return q.capacity
}

// Len 返回队列数据个数。
func (q *Queue[E]) Len() uint32 {
	return atomic.LoadUint32(&q.tail) - atomic.LoadUint32(&q.head)
}

// IsEmpty 判断队列是否有数据。
func (q *Queue[E]) IsEmpty() bool {
	return atomic.LoadUint32(&q.head) == atomic.LoadUint32(&q.tail)
}

// IsFull 判断队列是否已满。
func (q *Queue[E]) IsFull() bool {
	return atomic.LoadUint32(&q.tail)-atomic.LoadUint32(&q.head) == q.capacity
}

// String 返回队列字符串表示形式值。
func (q *Queue[E]) String() string {
	return fmt.Sprintf(`Queue: Head:%d Tail:%d Len:%d Cap:%d`,
		atomic.LoadUint32(&q.head), atomic.LoadUint32(&q.tail), q.Len(), q.Cap())
}

func (q *Queue[E]) usedSize(tail, head uint32) uint32 {
	return tail - head
}

func (q *Queue[E]) leftSize(tail, head uint32) uint32 {
	return q.capacity - q.usedSize(tail, head)
}

func (q *Queue[E]) acquirePut(size uint32) (uint32, uint32, uint32, error) {
	var head, tail, left uint32

	for {
		head = atomic.LoadUint32(&q.head)
		tail = atomic.LoadUint32(&q.tail)
		left = q.leftSize(tail, head)
		if left == 0 {
			return 0, 0, 0, ErrQueueIsFull
		}
		if size > left {
			size = left
		}
		if atomic.CompareAndSwapUint32(&q.tail, tail, tail+size) {
			return tail + 1, size, left - size, nil
		}
		runtime.Gosched()
	}
}

func (q *Queue[E]) acquireGet(size uint32) (uint32, uint32, uint32, error) {
	var head, tail, used uint32

	for {
		head = atomic.LoadUint32(&q.head)
		tail = atomic.LoadUint32(&q.tail)
		used = q.usedSize(tail, head)
		if used == 0 {
			return 0, 0, 0, ErrQueueIsEmpty
		}
		if size > used {
			size = used
		}
		if atomic.CompareAndSwapUint32(&q.head, head, head+size) {
			return head + 1, size, used - size, nil
		}
		runtime.Gosched()
	}
}

func (q *Queue[E]) get(position uint32) E {
	elem := &q.elements[position&q.mask]
	for !(position == atomic.LoadUint32(&elem.getSeq) && position == atomic.LoadUint32(&elem.putSeq)-q.capacity) {
		runtime.Gosched()
	}
	val := elem.value
	var empty E
	elem.value = empty
	_ = atomic.AddUint32(&elem.getSeq, q.capacity)
	return val
}

func (q *Queue[E]) put(position uint32, value E) {
	elem := &q.elements[position&q.mask]
	for !(position == atomic.LoadUint32(&elem.getSeq) && position == atomic.LoadUint32(&elem.putSeq)) {
		runtime.Gosched()
	}
	elem.value = value
	_ = atomic.AddUint32(&elem.putSeq, q.capacity)
}
