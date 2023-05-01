package main

//package protohackers

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

func toJSONLine[T any](t T) []byte {
	b, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}
	return append(b, '\n')
}

func fromJSON[T any](b []byte) (t T, err error) { err = json.Unmarshal(b, &t); return t, err }

type Job struct {
	Priority int
	ID       int64
	Val      json.RawMessage
	Assignee *net.TCPConn
	Queue    string
	// Index for priority queue consumption
	index int
}

type Request struct {
	Request string   `json:"request"` // "put", "get", "delete", "abort"
	Queues  []string `json:"queues"`  // GET only
	Wait    bool     `json:"wait"`    // GET only

	Queue string          `json:"queue"` // PUT only
	Job   json.RawMessage `json:"job"`   // PUT only
	Pri   int             `json:"pri"`   // PUT only

	ID int64 `json:"id"` // DELETE, ABORT only
}

var ids atomic.Int64

func nextID() int64 { return ids.Add(1) }

// var queues = make(map[string]map[int64]*Job) // queue_name => id => Job
var queues = make(map[string]*PriorityQueue) // queue_name => id => Job
// var queues sync.Map
var allJobs = make(map[int64]*Job)

// TODO repurpose this to just be for allJobs?
var mux sync.Mutex

/*
var queuePool = sync.Pool{New: func() any {
	return &PriorityQueue{q: []*Job{}}
}}

func getQueue(name string) *PriorityQueue {
	q := queuePool.Get().(*PriorityQueue)
	val, loaded := queues.LoadOrStore(name, q)
	if loaded {
		// Return q to pool, don't need it yet
		queuePool.Put(q)
	}
	return val.(*PriorityQueue)
}
*/

var responseOk = []byte(`{"status": "ok"}` + "\n")
var responseNoJob = []byte(`{"status": "no-job"}` + "\n")

func Put(request Request, clientJobs map[int64]*Job) (json.RawMessage, error) {
	if request.Queue == "" || request.Job == nil || request.Pri < 0 {
		return nil, errors.New("put: missing one or more of queue, job, or pri")
	}
	if len(request.Queues) != 0 || request.Wait || request.ID != 0 {
		return nil, errors.New("put: extra fields")
	}
	id := nextID()

	mux.Lock()
	if queues[request.Queue] == nil {
		queues[request.Queue] = &PriorityQueue{q: []*Job{}}
	}
	queue := queues[request.Queue]
	//queue := getQueue(request.Queue)
	job := &Job{Priority: request.Pri, ID: id, Val: request.Job, Assignee: nil, Queue: request.Queue}
	//queue.mux.Lock()
	queue.HPush(job)
	//queue.mux.Unlock()
	allJobs[id] = job
	mux.Unlock()
	log.Printf("Pushed job %d to queue %s", job.ID, job.Queue)
	return json.RawMessage(fmt.Sprintf(`{"status": "ok", "id":%d}`+"\n", id)), nil
}

func Get(conn *net.TCPConn, request Request, clientJobs map[int64]*Job) (*Job, bool, error) {
	if request.Queue != "" || request.Job != nil || request.Pri != 0 {
		return nil, false, errors.New("get: extra fields")
	}
	if len(request.Queues) == 0 {
		return nil, false, errors.New("get: missing field Queues")
	}
	// If request.Wait, loop forever until we find a request with sufficient priority.
	// we want the job with the HIGHEST priority in any of the queues
	//var maxJobQueue string
	//var maxJobID int64
	var maxJobPriority = -1
	var maxQueue *PriorityQueue
	var job *Job
	for i := 0; ; i++ {
		// Unlock after each check to allow jobs to be added,
		// otherwise no one will be able to add a job for us to assign.
		mux.Lock()
	FORQUEUE:
		for _, k := range request.Queues {
			/*
				maybeQ, ok := queues.Load(k)
				if !ok {
					continue
				}
				q := maybeQ.(*PriorityQueue)
			*/
			q, found := queues[k]
			if !found {
				//continue
				continue FORQUEUE
			}
			//q.mux.Lock()
			j, ok := q.Max()
			//q.mux.Unlock()
			if !ok {
				//continue
				continue FORQUEUE
			}
			if j.Priority > maxJobPriority {
				//log.Printf("Found job %d with priority %d", j.ID, j.Priority)
				maxJobPriority = j.Priority
				job = j
				maxQueue = q
			}
		}
		// If max found, assign to client and break
		if maxJobPriority > -1 {
			job = maxQueue.HPop()
			job.Assignee = conn
			clientJobs[job.ID] = job
			mux.Unlock()
			return job, true, nil
		}
		// Not found, so unlock so someone can add to the Queue.
		mux.Unlock()
		// If not waiting, send responseNoJob and listen for new request
		if !request.Wait {
			// Have to loop once before trying this
			// Response no-job, found=false
			return nil, false, nil
		}
		// Waiting, so just loop around again.
		time.Sleep(time.Duration(i) * 10 * time.Millisecond)
	}
	return nil, false, errors.New("Unreachable")
}

