package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"
)

const port = 3333

var NameRegexp = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

func main() {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("Could not listen on port %d: %s", port, err)
	}
	log.Printf("Listening on :%d", port)

	b := &Broker{
		Users: make(map[string][]string, 0),
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Printf("Couldn't accept connection: %s", err)
			continue
		}
		go handle(conn, b)
	}
}

func handle(conn net.Conn, b *Broker) {
	defer conn.Close()
	logger := log.New(log.Writer(),
		fmt.Sprintf("[%s] ", conn.RemoteAddr().String()),
		log.Flags()|log.Lmsgprefix|log.Lshortfile)
	scanner := bufio.NewScanner(conn)

	_, err := conn.Write([]byte("Welcome to budgetchat! What shall I call you?\n"))
	if err != nil {
		logger.Printf("Failed to ask for client name: %s", err)
		return
	}
	// Get a line for name.
	gotSomething := scanner.Scan()
	if !gotSomething {
		logger.Println("Couldn't scan name from client")
		return
	}
	// Get string version of name, if acceptable
	name, err := Validate(scanner.Bytes())
	if err != nil {
		text := fmt.Sprintf("Invalid name: %s", err)
		conn.Write([]byte(text))
		logger.Println(text)
		return
	}
	// Register and get users active prior to registration
	err = b.Register(name)
	if err != nil {
		text := fmt.Sprintf("Name %s already in use", name)
		conn.Write([]byte(text))
		logger.Println(text)
		return
	}
	defer b.Logoff(name)

	logger = log.New(log.Writer(),
		fmt.Sprintf("[%s:%s] ", conn.RemoteAddr().String(), name),
		log.Flags()|log.Lmsgprefix|log.Lshortfile)
	// Defering from here to use updated logger
	defer logger.Println("Logging off")
	logger.Println("Joined")

	// Listen for:
	// * messages from user
	// * messages to user
	// * exit signal

	// What about a separate goroutine?
	// One reads from scanner until failure, then does a Send
	// One waits to Receive from queue, then sends to user

	//TODO Not sure if this should come from main() or start here
	ctx, cancelCtx := context.WithCancel(context.TODO())
	go func(ctx context.Context, cancelCtx context.CancelFunc, logger *log.Logger, conn net.Conn, b *Broker) {
		for {
			select {
			case <-ctx.Done():
				// conn writer hit an error. Assume cleanup there.
				logger.Println("Reader done")
				return
			default:
				// try to read and send
				//logger.Println("Reader scanning")
				gotSomething := scanner.Scan()
				//TODO handle bool, check scanner.Err() and whatnot
				if !gotSomething {
					if err := scanner.Err(); err != nil {
						logger.Printf("Unexpected error scanning: %s", err)
					} else {
						logger.Println("Reader's scanner quit")
					}
					cancelCtx()
					return
				}
				txt := scanner.Text()
				logger.Println(txt)
				b.Send(name, txt)
			}
		}
	}(ctx, cancelCtx, logger, conn, b)

	// Receive from queue and send to client
	for {
		select {
		case <-ctx.Done():
			// Just leave. Cleanup via defer.
			logger.Println("Writer done")
			return
		default:
			// Receive from queue and send to client
			//logger.Println("Writer popping")
			msg, empty := b.Receive(name)
			if !empty {
				_, err := conn.Write([]byte(msg))
				if err != nil {
					logger.Printf("%s: Error writing to client: %s", name, err)
					cancelCtx()
					return
				}
			}
		}
	}
}

func Validate(rawName []byte) (string, error) {
	// coerce to string, strip whitespace
	name := strings.TrimSpace(string(rawName))
	// at least 1 character
	if len(name) == 0 {
		return "", errors.New("Received name of length 0")
	}
	// must be no more than 16 characters
	if len(name) > 16 {
		return "", fmt.Errorf("Name length must be at most 16 characters, got %d", len(name))
	}
	// must consist only of alphanumerics (upper, lower, digit)
	if NameRegexp.MatchString(name) {
		return name, nil
	}
	return "", fmt.Errorf("Expected 1-16 ASCII upper, lower, and digit characters. Got %s", name)
}

type Broker struct {
	mx sync.RWMutex
	// channel to receive (name, message)
	Users map[string][]string
}

// Register registers the name if available, or returns an error if not.
func (b *Broker) Register(name string) error {
	b.mx.Lock()
	defer b.mx.Unlock()
	if b.Users[name] != nil {
		// Already registered
		return fmt.Errorf("User %s already exists", name)
	}
	// Create list of already active users before adding, return to user
	active := make([]string, len(b.Users))
	i := 0
	// This shouldn't take us out of bounds. Length fixed thanks to mutex.
	for key, queue := range b.Users {
		active[i] = key
		b.Users[key] = append(queue, fmt.Sprintf("* %s has entered the room\n", name))
		i++
	}
	activeUsers := fmt.Sprintf("* The room contains: %s\n", strings.Join(active, ", "))
	queue := []string{activeUsers}
	b.Users[name] = queue
	return nil
}

// Returns msg,false on msg, else "",true on empty
//
// This is abusing the read unlock since we're technically
// modifying the underlying data structure, but only one
// goroutine (the user's) should ever call Receive(name)
// to "read" for a given user.
// The result is that users can pop from their queue
// without waiting for other reads, but sends require an
// exclusive lock. (like user registration and deletion)
func (b *Broker) Receive(name string) (string, bool) {
	//b.mx.RLock()
	//defer b.mx.RUnlock()
	b.mx.Lock()
	defer b.mx.Unlock()
	queue := b.Users[name]
	if len(queue) == 0 {
		return "", true
	}
	// pop from queue
	message := queue[0]
	b.Users[name] = queue[1:]
	return message, false
}

// Sends <message> to every user except <name>
func (b *Broker) Send(name string, message string) {
	b.mx.Lock()
	defer b.mx.Unlock()
	out := fmt.Sprintf("[%s] %s\n", name, message)
	for userName, queue := range b.Users {
		if name == userName {
			// Don't send to self
			continue
		}
		b.Users[userName] = append(queue, out)
	}
}

// Logoff removes name from the Users map
func (b *Broker) Logoff(name string) {
	b.mx.Lock()
	defer b.mx.Unlock()
	delete(b.Users, name)
	// Tell everyone that <name> has left
	message := fmt.Sprintf("* %s has left the room\n", name)
	for userName, queue := range b.Users {
		b.Users[userName] = append(queue, message)
	}
}
