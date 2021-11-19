// Package safe_queue 这是一个高性能的多协程安全的 FIFO 队列。
// TODO：极端情形下put回到上次put还未设置完的elem处，存在覆盖值问题。
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
	// ErrTooMoreValues PutMore 填充数据过多。
	ErrTooMoreValues = errors.New("数据个数大于队列长度无法填充")
	// ErrNotEnoughValues GetMore 欲取出数据个数太大。
	ErrNotEnoughValues = errors.New("欲取出数据个数大于队列长度")
)

type (
	// Queue 队列结构体。使用 New 创建变量。
	Queue struct {
		capacity, mask uint32
		_              [cacheLinePadSize - 8]byte
		head           uint32
		_              [cacheLinePadSize - 4]byte
		tail           uint32
		_              [cacheLinePadSize - 4]byte
		elements       []element
		_              [cacheLinePadSize - unsafe.Sizeof([]element{})]byte
	}
	element struct {
		getSeq, putSeq uint32
		value          interface{}
		_              [cacheLinePadSize - 8 - 16]byte
	}
)

// New 创建队列。
//
// capacity 队列长度。值将调整为以2为底的幂数，最小值为2。
//
// *Queue 队列对象。
func New(capacity uint32) *Queue {
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

	instance := &Queue{
		capacity: capacity,
		elements: make([]element, capacity),
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

// Put 向队列尾部填充数据。
//
// uint32 返回剩余可填充数据个数。
//
// error 若队列已满返回错误 ErrQueueIsFull。
func (q *Queue) Put(value interface{}) (uint32, error) {
	position, _, left, err := q.acquirePut(1)
	if err != nil {
		return 0, err
	}
	q.put(position, value)
	return left, nil
}

// Get 取出队列头部数据。
//
// interface{} 返回队列数据。
//
// uint32 队列剩余可取个数。
//
// error 当无数据可取时返回错误 ErrQueueIsEmpty。
func (q *Queue) Get() (interface{}, uint32, error) {
	position, _, used, err := q.acquireGet(1)
	if err != nil {
		return nil, 0, err
	}
	val := q.get(position)
	return val, used, nil
}

// PutEnough 向队列填充多个数据。
//
// values 数据。
//
// uint32 实际填充数据个数。
//
// uint32 剩余可填充数据个数。
//
// error 当数据个数大于队列长度时返回 ErrTooMoreValues。
func (q *Queue) PutEnough(values ...interface{}) (uint32, uint32, error) {
	size := uint32(len(values))
	if size == 0 {
		return 0, 0, nil
	}
	position, actualSize, left, err := q.acquirePut(size)
	if err != nil {
		return 0, 0, err
	}

	for i, j := position, 0; i < position+actualSize; i, j = i+1, j+1 {
		q.put(i, values[j])
	}

	return actualSize, left, nil
}

// GetEnough 从队列取出多个数据。
//
// size 欲取出的数据个数。
//
// []interface{} 队列数据。
//
// uint32 实际取出数据个数。
//
// uint32 剩余可取数据个数。
//
// error 异常返回。
func (q *Queue) GetEnough(size uint32) ([]interface{}, uint32, uint32, error) {
	if size == 0 {
		return nil, 0, 0, nil
	}

	position, actualSize, used, err := q.acquireGet(size)
	if err != nil {
		return nil, 0, 0, err
	}

	res := make([]interface{}, 0, actualSize)
	for i := position; i < position+actualSize; i++ {
		res = append(res, q.get(i))
	}

	return res, actualSize, used, nil
}

func (q *Queue) PutTimeout() {}
func (q *Queue) GetTimeout() {}

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
	return fmt.Sprintf(`Queue: Head:%d Tail:%d Len:%d Cap:%d`,
		atomic.LoadUint32(&q.head), atomic.LoadUint32(&q.tail), q.Len(), q.Cap())
}

func (q *Queue) usedSize(tail, head uint32) uint32 {
	return tail - head
}

func (q *Queue) leftSize(tail, head uint32) uint32 {
	return q.capacity - q.usedSize(tail, head)
}

func (q *Queue) acquirePut(size uint32) (uint32, uint32, uint32, error) {
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

func (q *Queue) acquireGet(size uint32) (uint32, uint32, uint32, error) {
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

func (q *Queue) get(position uint32) interface{} {
	elem := &q.elements[position&q.mask]
	for !(position == atomic.LoadUint32(&elem.getSeq) && position == atomic.LoadUint32(&elem.putSeq)-q.capacity) {
		runtime.Gosched()
	}
	val := elem.value
	elem.value = nil
	_ = atomic.AddUint32(&elem.getSeq, q.capacity)
	return val
}

func (q *Queue) put(position uint32, value interface{}) {
	elem := &q.elements[position&q.mask]
	for !(position == atomic.LoadUint32(&elem.getSeq) && position == atomic.LoadUint32(&elem.putSeq)) {
		runtime.Gosched()
	}
	elem.value = value
	_ = atomic.AddUint32(&elem.putSeq, q.capacity)
}
