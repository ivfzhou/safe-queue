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
		capacity  uint64
		_         [cacheLinePadSize - 8]byte
		headIndex uint64
		_         [cacheLinePadSize - 4]byte
		tailIndex uint64
		_         [cacheLinePadSize - 4]byte
		elements  []element[E]
		_         [cacheLinePadSize - unsafe.Sizeof([]element[E]{})]byte
	}
	element[E any] struct {
		getSequence, putSequence uint64
		value                    E
		_                        [cacheLinePadSize - 8 - 16]byte
	}
)

// New 创建队列。capacity 队列长度。
func New[E any](capacity uint32) *Queue[E] {
	if capacity <= 0 {
		capacity = 1
	}

	instance := &Queue[E]{
		capacity: uint64(capacity),
		elements: make([]element[E], capacity),
	}
	for i := range instance.elements {
		instance.elements[i].putSequence = uint64(i)
		instance.elements[i].getSequence = uint64(i)
	}

	return instance
}

// Put 向队列尾部填充数据。返回剩余可填充数据个数。若队列已满返回错误 ErrQueueIsFull。
func (q *Queue[E]) Put(value E) (leftSize uint32, err error) {
	var leftSize64 uint64
	position, _, leftSize64, err := q.acquirePut(1)
	if err != nil {
		return 0, err
	}
	q.put(position, value)
	return uint32(leftSize64), nil
}

// Get 取出队列头部数据。返回队列数据，队列剩余可取个数。当无数据可取时返回错误 ErrQueueIsEmpty。
func (q *Queue[E]) Get() (value E, usedSize uint32, err error) {
	var usedSize64 uint64
	position, _, usedSize64, err := q.acquireGet(1)
	if err != nil {
		return value, 0, err
	}
	value = q.get(position)
	return value, uint32(usedSize64), nil
}

// PutEnough 向队列填充多个数据。返回实际填充数据个数，剩余可填充数据个数。
func (q *Queue[E]) PutEnough(values ...E) (actualInsertedSize uint32, leftSize uint32) {
	size := uint32(len(values))
	if size == 0 {
		return 0, q.Cap() - q.Len()
	}
	var actualInsertedSize64, leftSize64 uint64
	position, actualInsertedSize64, leftSize64, err := q.acquirePut(uint64(size))
	if err != nil {
		return 0, 0
	}

	for i, j := position, 0; i < position+actualInsertedSize64; i, j = i+1, j+1 {
		q.put(i, values[j])
	}

	return uint32(actualInsertedSize64), uint32(leftSize64)
}

// GetEnough 从队列取出多个数据。返回队列队列数据，实际取出数据个数，剩余可取数据个数。
func (q *Queue[E]) GetEnough(size uint32) (elements []E, actualGetSize uint32, leftSize uint32) {
	if size == 0 {
		return nil, 0, q.Cap() - q.Len()
	}

	var actualGetSize64, leftSize64 uint64
	position, actualGetSize64, leftSize64, err := q.acquireGet(uint64(size))
	if err != nil {
		return nil, 0, 0
	}

	elements = make([]E, 0, actualGetSize)
	for i := position; i < position+actualGetSize64; i++ {
		elements = append(elements, q.get(i))
	}

	return elements, uint32(actualGetSize64), uint32(leftSize64)
}

// MustPut 向队列中塞数据，若队列已满将等待。返回剩余可填充数据个数。
func (q *Queue[E]) MustPut(value E) (leftSize uint32) {
	var (
		position uint64
		err      error
	)
	leftSize64 := uint64(0)
	for {
		position, _, leftSize64, err = q.acquirePut(1)
		if err == nil {
			break
		}
	}
	q.put(position, value)
	return uint32(leftSize64)
}

// MustGet 取出队列头部数据。，若队列无数据将等待。返回队列数据，队列剩余可取个数。
func (q *Queue[E]) MustGet() (value E, leftSize uint32) {
	var (
		position uint64
		err      error
	)
	leftSize64 := uint64(0)
	for {
		position, _, leftSize64, err = q.acquireGet(1)
		if err == nil {
			break
		}
	}
	return q.get(position), uint32(leftSize64)
}

// Cap 返回队列长度。
func (q *Queue[E]) Cap() uint32 {
	return uint32(q.capacity)
}

