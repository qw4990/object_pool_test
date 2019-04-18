package chunk

import (
	"container/list"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

type column struct {
}

func init() {
	fmt.Println("NUM CPU: ", runtime.NumCPU())
}

type lockSlice struct {
	sync.Mutex
	cols []*column
}

func (ps *lockSlice) put(col *column) {
	ps.Lock()
	defer ps.Unlock()

	ps.cols = append(ps.cols, col)
}

func (ps *lockSlice) get() *column {
	ps.Lock()
	defer ps.Unlock()

	if len(ps.cols) > 0 {
		col := ps.cols[len(ps.cols)-1]
		ps.cols = ps.cols[:len(ps.cols)-1]
		return col
	}
	return new(column)
}

func BenchmarkLockSlice(b *testing.B) {
	pool := &lockSlice{cols: make([]*column, 0, 128)}
	pool.put(new(column))

	b.SetParallelism(runtime.NumCPU())
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.put(pool.get())
		}
	})
}

type lockList struct {
	sync.Mutex
	cols *list.List
}

func (ps *lockList) put(col *column) {
	ps.Lock()
	defer ps.Unlock()
	ps.cols.PushFront(col)
}

func (ps *lockList) get() *column {
	ps.Lock()
	defer ps.Unlock()
	if ps.cols.Len() > 0 {
		head := ps.cols.Front()
		return ps.cols.Remove(head).(*column)
	}
	return new(column)
}

func BenchmarkLockList(b *testing.B) {
	pool := &lockList{cols: list.New()}
	pool.put(new(column))

	b.SetParallelism(runtime.NumCPU())
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.put(pool.get())
		}
	})
}

type channel struct {
	pool chan *column
}

func (p *channel) put(col *column) {
	select {
	case p.pool <- col:
	default:
	}
}

func (p *channel) get() *column {
	select {
	case col := <-p.pool:
		return col
	default:
		return new(column)
	}
}

func BenchmarkChannel(b *testing.B) {
	pool := &channel{
		pool: make(chan *column, 128),
	}
	pool.put(new(column))

	b.SetParallelism(runtime.NumCPU())
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.put(pool.get())
		}
	})
}

type multiChannel struct {
	cs      []*channel
	mask    uint64
	curPush uint64
	curPop  uint64
}

func NewMultiChannel(limit, nBit uint64) *multiChannel {
	cs := make([]*channel, 1<<nBit)
	for i := 0; i < (1 << nBit); i++ {
		cs[i] = &channel{
			pool: make(chan *column, limit),
		}
	}
	return &multiChannel{
		cs:   cs,
		mask: (1 << nBit) - 1,
	}
}

func (ma *multiChannel) push(c *column) {
	cur := atomic.AddUint64(&ma.curPush, 1)
	cur &= ma.mask
	ma.cs[cur].put(c)
}

func (ma *multiChannel) pop() *column {
	cur := atomic.AddUint64(&ma.curPop, 1)
	cur &= ma.mask
	return ma.cs[cur].get()
}

func BenchmarkMultiChannel(b *testing.B) {
	pool := NewMultiChannel(128, 8)
	pool.push(new(column))

	b.SetParallelism(runtime.NumCPU())
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.push(pool.pop())
		}
	})
}

type node struct {
	value *column
	next  *node
}

type lockFreeList struct {
	head *node
	tail *node
}

func newLockFreeList() *lockFreeList {
	dummy := new(node)
	return &lockFreeList{
		head: dummy,
		tail: dummy,
	}
}

func (q *lockFreeList) push(v *column) {
	node := &node{value: v}

	for swapped := false; !swapped; {
		swapped = atomic.CompareAndSwapPointer(
			/* addr */ (*unsafe.Pointer)(unsafe.Pointer(&q.tail.next)),
			/* old  */ nil,
			/* new  */ unsafe.Pointer(node))
	}

	oldTail := q.tail
	atomic.CompareAndSwapPointer(
		/* addr */ (*unsafe.Pointer)(unsafe.Pointer(&q.tail)),
		/* old  */ unsafe.Pointer(oldTail),
		/* new  */ unsafe.Pointer(node))
}

func (q *lockFreeList) pop() *column {
	var res *node
	for swapped := false; !swapped; {
		res = q.head.next
		if res == nil {
			return new(column)
		}

		swapped = atomic.CompareAndSwapPointer(
			/* addr */ (*unsafe.Pointer)(unsafe.Pointer(&q.head.next)),
			/* old  */ unsafe.Pointer(res),
			/* new  */ unsafe.Pointer(res.next))
	}

	atomic.CompareAndSwapPointer(
		/* addr */ (*unsafe.Pointer)(unsafe.Pointer(&q.tail)),
		/* old  */ unsafe.Pointer(res),
		/* new  */ unsafe.Pointer(q.head))

	return res.value
}

func BenchmarkLockFreeList(b *testing.B) {
	q := newLockFreeList()
	q.push(new(column))
	b.SetParallelism(runtime.NumCPU())
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.push(q.pop())
		}
	})
}

type multiLockFreeList struct {
	qs      []*lockFreeList
	mask    uint64
	curPush uint64
	curPop  uint64
}

func (ma *multiLockFreeList) push(c *column) {
	cur := atomic.AddUint64(&ma.curPush, 1)
	cur &= ma.mask
	ma.qs[cur].push(c)
}

