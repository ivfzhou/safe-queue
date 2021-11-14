// Package safe_queue 这是一个高性能的多协程安全的 FIFO 队列。
package safe_queue

import (
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
)

var (
	// ErrQueueIsFull 表明队列已满。
	ErrQueueIsFull = errors.New("队列已满")
	// ErrQueueIsEmpty 表明队列为空。
	ErrQueueIsEmpty = errors.New("队列为空")
)

type (
	queue struct {
		capacity, mask, head, tail uint32
		elements                   []element
	}
	element struct {
		hadSet uint32
		value  interface{}
	}
)

// New 创建队列。
//
// capacity 队列长度。
func New(capacity uint32) *queue {
	if capacity == 0 {
		return nil
	}

	instance := &queue{
		capacity: capacity,
		mask:     capacity - 1,
		elements: make([]element, capacity),
	}

	return instance
}

// Put 向队列尾部填充数据。
//
// uint32 返回剩余可填充数据个数。
//
// error 若队列已满返回错误 ErrQueueIsFull。
func (q *queue) Put(value interface{}) (uint32, error) {
	position := uint32(0)
	used := uint32(0)
	for {
		head := atomic.LoadUint32(&q.head)
		tail := atomic.LoadUint32(&q.tail)
		position = tail + 1
		used = position - head
		if used > q.capacity || used == 0 {
			return 0, ErrQueueIsFull
		}
		if atomic.CompareAndSwapUint32(&q.tail, tail, position) {
			break
		}
		runtime.Gosched()
	}

	elem := &q.elements[position&q.mask]
	for {
		hadSet := atomic.LoadUint32(&elem.hadSet)
		if hadSet == 0 {
			break
		}
		runtime.Gosched()
	}
	elem.value = value
	atomic.StoreUint32(&elem.hadSet, 1)

	return q.capacity - used, nil
}

// Get 取出队列头部数据。
//
// interface{} 返回队列数据。
//
// uint32 队列剩余可取个数。
//
// error 当无数据可取时返回错误 ErrQueueIsEmpty。
func (q *queue) Get() (interface{}, uint32, error) {
	position := uint32(0)
	used := uint32(0)
	for {
		head := atomic.LoadUint32(&q.head)
		tail := atomic.LoadUint32(&q.tail)
		used = tail - head
		if used == 0 {
			return nil, 0, ErrQueueIsEmpty
		}
		if atomic.CompareAndSwapUint32(&q.head, head, head+1) {
			position = head + 1
			break
		}
		runtime.Gosched()
	}

	return q.get(position), used - 1, nil
}

// GetMore 取出多个容器数据。返回剩余可取个数，当无任何数据可取时返回错误 ErrQueueIsEmpty。
// num 欲取出的数据个数。
func (q *queue) GetMore(num uint32) ([]interface{}, uint32, error) {
	position := uint32(0)
	left := uint32(0)
	size := num
	for {
		head := atomic.LoadUint32(&q.head)
		tail := atomic.LoadUint32(&q.tail)
		left = tail - head
		if left <= 0 {
			return nil, 0, ErrQueueIsEmpty
		}
		if size > left {
			size = left
		}
		if atomic.CompareAndSwapUint32(&q.head, head, head+size) {
			position = head + 1
			break
		}
		runtime.Gosched()
	}

	res := make([]interface{}, 0, size)
	for i := position; i < position+size; i++ {
		res = append(res, q.get(i))
	}

	return res, left - size, nil
}

// Cap 返回队列长度。
//
// uint32 队列长度。
func (q *queue) Cap() uint32 {
	return q.capacity
}

// Len 返回队列数据个数。
//
// 此时队列数据个数。
func (q *queue) Len() uint32 {
	return atomic.LoadUint32(&q.tail) - atomic.LoadUint32(&q.head)
}

// IsEmpty 判断队列是否有数据。
//
// bool 队列数据个数是否为零。
func (q *queue) IsEmpty() bool {
	return atomic.LoadUint32(&q.head) == atomic.LoadUint32(&q.tail)
}

// IsFull 判断队列是否已满。
//
// bool 队列数据个数是否已满。
func (q *queue) IsFull() bool {
	return atomic.LoadUint32(&q.tail)-atomic.LoadUint32(&q.head) == q.capacity
}

func (q *queue) get(position uint32) interface{} {
	elem := &q.elements[position&q.mask]
	for {
		hadSet := atomic.LoadUint32(&elem.hadSet)
		if hadSet != 0 {
			break
		}
		runtime.Gosched()
	}
	val := elem.value
	atomic.StoreUint32(&elem.hadSet, 0)
	elem.value = nil
	return val
}

// String 返回队列字符串表示形式值。
//
// string 队列字符串值。
func (q *queue) String() string {
	if q == nil {
		return `<nil>`
	}
	elems := make([]interface{}, len(q.elements))
	for i, v := range q.elements {
		elems[i] = v.value
	}
	return fmt.Sprintf(`{Len:%d Cap:%d Values:%v`, q.Len(), q.Cap(), elems)
}
