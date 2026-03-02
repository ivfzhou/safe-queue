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
		headIndex      uint32
		_              [cacheLinePadSize - 4]byte
		tailIndex      uint32
		_              [cacheLinePadSize - 4]byte
		elements       []element[E]
		_              [cacheLinePadSize - unsafe.Sizeof([]element[E]{})]byte
	}
	element[E any] struct {
		getSequence, putSequence uint32
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
		capacity: capacity,
		elements: make([]element[E], capacity),
		mask:     capacity - 1,
	}
	for i := range instance.elements {
		instance.elements[i].putSequence = uint32(i)
		instance.elements[i].getSequence = uint32(i)
	}
	instance.elements[0].putSequence = capacity
	instance.elements[0].getSequence = capacity

	return instance
}

// Put 向队列尾部填充数据。返回剩余可填充数据个数。若队列已满返回错误 ErrQueueIsFull。
func (q *Queue[E]) Put(value E) (leftSize uint32, err error) {
	position, _, leftSize, err := q.acquirePut(1)
	if err != nil {
		return 0, err
	}
	q.put(position, value)
	return
}

// Get 取出队列头部数据。返回队列数据，队列剩余可取个数。当无数据可取时返回错误 ErrQueueIsEmpty。
func (q *Queue[E]) Get() (value E, usedSize uint32, err error) {
	position, _, usedSize, err := q.acquireGet(1)
	if err != nil {
		return value, 0, err
	}
	value = q.get(position)
	return
}

// PutEnough 向队列填充多个数据。返回实际填充数据个数，剩余可填充数据个数。
func (q *Queue[E]) PutEnough(values ...E) (actualInsertedSize uint32, leftSize uint32) {
	size := uint32(len(values))
	if size == 0 {
		return 0, q.Cap() - q.Len()
	}
	position, actualInsertedSize, leftSize, err := q.acquirePut(size)
	if err != nil {
		return 0, 0
	}

	for i, j := position, 0; i < position+actualInsertedSize; i, j = i+1, j+1 {
		q.put(i, values[j])
	}

	return
}

// GetEnough 从队列取出多个数据。返回队列队列数据，实际取出数据个数，剩余可取数据个数。
func (q *Queue[E]) GetEnough(size uint32) (elements []E, actualGetSize uint32, leftSize uint32) {
	if size == 0 {
		return nil, 0, q.Cap() - q.Len()
	}

	position, actualGetSize, leftSize, err := q.acquireGet(size)
	if err != nil {
		return nil, 0, 0
	}

	elements = make([]E, 0, actualGetSize)
	for i := position; i < position+actualGetSize; i++ {
		elements = append(elements, q.get(i))
	}

	return
}

// MustPut 向队列中塞数据，若队列已满将等待。返回剩余可填充数据个数。
func (q *Queue[E]) MustPut(value E) (leftSize uint32) {
	var (
		position uint32
		err      error
	)
	for {
		position, _, leftSize, err = q.acquirePut(1)
		if err == nil {
			break
		}
	}
	q.put(position, value)
	return
}

// MustGet 取出队列头部数据。，若队列无数据将等待。返回队列数据，队列剩余可取个数。
func (q *Queue[E]) MustGet() (value E, leftSize uint32) {
	var (
		position uint32
		err      error
	)
	for {
		position, _, leftSize, err = q.acquireGet(1)
		if err == nil {
			break
		}
	}
	return q.get(position), leftSize
}

// Cap 返回队列长度。
func (q *Queue[E]) Cap() uint32 {
	return q.capacity
}

// Len 返回队列数据个数。
func (q *Queue[E]) Len() uint32 {
	return atomic.LoadUint32(&q.tailIndex) - atomic.LoadUint32(&q.headIndex)
}

// IsEmpty 判断队列是否有数据。
func (q *Queue[E]) IsEmpty() bool {
	return atomic.LoadUint32(&q.headIndex) == atomic.LoadUint32(&q.tailIndex)
}

// IsFull 判断队列是否已满。
func (q *Queue[E]) IsFull() bool {
	return atomic.LoadUint32(&q.tailIndex)-atomic.LoadUint32(&q.headIndex) == q.capacity
}

// String 返回队列字符串表示形式值。
func (q *Queue[E]) String() string {
	return fmt.Sprintf(`Queue: Head:%d Tail:%d Len:%d Cap:%d`,
		atomic.LoadUint32(&q.headIndex), atomic.LoadUint32(&q.tailIndex), q.Len(), q.Cap())
}

// 返回已使用的位置数量。
func (q *Queue[E]) usedSize(tailIndex, headIndex uint32) uint32 {
	return tailIndex - headIndex
}

// 剩余可用的位置数量。
func (q *Queue[E]) leftSize(tailIndex, headIndex uint32) uint32 {
	return q.capacity - q.usedSize(tailIndex, headIndex)
}

// 循环和 CAS 方式获取可以插入队列的位置。
func (q *Queue[E]) acquirePut(wanSize uint32) (
	canInsertIndexStart uint32, canInsertSize uint32, insertedLeftSize uint32, err error) {

	var headIndex, tailIndex, leftSize uint32

	for {
		headIndex = atomic.LoadUint32(&q.headIndex)
		tailIndex = atomic.LoadUint32(&q.tailIndex)
		leftSize = q.leftSize(tailIndex, headIndex)
		if leftSize == 0 {
			return 0, 0, 0, ErrQueueIsFull
		}
		if wanSize > leftSize {
			wanSize = leftSize
		}

		if atomic.CompareAndSwapUint32(&q.tailIndex, tailIndex, tailIndex+wanSize) {
			return tailIndex + 1, wanSize, leftSize - wanSize, nil
		}

		runtime.Gosched()
	}
}

// 循环和 CAS 方式获取可以取出元素的位置。
func (q *Queue[E]) acquireGet(wantSize uint32) (
	canTakeIndexStart uint32, canTakeSize uint32, leftSize uint32, err error) {

	var headIndex, tailIndex, usedIndex uint32

	for {
		headIndex = atomic.LoadUint32(&q.headIndex)
		tailIndex = atomic.LoadUint32(&q.tailIndex)
		usedIndex = q.usedSize(tailIndex, headIndex)
		if usedIndex == 0 {
			return 0, 0, 0, ErrQueueIsEmpty
		}
		if wantSize > usedIndex {
			wantSize = usedIndex
		}

		if atomic.CompareAndSwapUint32(&q.headIndex, headIndex, headIndex+wantSize) {
			return headIndex + 1, wantSize, usedIndex - wantSize, nil
		}

		runtime.Gosched()
	}
}

// 获取队列元素。
func (q *Queue[E]) get(position uint32) E {
	elem := &q.elements[position&q.mask]
	for !(position == atomic.LoadUint32(&elem.getSequence) &&
		position == atomic.LoadUint32(&elem.putSequence)-q.capacity) {
		runtime.Gosched()
	}

	value := elem.value
	var emptyValue E
	elem.value = emptyValue
	_ = atomic.AddUint32(&elem.getSequence, q.capacity)
	return value
}

// 放入队列元素。
func (q *Queue[E]) put(position uint32, value E) {
	elem := &q.elements[position&q.mask]
	for !(position == atomic.LoadUint32(&elem.getSequence) && position == atomic.LoadUint32(&elem.putSequence)) {
		runtime.Gosched()
	}

	elem.value = value
	_ = atomic.AddUint32(&elem.putSequence, q.capacity)
}