func handle09(conn *net.TCPConn) error {
	defer conn.Close()
	//conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	//readDeadlineSeconds := 30 * time.Second
	//conn.SetReadDeadline(time.Now().Add(readDeadlineSeconds))
	conn.SetKeepAlive(true)
	conn.SetKeepAlivePeriod(100 * time.Millisecond)

	logger := log.New(log.Writer(),
		conn.RemoteAddr().String()+" ",
		log.Flags()|log.Lshortfile|log.Lmsgprefix)

	logger.Println("Connected")

	// each request and response is a single string, terminated by a newline, that's a JSON object
	sendErrf := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		logger.Print("sendErrf: " + msg)
		resp := toJSONLine(struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}{
			Status: "error",
			Error:  msg,
		})
		conn.Write(resp)
	}
	scanner := bufio.NewScanner(conn)
	clientJobs := make(map[int64]*Job) // Job ID to job
	defer func() {                     // on exit, abort all the jobs this client was working on
		mux.Lock()
		for _, job := range clientJobs {
			if job.Assignee == conn {
				// Be sure it's still alive
				_, ok := allJobs[job.ID]
				if !ok {
					// Job was probably deleted
					logger.Printf("Disconnecting %s. Job %d not available to abort.", conn.RemoteAddr().String(), job.ID)
					continue
				}
				// Only nil out if it's assigned to this conn
				job.Assignee = nil
				// Return to queue
				/*
					maybeQ, ok := queues.Load(job.Queue)
					if !ok {
						panic(fmt.Sprintf("Queue %s does not exist but job %d specifies it", job.Queue, job.ID))
					}
					q := maybeQ.(*PriorityQueue)
				*/
				q := queues[job.Queue]
				q.HPush(job)
				logger.Printf("Disconnecting %s. Aborted job %d.", conn.RemoteAddr().String(), job.ID)
			}
		}
		mux.Unlock()
		logger.Println("Disconnected")
	}()
