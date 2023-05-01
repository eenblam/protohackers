package main

import (
	"container/heap"
	"log"
	"sync"
)

/*
type Job struct {
	Priority int
	ID       int64
	Val      json.RawMessage
	Assignee *net.TCPConn
	Queue    string
	// Index for priority queue consumption
	index int
}
*/

// See https://pkg.go.dev/container/heap#pkg-types
type PriorityQueue struct {
	mux sync.Mutex
	q   []*Job
}

func (pq PriorityQueue) Len() int { return len(pq.q) }

func (pq PriorityQueue) Less(i, j int) bool {
	// > for max priority queue, < for min priority queue
	return pq.q[i].Priority > pq.q[j].Priority
}

func (pq *PriorityQueue) Swap(i, j int) {
	pq.q[i], pq.q[j] = pq.q[j], pq.q[i]
	pq.q[i].index = i
	pq.q[j].index = j
}

func (pq *PriorityQueue) Push(x any) {
	n := len(pq.q)
	job := x.(*Job)
	job.index = n
	pq.q = append(pq.q, job)
}

func (pq *PriorityQueue) Pop() any {
	old := pq.q
	n := len(pq.q)
	job := old[n-1]
	// Avoid memory leak
	old[n-1] = nil
	// for safety (???)
	job.index = -1
	// Strip removed element from array
	pq.q = old[0 : n-1]
	return job
}

// Returns the job at the top of the queue without removal.
// Like other methods, callers should manually lock and unlock the queue.
func (pq PriorityQueue) Max() (*Job, bool) {
	n := len(pq.q)
	if n > 0 {
		//return pq.q[n-1], true
		return pq.q[0], true
	}
	return nil, false
}

// Just don't wanna write this all the time, just want job := pq.HPop()
func (pq *PriorityQueue) HPop() *Job {
	return heap.Pop(pq).(*Job)
}

func (pq *PriorityQueue) HPush(job *Job) {
	heap.Push(pq, job)
}

func (pq *PriorityQueue) Delete(job *Job) {
	//TODO check if job is assigned / if index = -1
	// Otherwise heap.Remove will hang if index=-1
	log.Printf("PQ.Delete: heap.Remove(pq, %d)", job.index)
	heap.Remove(pq, job.index)
	log.Printf("PQ.Delete: deleted %d", job.index)
}

// Don't really need this, but it calls heap.Fix after updating priority
// Would be useful if we had to support updating an item's priority
// func (pq *PriorityQueue) update(job *Job, value TODO, priority Int)
