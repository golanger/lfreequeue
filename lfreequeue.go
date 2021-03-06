package lfreequeue

import (
	"unsafe"
	"sync"
	"sync/atomic"
)

type Node struct {
	value interface{}
	next *Node
}

type Queue struct {
	dummy *Node
	tail *Node
	
	wakeUpNotifiesLock *sync.Mutex
	wakeUpNotifies []chan int
}

func NewQueue() *Queue {
	q := new(Queue)
	q.dummy = new(Node)
	q.tail = q.dummy

	q.wakeUpNotifiesLock = new(sync.Mutex)

	return q
}

func (q *Queue) Enqueue(v interface{}) {
	var oldTail, oldTailNext *Node

	newNode := new(Node)
	newNode.value = v

	newNodeAdded := false

	for !newNodeAdded {
		oldTail = q.tail
		oldTailNext = oldTail.next

		if q.tail != oldTail {
			continue
		}

		if oldTailNext != nil {
			atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(oldTail), unsafe.Pointer(oldTailNext))
			continue
		}

		newNodeAdded = atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&oldTail.next)), unsafe.Pointer(oldTailNext), unsafe.Pointer(newNode))
	}

	atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(oldTail), unsafe.Pointer(newNode))

	// notify
	q.wakeUpNotifiesLock.Lock()
	toNotifies := q.wakeUpNotifies
	q.wakeUpNotifies = []chan int{}
	q.wakeUpNotifiesLock.Unlock()

	for _, notify := range toNotifies {
		notify <- 1
	}
}

func (q *Queue) Dequeue() (interface{}, bool) {
	var temp interface{}
	var oldDummy, oldHead *Node

	removed := false

	for !removed {
		oldDummy = q.dummy
		oldHead = oldDummy.next
		oldTail := q.tail

		if q.dummy != oldDummy {
			continue
		}

		if oldHead == nil {
			return nil, false
		}

		if oldTail == oldDummy {
			atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(oldTail), unsafe.Pointer(oldHead))
			continue
		}

		temp = oldHead.value
		removed = atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.dummy)), unsafe.Pointer(oldDummy), unsafe.Pointer(oldHead))
	}

	return temp, true
}

func (q *Queue) iterate(c chan<- interface{}) {
	for {
		datum, ok := q.Dequeue()
		if !ok {
			break
		}

		c <- datum
	}
	close(c)
}

func (q *Queue) Iter() <-chan interface{} {
	c := make(chan interface{})
	go q.iterate(c)
	return c
}

func (q *Queue) WatchWakeup() <-chan int {
	c := make(chan int)

	q.wakeUpNotifiesLock.Lock()
	defer q.wakeUpNotifiesLock.Unlock()

	q.wakeUpNotifies = append(q.wakeUpNotifies, c)

	return c
}

func (q *Queue) NewWatchIterator() *watchIterator {
	return &watchIterator{
		queue: q,
		quit: make(chan int),
	}
}