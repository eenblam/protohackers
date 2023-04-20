package main

//package protohackers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
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
}

var ids atomic.Int64

func nextID() int64 { return ids.Add(1) }

var queues = make(map[string]map[int64]*Job) // queue_name => id => Job
var allJobs = make(map[int64]*Job)
var mux sync.Mutex

var responseOk = []byte(`{"status": "ok"}` + "\n")
var responseNoJob = []byte(`{"status": "no-job"}` + "\n")

func handle09(conn *net.TCPConn) error {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
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
		fmt.Fprintf(conn, `{"status": "error", "error": %q}`, msg)
	}
	scanner := bufio.NewScanner(conn)
	clientJobs := make(map[int64]*Job) // Job ID to job
	defer func() {                     // on exit, abort all the jobs this client was working on
		mux.Lock()
		for _, job := range clientJobs {
			if job.Assignee == conn {
				// Only nil out if it's assigned to this conn
				job.Assignee = nil
				logger.Printf("Disconnecting %s. Aborted job %d.", conn.RemoteAddr().String(), job.ID)
			}
		}
		mux.Unlock()
		logger.Println("Disconnected")
	}()
READLINE:

	for scanner.Scan() {
		conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		request, err := fromJSON[struct {
			Request string   `json:"request"` // "put", "get", "delete", "abort"
			Queues  []string `json:"queues"`  // GET only
			Wait    bool     `json:"wait"`    // GET only

			Queue string          `json:"queue"` // PUT only
			Job   json.RawMessage `json:"job"`   // PUT only
			Pri   int             `json:"pri"`   // PUT only

			ID int64 `json:"id"` // DELETE, ABORT only
		}](scanner.Bytes())
		if err != nil {
			sendErrf("invalid request: %v", err)
			continue READLINE
		}
		logger.Println(scanner.Text())
		switch request.Request {
		default:
			sendErrf("invalid request: unknown request type %q", request.Request)
			continue READLINE
		case "put":
			if request.Queue == "" || request.Job == nil || request.Pri < 0 {
				sendErrf("put: missing one or more of queue, job, or pri")
				continue READLINE
			}
			if len(request.Queues) != 0 || request.Wait || request.ID != 0 {
				sendErrf("put: extra fields")
				continue READLINE
			}
			id := nextID()

			mux.Lock()
			if queues[request.Queue] == nil {
				queues[request.Queue] = make(map[int64]*Job)
			}
			job := &Job{Priority: request.Pri, ID: id, Val: request.Job, Assignee: nil, Queue: request.Queue}
			queues[request.Queue][id] = job
			allJobs[id] = job
			mux.Unlock()
			if _, err := fmt.Fprintf(conn, `{"status": "ok", "id":%d}`+"\n", id); err != nil {
				return fmt.Errorf("put: %w", err)
			}

		case "get":
			if request.Queue != "" || request.Job != nil || request.Pri != 0 {
				sendErrf("get: extra fields")
				continue READLINE
			}
			if len(request.Queues) == 0 {
				sendErrf("get: missing field Queues")
				continue READLINE
			}
			// If request.Wait, loop forever until we find a request with sufficient priority.
			// we want the job with the HIGHEST priority in any of the queues
			var maxJobQueue string
			var maxJobID int64
			var maxJobPriority = -1
			var job *Job
			for maxJobPriority == -1 {
				// Unlock after each check to allow jobs to be added,
				// otherwise no one will be able to add a job for us to assign.
				mux.Lock()
				for _, k := range request.Queues {
					for _, j := range queues[k] {
						if j.Assignee != nil {
							continue
						}
						if j.Priority > maxJobPriority {
							maxJobPriority = j.Priority
							maxJobQueue = k
							maxJobID = j.ID
						}
					}
				}
				// If max found, assign to client and break
				if maxJobPriority > -1 {
					// we found a job
					job = queues[maxJobQueue][maxJobID]
					// assign to current client
					job.Assignee = conn
					clientJobs[job.ID] = job
					mux.Unlock()
					goto ASSIGNED
				}
				// Not found, so unlock so someone can add to the Queue.
				mux.Unlock()
				// If not waiting, send responseNoJob and listen for new request
				if !request.Wait {
					// Have to loop once before trying this
					if _, err := conn.Write(responseNoJob); err != nil {
						return fmt.Errorf("get: %s", err) // client disconnected
					}
					logger.Println("response no-job")
					continue READLINE
				}
				// Waiting, so just loop around again.
			}

		ASSIGNED:
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
				log.Printf("get: %s", err)
				return fmt.Errorf("get: %s", err) // client disconnected
			}
		case "delete":
			if request.ID <= 0 { // can't be zero, since the first call to nextID() returns 1
				sendErrf("delete: bad id")
				continue READLINE
			}
			j, ok := allJobs[request.ID]
			if !ok {
				logger.Printf("delete: id %d not found", request.ID)
				conn.Write([]byte(responseNoJob))
				continue READLINE
			}
			mux.Lock()
			delete(queues[j.Queue], request.ID)
			delete(allJobs, request.ID)
			delete(clientJobs, request.ID)
			mux.Unlock()
			conn.Write(responseOk)
		case "abort":
			//TODO don't have a great way to tell if ID is 0 or just missing. Make pointer?
			if request.ID <= 0 {
				sendErrf("abort: bad id")
				continue READLINE
			}
			_, ok := allJobs[request.ID]
			if !ok {
				// Job does not exist: `{"status":"no-job"}`
				conn.Write(responseNoJob)
				continue READLINE
			}
			job, ok := clientJobs[request.ID]
			if !ok {
				sendErrf("abort: job %d not owned by client", request.ID)
				continue READLINE
			}
			// Unset user
			mux.Lock()
			job.Assignee = nil
			delete(clientJobs, job.ID)
			mux.Unlock()
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
