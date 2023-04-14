package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
)

const nine = 9

// demo is an early sanity check for SearchRange prior to writing main()
func demo() {
	vals := map[int32]int32{
		4: 99,
		5: 200,
		6: 80,
		3: 40,
		2: 90,
		1: 800,
	}
	var t *Node
	for k, v := range vals {
		if t == nil {
			t = NewNode(k, v)
		} else {
			t.InsertKeyValue(k, v)
		}
		t.Show()
		fmt.Println("-------")
	}
	t.Show()
	fmt.Println(t.SearchRange(3, 5))
}

func main() {
	//demo()

	log.Println("Listening on :3332")
	l, err := net.Listen("tcp", ":3332")
	if err != nil {
		log.Fatalf("Received error %s", err)
	}
	defer l.Close()

	// Just kick off a handler per-connection. Each maintains its own database.
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
	reader := bufio.NewReader(conn)
	buf := make([]byte, nine)
	var tree *Node
	for {
		v, err := reader.Read(buf)
		switch {
		case err == io.EOF || err == io.ErrUnexpectedEOF:
			log.Println("EOF")
			return
		case err != nil:
			log.Printf("Unexpected error: %s", err)
			return
		}
		if v != nine {
			//TODO do we try to buffer this and wait for more?
			// For now let's just focus on the case where we have 9 bytes
			log.Printf("Expected 9 bytes, got %d", v)
			break
		}
		log.Printf("Got %d bytes", v)
		// parse
		msg, err := ParseMessage(buf)
		if err != nil {
			log.Printf("Couldn't parse message: %s", err)
			break
		}
		log.Printf("RECEIVED %s", msg.Text())
		switch msg.Type {
		case insert:
			if tree == nil {
				tree = NewNode(msg.A, msg.B)
			} else {
				tree.InsertKeyValue(msg.A, msg.B)
			}
		case query:
			if tree == nil {
				// Undefined - just return 0.
				reply(conn, 0)
			} else {
				log.Println("COMPUTING MEAN")
				mean := tree.MeanRange(msg.A, msg.B)
				reply(conn, mean)
			}
		default:
		}
	}
}

func reply(conn net.Conn, mean int32) error {
	log.Printf("REPLY %d", mean)
	return binary.Write(conn, binary.BigEndian, mean)
}
