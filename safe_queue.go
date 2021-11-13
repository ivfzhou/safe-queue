// Package safe_queue 这是一个高性能多线程安全的FIFO队列容器库。
package safe_queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
)

var (
	// ErrFull 表明容器已满。
	ErrFull = errors.New("队列已满")
	// ErrEmpty 表明容器为空。
	ErrEmpty = errors.New("队列已空")
)

type (
	queue struct {
		capacity, mask, head, tail uint32
		data                       []element
	}
	element struct {
		putSeq, getSeq uint32
		Value          interface{}
	}
)

// New 创建容器实例。
// capacity 容器大概的容量，将调整为 2 的幂数。
func New(capacity uint32) *queue {
	capacity = roundCapacity(capacity)
	if capacity < 2 {
		capacity = 2
	}

	instance := &queue{
		capacity: capacity,
		mask:     capacity - 1,
		data:     make([]element, capacity),
	}

	for i := range instance.data {
		instance.data[i].getSeq = uint32(i)
		instance.data[i].putSeq = uint32(i)
	}
	instance.data[0].getSeq = capacity
	instance.data[0].putSeq = capacity

	return instance
}

// Put 向容器填充数据。返回剩余可填充数据个数，若容器已满返回错误 ErrFull。
func (q *queue) Put(val interface{}) (uint32, error) {
	position := uint32(0)
	remain := uint32(0)
	for {
		head := atomic.LoadUint32(&q.head)
		tail := atomic.LoadUint32(&q.tail)
		position = tail + 1
		remain = position - head
		if remain > q.mask {
			return 0, ErrFull
		}
		if atomic.CompareAndSwapUint32(&q.tail, tail, position) {
			break
		}
		runtime.Gosched()
	}

	elem := &q.data[position&q.mask]
	for {
		getSeq := atomic.LoadUint32(&elem.getSeq)
		putSeq := atomic.LoadUint32(&elem.putSeq)
		if position == putSeq && position == getSeq {
			break
		}
		runtime.Gosched()
	}
	elem.Value = val
	atomic.AddUint32(&elem.putSeq, q.capacity)

	return q.mask - remain, nil
}

// Get 取出容器数据。返回剩余可取个数，当无数据可取时返回错误 ErrEmpty。
func (q *queue) Get() (interface{}, uint32, error) {
	position := uint32(0)
	leave := uint32(0)
	for {
		head := atomic.LoadUint32(&q.head)
		tail := atomic.LoadUint32(&q.tail)
		leave = tail - head
		if leave <= 0 {
			return nil, 0, ErrEmpty
		}
		if atomic.CompareAndSwapUint32(&q.head, head, head+1) {
			position = head + 1
			break
		}
		runtime.Gosched()
	}

	return q.getOne(position), leave - 1, nil
}

// GetMore 取出多个容器数据。返回剩余可取个数，当无任何数据可取时返回错误 ErrEmpty。
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
			return nil, 0, ErrEmpty
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
		res = append(res, q.getOne(i))
	}

	return res, left - size, nil
}

// Cap 返回队列容量。
func (q *queue) Cap() uint32 {
	return q.mask
}

// Len 返回队列数据个数。
func (q *queue) Len() uint32 {
	return atomic.LoadUint32(&q.tail) - atomic.LoadUint32(&q.head)
}

// IsEmpty 判断队列是否有数据。
func (q *queue) IsEmpty() bool {
	return atomic.LoadUint32(&q.head) == atomic.LoadUint32(&q.tail)
}

// IsFull 判断队列是否已满。
func (q *queue) IsFull() bool {
	return atomic.LoadUint32(&q.tail)-atomic.LoadUint32(&q.head) >= q.mask
}

func (q *queue) getOne(position uint32) interface{} {
	elem := &q.data[position&q.mask]
	for {
		getSeq := atomic.LoadUint32(&elem.getSeq)
		putSeq := atomic.LoadUint32(&elem.putSeq)
		if position == getSeq && position == putSeq-q.capacity {
			break
		}
		runtime.Gosched()
	}
	atomic.AddUint32(&elem.getSeq, q.capacity)
	val := elem.Value
	elem.Value = nil
	return val
}

func (q *queue) String() string {
	data, _ := json.Marshal(q.data)
	return fmt.Sprintf(`{"queue":{"head":%q,"tail":%q,"cap":%q,"len":%q,"data":%s}}`,
		atomic.LoadUint32(&q.head), atomic.LoadUint32(&q.tail), q.Cap(), q.Len(), data)
}

func roundCapacity(capacity uint32) uint32 {
	capacity--
	capacity |= capacity >> 1
	capacity |= capacity >> 2
	capacity |= capacity >> 4
	capacity |= capacity >> 8
	capacity |= capacity >> 16
	capacity++
	return capacity
}
