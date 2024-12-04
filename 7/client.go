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
		coordinator.cleanup)
	go coordinator.listen(session)
	// Send initial connect before making session available for use
	err = session.SendConnect()
	if err != nil {
		return nil, fmt.Errorf("error sending connect message on dial: %v", err)
	}
	return session, nil
}

func getClientCoordinator() *ClientCoordinator {
	if Coordinator == nil {
		Coordinator = &ClientCoordinator{
			pool:         &sync.Pool{New: func() any { return &Msg{} }},
			sessionStore: sync.Map{},
		}
	}
	return Coordinator
}

type ClientCoordinator struct {
	// *Msg pool for incoming messages
	pool *sync.Pool
	// sessionStore is a map of session keys to Sessions.
	sessionStore sync.Map
}

// cleanup is a callback for sessions that have quit (for whatever reason).
func (c *ClientCoordinator) cleanup(session *Session) {
	log.Printf(`Coordinator.reapSessions: Session[%s] has quit. Removing from client session store.`, session.Key())
	c.sessionStore.Delete(session.ID)
	// Unlike Sessions spawned by a Listener, these each have their own underlying net.UDPConn.
	err := session.conn.Close()
	if err != nil {
		log.Printf(`Coordinator.reapSessions: error closing Session[%s]`, session.Key())
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

// listen is the core listen loop for a single client-only session, since it isn't
// being managed by a server Listener.
func (c *ClientCoordinator) listen(s *Session) {
	buf := make([]byte, maxMessageSize)
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Read a packet
		n, err := s.conn.Read(buf)
		if err != nil {
			log.Printf(`Client[%s].listen: error reading: %v`, s.Key(), err)
			continue
		}
		rawMsg := buf[:n]
		log.Printf(`Client[%s].listen: got %d bytes`, s.Key(), n)

		// Parse a message; pull from pool since we'd otherwise be allocating a lot of these.
		parsedMsg, err := parseMessage(rawMsg)
		if err != nil {
			// Just drop invalid messages
			log.Printf(`Client[%s].listen: error parsing message: [%v]`, s.Key(), err)
			continue
		}
		if parsedMsg.Session != s.ID {
			log.Printf(`Client[%s].listen: got [%s] for session [%d], expected [%d]`, s.Key(), parsedMsg.Type, parsedMsg.Session, s.ID)
			s.Close()
			return
		}
		log.Printf(`Client[%s].listen: got %d bytes of type [%s]`, s.Key(), n, parsedMsg.Type)

		switch parsedMsg.Type {
		case `connect`:
			// For now, we aren't supporting 1-1 connections, so just close.
			log.Printf(`Client[%s].listen: unexpected connect from server`, s.Key())
			s.Close()
		case `close`:
			log.Printf(`Client[%s].listen: peer disconnect; closing`, s.Key())
			// Send a Close msg if we *haven't* already closed ourselves
			s.Close()
		case `ack`, `data`:
			// Forward ACK and DATA to session.
			// Don't acknowledge DATA yet, since we may drop packets here.
			err = s.Receive(parsedMsg)
			if err != nil {
				// Do nothing; just drop the packet.
				log.Printf(`Client[%s].listen: dropped packet: %v`, s.Key(), err)
			}
		default:
			log.Printf(`Client[%s].listen: unexpected packet type [%s]`, s.Key(), parsedMsg.Type)
		}
	}
}
