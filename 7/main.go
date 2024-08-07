package main

import (
	"fmt"
	"log"
	"net"
)

// "LRCP messages must be smaller than 1000 bytes.
// You might have to break up data into multiple data
// messages in order to fit it below this limit."
const maxMessageSize = 999

var (
	localAddr = "0.0.0.0"
	localPort = 4321
)

// TODO add mux? sync.Map?
var sessionStore = make(map[string]*Session)

func main() {
	laddr := &net.UDPAddr{
		IP:   net.ParseIP(localAddr),
		Port: localPort,
		Zone: "",
	}

	//ctx := context.Background()

	l, err := Listen(laddr)
	if err != nil {
		log.Fatalf(`error listening: %s`, err)
	}
	// for { session := l.Accept() }

	session := l.Accept()
	// do something with session
	fmt.Println(session)
	for {
		buf := make([]byte, maxMessageSize)
		n, err := session.Read(buf)
		if err != nil {
			sendClose(session.ID, session.Addr, session.conn)
			log.Fatalf(`error reading from session: %s`, err)
		}
		// do something with msg
		fmt.Println(buf[:n])
	}
}