// Len 返回队列数据个数。
func (q *Queue[E]) Len() uint32 {
	return uint32(atomic.LoadUint64(&q.tailIndex) - atomic.LoadUint64(&q.headIndex))
}

// IsEmpty 判断队列是否有数据。
func (q *Queue[E]) IsEmpty() bool {
	return atomic.LoadUint64(&q.headIndex) == atomic.LoadUint64(&q.tailIndex)
}

// IsFull 判断队列是否已满。
func (q *Queue[E]) IsFull() bool {
	return atomic.LoadUint64(&q.tailIndex)-atomic.LoadUint64(&q.headIndex) == q.capacity
}

// String 返回队列字符串表示形式值。
func (q *Queue[E]) String() string {
	return fmt.Sprintf(`Queue: Head:%d Tail:%d Len:%d Cap:%d`,
		atomic.LoadUint64(&q.headIndex), atomic.LoadUint64(&q.tailIndex), q.Len(), q.Cap())
}

// 返回已使用的位置数量。
func (q *Queue[E]) usedSize(tailIndex, headIndex uint64) uint64 {
	return tailIndex - headIndex
}

// 剩余可用的位置数量。
func (q *Queue[E]) leftSize(tailIndex, headIndex uint64) uint64 {
	return q.capacity - q.usedSize(tailIndex, headIndex)
}

// 循环和 CAS 方式获取可以插入队列的位置。
func (q *Queue[E]) acquirePut(wanSize uint64) (
	canInsertIndexStart uint64, canInsertSize uint64, insertedLeftSize uint64, err error) {

	var headIndex, tailIndex, leftSize uint64

	for {
		headIndex = atomic.LoadUint64(&q.headIndex)
		tailIndex = atomic.LoadUint64(&q.tailIndex)
		leftSize = q.leftSize(tailIndex, headIndex)
		if leftSize == 0 {
			return 0, 0, 0, ErrQueueIsFull
		}
		if wanSize > leftSize {
			wanSize = leftSize
		}

		if atomic.CompareAndSwapUint64(&q.tailIndex, tailIndex, tailIndex+wanSize) {
			return tailIndex, wanSize, leftSize - wanSize, nil
		}

		runtime.Gosched()
	}
}

// 循环和 CAS 方式获取可以取出元素的位置。
func (q *Queue[E]) acquireGet(wantSize uint64) (
	canTakeIndexStart uint64, canTakeSize uint64, leftSize uint64, err error) {

	var headIndex, tailIndex, usedIndex uint64

	for {
		headIndex = atomic.LoadUint64(&q.headIndex)
		tailIndex = atomic.LoadUint64(&q.tailIndex)
		usedIndex = q.usedSize(tailIndex, headIndex)
		if usedIndex == 0 {
			return 0, 0, 0, ErrQueueIsEmpty
		}
		if wantSize > usedIndex {
			wantSize = usedIndex
		}

		if atomic.CompareAndSwapUint64(&q.headIndex, headIndex, headIndex+wantSize) {
			return headIndex, wantSize, usedIndex - wantSize, nil
		}

		runtime.Gosched()
	}
}

// 获取队列元素。
func (q *Queue[E]) get(position uint64) E {
	elem := &q.elements[position%q.capacity]
	// 自旋等待：直到该位置的元素已经由生产者写入，且该位置对消费者可读。
	// 这确保了读取操作与并发写入不冲突，满足无锁并发安全。
	for !(position == atomic.LoadUint64(&elem.getSequence) &&
		position == atomic.LoadUint64(&elem.putSequence)-q.capacity) {
		runtime.Gosched()
	}

	value := elem.value
	var emptyValue E
	elem.value = emptyValue
	_ = atomic.AddUint64(&elem.getSequence, q.capacity)
	return value
}

// 放入队列元素。
func (q *Queue[E]) put(position uint64, value E) {
	elem := &q.elements[position%q.capacity]
	// 自旋等待：直到该位置可写（已被前一轮消费者取走）。
	// 通过序号比较保证写入不会覆盖尚未消费的数据。
	for !(position == atomic.LoadUint64(&elem.getSequence) && position == atomic.LoadUint64(&elem.putSequence)) {
		runtime.Gosched()
	}

	elem.value = value
	_ = atomic.AddUint64(&elem.putSequence, q.capacity)
}
