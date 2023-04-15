package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
)

const port = 3333

func main() {
	// They hit me with 321631
	// Hopefully I can just pre-compute high enough that I don't need to dynamically grow the sieve
	n := 100000000
	s, err := NewSieve(n)
	if err != nil {
		log.Fatalf("Couldn't generate to %d: %s", n, err)
	}

	log.Printf("Listening on :%d", port)
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
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
	//reader := bufio.NewReader(conn)
	scanner := bufio.NewScanner(conn)
	for {
		for scanner.Scan() {
			// Unpack line into Request
			got := scanner.Bytes()
			log.Printf("REQUEST: %s", string(got))
			request, err := UnwrapRequest(got)
			if err != nil {
				fail(conn, "Couldn't unmarshal JSON", string(got))
				break
			}

			// Float?
			if request.Float {
				log.Println("Float is false")
				conn.Write([]byte(`{"method":"isPrime","prime":false}` + "\n"))
				continue
			}
			prime, err := s.IsPrime(request.Number)
			if err != nil {
				// This probably happened because we haven't computed this high
				fail(conn, err.Error(), string(got))
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
		if err := scanner.Err(); err != nil {
			log.Printf("Unexpected error: %s", err)
			return
		}
	}
}

// fail lets an offending client know its input was malformed.
func fail(conn net.Conn, errMessage string, buffer string) error {
	a := conn.RemoteAddr().String()
	log.Printf("ERROR %s %s: %s", a, errMessage, buffer)
	_, err := conn.Write([]byte(`¯\_(ツ)_/¯` + "\n"))
	// We return this error (or nil)... but doesn't matter really.
	// If we called fail(), then the Conn should be closed anyway.
	return err
}
