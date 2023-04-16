package main

import (
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
	logger := log.New(log.Writer(), conn.RemoteAddr().String(), log.Flags()|log.Lshortfile)
	buf := make([]byte, nine)
	var tree *Node
	for {
		_, err := io.ReadFull(conn, buf)
		switch {
		case err == io.ErrUnexpectedEOF:
			logger.Println("EOF")
			return
		case err != nil:
			logger.Printf("Unexpected error: %s", err)
			return
		}
		// parse
		kind, a, b, err := Parse(buf)
		if err != nil {
			logger.Printf("Couldn't parse message: %s", err)
			return
		}
		logger.Printf("RECEIVED %c %d %d", kind, a, b)
		switch kind {
		case 'I':
			if tree == nil {
				tree = NewNode(a, b)
			} else {
				tree.InsertKeyValue(a, b)
			}
		case 'Q':
			if tree == nil {
				// Undefined - just return 0.
				log.Printf("REPLY %d", 0)
				binary.Write(conn, binary.BigEndian, 0)
			} else {
				logger.Println("COMPUTING MEAN")
				mean := tree.MeanRange(a, b)
				log.Printf("REPLY %d", mean)
				binary.Write(conn, binary.BigEndian, mean)
			}
		default:
		}
	}
}

func Parse(bs []byte) (kind byte, a, b int32, err error) {
	if len(bs) != 9 {
		return 0, 0, 0, fmt.Errorf("Expected 9 bytes, got %d", len(bs))
	}
	// Parse message type
	switch bs[0] {
	case 'I', 'Q':
		a = int32(binary.BigEndian.Uint32(bs[1:5]))
		b = int32(binary.BigEndian.Uint32(bs[5:9]))
		return bs[0], a, b, nil
	default:
		return 0, 0, 0, fmt.Errorf("Want I or Q, got %x", bs[0])
	}
}
