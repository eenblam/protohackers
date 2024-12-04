package main

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
)

var Coordinator *ClientCoordinator

// DialLRCP creates a new Session for an LRCP client.
// Like Dial and related functions, `network` must be a valid LRCP network name.
// Currently, "lrcp" and "lrcp4" are supported, but "lrcp6" may not be. ;)
// If laddr is nil, a local address and port are automatically chosen.
func DialLRCP(network string, laddr, raddr *net.UDPAddr) (*Session, error) {
	conn, err := net.DialUDP("udp", laddr, raddr)
	if err != nil {
		return nil, err
	}
	log.Printf("DialLRCP: dialed [%s], listening on [%s]", raddr.String(), conn.LocalAddr().String())
	coordinator := getClientCoordinator()
	session := newClientSession(raddr,
		coordinator.getClientId(conn),
		conn,
		coordinator.quitCh)
	go session.listenClient()
	// Send initial connect before making session available for use
	err = session.sendConnect()
	if err != nil {
		return nil, fmt.Errorf("error sending connect message on dial: %v", err)
	}
	return session, nil
}

func getClientCoordinator() *ClientCoordinator {
	if Coordinator == nil {
		Coordinator = &ClientCoordinator{
			quitCh:       make(chan *Session, 1),
			pool:         &sync.Pool{New: func() any { return &Msg{} }},
			sessionStore: sync.Map{},
		}
		go Coordinator.reapSessions()
	}
	return Coordinator
}

type ClientCoordinator struct {
	// quitCh allows Sessions to indicate they can be safely reaped from the clientSessionStore.
	quitCh chan *Session
	// *Msg pool for incoming messages
	pool *sync.Pool
	// sessionStore is a map of session keys to Sessions.
	sessionStore sync.Map
}

// reapSessions listens for sessions that have quit (for whatever reason) and removes them from the session store.
func (c *ClientCoordinator) reapSessions() {
	for {
		session := <-c.quitCh
		log.Printf(`Coordinator.reapSessions: Session[%s] has quit. Removing from client session store.`, session.Key())
		session.Close()
		session.sendClose()
		c.sessionStore.Delete(session.ID)
		err := session.conn.Close()
		if err != nil {
			log.Printf(`Coordinator.reapSessions: error closing Session[%s]`, session.Key())
		}
	}
}

// getClientId produces an pseudo random session ID (a random integer below 2147483648
// (max LRCP numeric size) that's not yet in use by a session.
// Not at all cryptographically secure, or remotely efficient at high working loads!
// Note that Listener.sessionStore maps Session.Key() so that clients on different IPs
// can create sessions with colliding IDs. That shouldn't be needed by a singular client,
// so we track only ID.
func (c *ClientCoordinator) getClientId(conn *net.UDPConn) (i int) {
	// Numeric field, must be smaller than 2147483648
	for i = rand.Intn(2147483648); ; {
		_, loaded := c.sessionStore.LoadOrStore(i, conn)
		if !loaded {
			return
		}
	}

}
