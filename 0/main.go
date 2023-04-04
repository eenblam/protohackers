package main

import (
	//"fmt"
	"io"
	"log"
	"net"
)

// Literally the example given for net.Listener
// https://pkg.go.dev/net#example-Listener

func main() {
	//l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	l, err := net.Listen("tcp", ":9999")
	dieIf(err)
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Printf("Couldn't accept connection: %s", err)
			continue
		}
		go handle(conn)
	}
}

func handle(conn net.Conn) {
	defer conn.Close()
	a := conn.RemoteAddr().String()
	log.Printf("ACCEPT %s\n", a)
	// Conn supports Read and Write interfaces
	// io.Copy(a, b) does a.WriteTo(b), or b.ReadFrom(a)
	written, err := io.Copy(conn, conn)
	if err != nil {
		log.Printf("ERROR %s %s\n", a, err)
	} else {
		log.Printf("CLOSE %s Wrote %d bytes\n", a, written)
	}
}

func dieIf(err error) {
	if err != nil {
		log.Fatalf("Received error %s\n", err)
	}
}
