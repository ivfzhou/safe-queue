// Package safe_queue 这是一个高性能的多协程安全的 FIFO 队列。
// TODO：极端情形下put回到上次put还未设置完的elem处，存在覆盖值问题。
package safe_queue

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
)

var (
	// ErrQueueIsFull 表明队列已满。
	ErrQueueIsFull = errors.New("队列已满")
	// ErrQueueIsEmpty 表明队列为空。
	ErrQueueIsEmpty = errors.New("队列为空")
	// ErrTooMoreValues PutMore 填充数据过多。
	ErrTooMoreValues = errors.New("数据个数大于队列长度无法填充")
	// ErrNotEnoughValues GetMore 欲取出数据个数太大。
	ErrNotEnoughValues = errors.New("欲取出数据个数大于队列长度")
)

type (
	Queue struct {
		capacity, head, tail uint32
		elements             []element
		once                 sync.Once
	}
	element struct {
		getSeq, putSeq uint32
		value          interface{}
	}
)

// New 创建队列。
//
// capacity 队列长度。最大值为 1<<32-2。
//
// *Queue 队列对象。
func New(capacity uint32) *Queue {
	if capacity == 0 {
		return nil
	}
	if capacity == math.MaxUint32 {
		capacity = math.MaxUint32 - 1
	}

	instance := &Queue{
		capacity: capacity,
		elements: make([]element, capacity),
	}
	for i := range instance.elements {
		instance.elements[i].putSeq = uint32(i)
		instance.elements[i].getSeq = uint32(i)
	}
	instance.elements[0].putSeq = capacity
	instance.elements[0].getSeq = capacity

	return instance
}

// Put 向队列尾部填充数据。
//
// uint32 返回剩余可填充数据个数。
//
// error 若队列已满返回错误 ErrQueueIsFull。
func (q *Queue) Put(value interface{}) (uint32, error) {
	q.init()
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

	q.put(position, value)

	return q.capacity - used, nil
}

// Get 取出队列头部数据。
//
// interface{} 返回队列数据。
//
// uint32 队列剩余可取个数。
//
// error 当无数据可取时返回错误 ErrQueueIsEmpty。
func (q *Queue) Get() (interface{}, uint32, error) {
	q.init()
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

// PutMore 向队列填充多个数据。
//
// ctx 上下文。
//
// values 数据。
//
// uint32 返回剩余可填充数据个数。
//
// error 当数据个数大于队列长度时返回 ErrTooMoreValues。
func (q *Queue) PutMore(ctx context.Context, values ...interface{}) (uint32, error) {
	q.init()
	if len(values) == 0 {
		return 0, nil
	}
	if uint32(len(values)) > q.capacity {
		return 0, ErrTooMoreValues
	}
	position := uint32(0)
	used := uint32(0)
	size := uint32(len(values))
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
		head := atomic.LoadUint32(&q.head)
		tail := atomic.LoadUint32(&q.tail)
		position = tail + size
		used = position - head
		if used > q.capacity || used == 0 {
			runtime.Gosched()
			continue
		}
		if atomic.CompareAndSwapUint32(&q.tail, tail, tail+size) {
			position = tail + 1
			break
		}
		runtime.Gosched()
	}

	for i, j := position, 0; i < position+size; i, j = i+1, j+1 {
		q.put(i, values[j])
	}

	return q.capacity - used, nil
}

// GetMore 从队列取出多个数据。返回剩余可取个数，当无任何数据可取时返回错误 ErrQueueIsEmpty。
//
// ctx 若上下文结束，error 返回 ctx.Err()。
//
// num 欲取出的数据个数。
//
// []interface{} 队列数据。
//
// uint32 剩余可取数据个数。
//
// error 异常返回。
func (q *Queue) GetMore(ctx context.Context, num uint32) ([]interface{}, uint32, error) {
	q.init()
	if num == 0 {
		return nil, 0, nil
	}
	if num > q.capacity {
		return nil, 0, ErrNotEnoughValues
	}
	position := uint32(0)
	used := uint32(0)
	size := num
	for {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		default:
		}
		head := atomic.LoadUint32(&q.head)
		tail := atomic.LoadUint32(&q.tail)
		used = tail - head
		if used == 0 || size > used {
			runtime.Gosched()
			continue
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

	return res, used - size, nil
}

// Cap 返回队列长度。
//
// uint32 队列长度。
func (q *Queue) Cap() uint32 {
	return q.capacity
}

// Len 返回队列数据个数。
//
// 此时队列数据个数。
func (q *Queue) Len() uint32 {
	return atomic.LoadUint32(&q.tail) - atomic.LoadUint32(&q.head)
}

// IsEmpty 判断队列是否有数据。
//
// bool 队列数据个数是否为零。
func (q *Queue) IsEmpty() bool {
	return atomic.LoadUint32(&q.head) == atomic.LoadUint32(&q.tail)
}

// IsFull 判断队列是否已满。
//
// bool 队列数据个数是否已满。
func (q *Queue) IsFull() bool {
	return atomic.LoadUint32(&q.tail)-atomic.LoadUint32(&q.head) == q.capacity
}

// String 返回队列字符串表示形式值。
//
// string 队列字符串值。
func (q *Queue) String() string {
	if q == nil {
		return `<nil>`
	}
	elems := make([]interface{}, len(q.elements))
	for i, v := range q.elements {
		elems[i] = v.value
	}
	return fmt.Sprintf(`{Len:%d Cap:%d Values:%v}`, q.Len(), q.Cap(), elems)
}

func (q *Queue) init() {
	q.once.Do(func() {
		if q.capacity == 0 {
			*q = *New(math.MaxUint32)
		}
	})
}

func (q *Queue) get(position uint32) interface{} {
	elem := &q.elements[position%q.capacity]
	for !(position == atomic.LoadUint32(&elem.getSeq) && position == atomic.LoadUint32(&elem.putSeq)-q.capacity) {
		runtime.Gosched()
	}
	val := elem.value
	elem.value = nil
	_ = atomic.AddUint32(&elem.getSeq, q.capacity)
	return val
}

func (q *Queue) put(position uint32, value interface{}) {
	elem := &q.elements[position%q.capacity]
	for !(position == atomic.LoadUint32(&elem.getSeq) && position == atomic.LoadUint32(&elem.putSeq)) {
		runtime.Gosched()
	}
	elem.value = value
	_ = atomic.AddUint32(&elem.putSeq, q.capacity)
}