READLINE:

	for scanner.Scan() {
		logger.Println(scanner.Text())
		//conn.SetReadDeadline(time.Now().Add(readDeadlineSeconds))
		request, err := fromJSON[Request](scanner.Bytes())
		if err != nil {
			sendErrf("invalid request: %s", err)
			continue READLINE
		}
		switch request.Request {
		default:
			sendErrf("invalid request: unknown request type %s", request.Request)
			continue READLINE
		case "put":
			response, err := Put(request, clientJobs)
			if err != nil {
				sendErrf("put: %s", err)
			} else if _, err := conn.Write(response); err != nil {
				return fmt.Errorf("put: %w", err)
			}

		case "get":
			job, found, err := Get(conn, request, clientJobs)
			if err != nil {
				sendErrf("get: %s", err)
				continue READLINE
			}
			if !found {
				if _, err := conn.Write(responseNoJob); err != nil {
					logger.Printf("get: %s", err)
					return fmt.Errorf("get: %s", err) // client disconnected
				}
				continue READLINE
			}
			// If we got here, we've already assigned a job
			logger.Printf("Assigned job %d to conn %s", job.ID, conn.RemoteAddr().String())
			resp := toJSONLine(struct {
				Status string          `json:"status"`
				ID     int64           `json:"id"`
				Job    json.RawMessage `json:"job"`
				Pri    int             `json:"pri"`
				Queue  string          `json:"queue"`
			}{
				Status: "ok",
				ID:     job.ID,
				Job:    job.Val,
				Pri:    job.Priority,
				Queue:  job.Queue,
			})
			if _, err := conn.Write(resp); err != nil {
				logger.Printf("get: %s", err)
				return fmt.Errorf("get: %s", err) // client disconnected
			}
		case "delete":
			if request.ID <= 0 { // can't be zero, since the first call to nextID() returns 1
				sendErrf("delete: bad id")
				continue READLINE
			}
			logger.Println("DELETE: waiting for lock")
			mux.Lock()
			logger.Println("DELETE: got lock")
			job, ok := allJobs[request.ID]
			if !ok {
				mux.Unlock()
				logger.Printf("delete: id %d not found", request.ID)
				conn.Write([]byte(responseNoJob))
				continue READLINE
			}
			/*
				maybeQ, ok := queues.Load(job.Queue)
				if !ok {
					// Shouldn't happen, since queue is always created before job in PUT
					panic(fmt.Sprintf("Queue %s does not exist but job %d specifies it", job.Queue, job.ID))
				}
				q := maybeQ.(*PriorityQueue)
			*/
			if job.Assignee == nil {
				// Only try removing from queue if unassigned!
				q, ok := queues[job.Queue]
				if !ok {
					panic(fmt.Sprintf("DELETE: job %d has queue %s, but queue not found", job.ID, job.Queue))
				}
				//q.mux.Lock()
				q.Delete(job)
				//q.mux.Unlock()
			}
			delete(allJobs, request.ID)
			mux.Unlock()
			logger.Println("DELETE: released lock")
			delete(clientJobs, request.ID)
			logger.Printf("Deleted %d from %s", request.ID, job.Queue)
			conn.Write(responseOk)
		case "abort":
			//TODO don't have a great way to tell if ID is 0 or just missing. Make pointer?
			if request.ID <= 0 {
				sendErrf("abort: bad id")
				continue READLINE
			}
			mux.Lock()
			_, ok := allJobs[request.ID]
			mux.Unlock()
			if !ok {
				// Job does not exist: `{"status":"no-job"}`
				conn.Write(responseNoJob)
				// May be deleted job that was owned by this client. Remove if so.
				delete(clientJobs, request.ID)
				continue READLINE
			}
			job, ok := clientJobs[request.ID]
			if !ok {
				sendErrf("abort: job %d not owned by client", request.ID)
				continue READLINE
			}
			mux.Lock()
			// Unset user
			job.Assignee = nil
			delete(clientJobs, job.ID)
			// Return to queue
			/*
				maybeQ, ok := queues.Load(job.Queue)
				if !ok {
					// Shouldn't happen, since queue is always created before job in PUT
					panic(fmt.Sprintf("Queue %s does not exist but job %d specifies it", job.Queue, job.ID))
				}
				q := maybeQ.(*PriorityQueue)
			*/
			q := queues[job.Queue]
			q.HPush(job)
			mux.Unlock()
			logger.Printf("Aborted %d", job.ID)
			conn.Write(responseOk)
		}
	}
	return nil

}

func swapRemove[T any](s []T, i int) []T {
	s[i] = s[len(s)-1]  // copy last element to index i
	return s[:len(s)-1] // truncate slice
}

const port = 3339

func main() {
	//l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	l, err := net.ListenTCP("tcp", &net.TCPAddr{Port: port})
	if err != nil {
		log.Fatalf("Could not listen on port %d: %s", port, err)
	}
	log.Printf("Listening on :%d", port)

	for {
		client, err := l.AcceptTCP()
		if err != nil {
			log.Printf("Couldn't accept connection: %s", err)
			continue
		}
		go handle09(client)
	}
}