func (ma *multiLockFreeList) pop() *column {
	cur := atomic.AddUint64(&ma.curPop, 1)
	cur &= ma.mask
	return ma.qs[cur].pop()
}

func newMultiLockFreeList(nBit uint64) *multiLockFreeList {
	qs := make([]*lockFreeList, 1<<nBit)
	for i := 0; i < (1 << nBit); i++ {
		qs[i] = newLockFreeList()
	}
	return &multiLockFreeList{
		qs:   qs,
		mask: (1 << nBit) - 1,
	}
}

func BenchmarkMultiLockFreeList(b *testing.B) {
	q := newMultiLockFreeList(8)
	q.push(new(column))
	b.SetParallelism(runtime.NumCPU())
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.push(q.pop())
		}
	})
}

type lockFreeSlice struct {
	buf   []*column
	head  int64
	tail  int64
	n     int64
	limit int64
}

func newArray(limit int) *lockFreeSlice {
	return &lockFreeSlice{
		buf:   make([]*column, limit),
		limit: int64(limit),
	}
}

func (a *lockFreeSlice) push(c *column) {
	if atomic.AddInt64(&a.n, 1) >= a.limit {
		atomic.AddInt64(&a.n, -1)
		return
	}
	for {
		t1 := atomic.LoadInt64(&a.tail)
		t2 := t1 + 1
		if t2 >= a.limit {
			t2 -= a.limit
		}
		if atomic.CompareAndSwapInt64(&a.tail, t1, t2) {
			a.buf[t1] = c
			return
		}
	}
}

func (a *lockFreeSlice) pop() *column {
	if atomic.AddInt64(&a.n, -1) < 0 {
		atomic.AddInt64(&a.n, 1)
		return new(column)
	}
	for {
		h1 := atomic.LoadInt64(&a.head)
		h2 := h1 + 1
		if h2 >= a.limit {
			h2 -= a.limit
		}
		if atomic.CompareAndSwapInt64(&a.head, h1, h2) {
			x := a.buf[h1]
			a.buf[h1] = nil
			return x
		}
	}
}

func BenchmarkLockFreeSlice(b *testing.B) {
	q := newArray(10000)
	q.push(new(column))
	b.SetParallelism(runtime.NumCPU())
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.push(q.pop())
		}
	})
}

type multiLockFreeSlice struct {
	as      []*lockFreeSlice
	mask    uint64
	curPush uint64
	curPop  uint64
}

func NewMultiArray(limit, nBit uint64) *multiLockFreeSlice {
	as := make([]*lockFreeSlice, 1<<nBit)
	for i := 0; i < (1 << nBit); i++ {
		as[i] = newArray(int(limit))
	}
	return &multiLockFreeSlice{
		as:   as,
		mask: (1 << nBit) - 1,
	}
}

func (ma *multiLockFreeSlice) push(c *column) {
	cur := atomic.AddUint64(&ma.curPush, 1)
	cur &= ma.mask
	ma.as[cur].push(c)
}

func (ma *multiLockFreeSlice) pop() *column {
	cur := atomic.AddUint64(&ma.curPop, 1)
	cur &= ma.mask
	return ma.as[cur].pop()
}

func BenchmarkMultiLockFreeSlice(b *testing.B) {
	q := NewMultiArray(1000, 8)
	q.push(new(column))
	b.SetParallelism(runtime.NumCPU())
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.push(q.pop())
		}
	})
}

func BenchmarkSyncPool(b *testing.B) {
	p := &sync.Pool{
		New: func() interface{} { return new(column) },
	}
	p.Put(1)
	//b.SetParallelism(runtime.NumCPU())
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p.Put(p.Get())
		}
	})
}

type MultiSyncPool struct {
	ps      []*sync.Pool
	mask    uint64
	curPush uint64
	curPop  uint64
}

func newMultiSyncPool(nBit uint64) *MultiSyncPool {
	ps := make([]*sync.Pool, 1<<nBit)
	for i := 0; i < (1 << nBit); i++ {
		ps[i] = &sync.Pool{New: func() interface{} { return new(column) }}
	}
	return &MultiSyncPool{
		ps:   ps,
		mask: (1 << nBit) - 1,
	}
}

func (ma *MultiSyncPool) push(c *column) {
	cur := atomic.AddUint64(&ma.curPush, 1)
	cur &= ma.mask
	ma.ps[cur].Put(c)
}

func (ma *MultiSyncPool) pop() *column {
	cur := atomic.AddUint64(&ma.curPop, 1)
	cur &= ma.mask
	return ma.ps[cur].Get().(*column)
}

func BenchmarkMultiSyncPool(b *testing.B) {
	p := newMultiSyncPool(8)
	p.push(new(column))
	b.SetParallelism(runtime.NumCPU())
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p.push(p.pop())
		}
	})
}

func BenchmarkCASUnsafe(b *testing.B) {
	var x int
	b.SetParallelism(runtime.NumCPU())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		atomic.CompareAndSwapPointer(
			(*unsafe.Pointer)(unsafe.Pointer(&x)),
			unsafe.Pointer(&x),
			unsafe.Pointer(&x))
	}
}

func BenchmarkCASInt64(b *testing.B) {
	var x int64
	b.SetParallelism(runtime.NumCPU())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		atomic.CompareAndSwapInt64(&x, 0, 0)
	}
}
