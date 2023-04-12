package main

import (
	"bufio"
	"io"
	"log"
	"net"
)

const malformed = `¯\_(ツ)_/¯`

func main() {
	// They hit me with 321631
	// Hopefully I can just pre-compute high enough that I don't need to dynamically grow the sieve
	n := 100000000
	s, err := NewSieve(n)
	if err != nil {
		log.Fatalf("Couldn't generate to %d: %s", n, err)
	}

	log.Println("Listening on :3333")
	l, err := net.Listen("tcp", ":3333")
	if err != nil {
		log.Fatalf("Received error %s", err)
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Printf("Couldn't accept connection: %s", err)
			continue
		}
		go handle(conn, s)
	}
}

func handle(conn net.Conn, s *Sieve) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	connected := true
	for connected {
		readbuf, err := reader.ReadBytes('\n')
		switch {
		case err == io.EOF || err == io.ErrUnexpectedEOF:
			log.Println("")
			connected = false
			continue
		case err != nil:
			log.Printf("Unexpected error: %s", err)
			continue
		default:
			// Do nothing
		}

		// Unpack line into Request
		log.Printf("REQUEST: %s", string(readbuf))
		request, err := UnwrapRequest(readbuf)
		if err != nil {
			fail(conn, "Couldn't unmarshal JSON", string(readbuf))
			break
		}

		// Malformed?
		if !isValid(*request) {
			fail(conn, "Invalid request", string(readbuf))
			break
		}
		// Well-formed:
		// Float?
		if request.Float {
			log.Println("Float is false")
			conn.Write([]byte(`{"method":"isPrime","prime":false}` + "\n"))
			continue
		}
		prime, err := s.IsPrime(request.Number)
		if err != nil {
			// This probably happened because we haven't computed this high
			fail(conn, err.Error(), string(readbuf))
			break
		}
		if prime {
			log.Printf("Prime: %d", request.Number)
			conn.Write([]byte(`{"method":"isPrime","prime":true}` + "\n"))
		} else {
			log.Printf("Not prime: %d", request.Number)
			conn.Write([]byte(`{"method":"isPrime","prime":false}` + "\n"))
		}
	}
}

// fail lets an offending client know its input was malformed.
func fail(conn net.Conn, errMessage string, buffer string) error {
	a := conn.RemoteAddr().String()
	log.Printf("ERROR %s %s: %s", a, errMessage, buffer)
	_, err := conn.Write([]byte(malformed + "\n"))
	// We return this error (or nil)... but doesn't matter really.
	// If we called fail(), then the Conn should be closed anyway.
	return err
}
