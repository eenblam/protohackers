package main

import (
	"fmt"
	"log"
	"net"
	"sync"
)

type Listener struct {
	conn *net.UDPConn
	// acceptCh syncronizes Accept() with the listen() goroutine.
	acceptCh chan *Session
	// quitCh allows Sessions to an indicate they can be safely reaped from the sessionStore.
	quitCh chan *Session
	// *Msg pool for incoming messages
	pool *sync.Pool
	// sessionStore is a map of session keys to sessions.
	sessionStore sync.Map
}

func Listen(laddr *net.UDPAddr) (*Listener, error) {
	// Listen or die
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return nil, fmt.Errorf(`error listening on %s:%d: %s`, localAddr, localPort, err)
	}
	log.Printf(`listening on %s:%d`, laddr.IP, laddr.Port)

	l := &Listener{
		conn:     conn,
		acceptCh: make(chan *Session, 1),
		quitCh:   make(chan *Session),
		pool:     &sync.Pool{New: func() any { return &Msg{} }},
	}
	go l.reapSessions()
	go l.listen()

	return l, nil
}

// reapSessions listens for sessions that have quit (for whatever reason) and removes them from the session store.
func (l *Listener) reapSessions() {
	for {
		session := <-l.quitCh
		log.Printf(`Listener: Session[%s] has quit. Removing from session store.`, session.Key())
		session.Close()
		sendClose(session.ID, session.Addr, l.conn)
		l.sessionStore.Delete(session.Key())
	}
}

// listen is the core read loop for all incoming packets, demux'ing them to their respective sessions
// and creating new sessions as needed.
func (l *Listener) listen() {
	// Get a packet; parse a message (or don't)
	// New session: Create if CONNECT, otherwise send CLOSE.
	// Not a new session: send ACK and DATA to session over buffered channel (send via select; just drop if buffer full)

	buf := make([]byte, maxMessageSize)
	for {
		// Read a packet
		n, addr, err := l.conn.ReadFrom(buf)
		if err != nil {
			log.Printf(`Listener: error reading from %s: %s`, addr.String(), err)
			continue
		}
		rawMsg := buf[:n]
		log.Printf(`Listener: got %d bytes from %s: [%s]`, n, addr.String(), string(rawMsg))

		// Parse a message; pull from pool since we'd otherwise be allocating a lot of these.
		parsedMsg := l.pool.Get().(*Msg)
		if err = parseMessageInto(parsedMsg, rawMsg); err != nil {
			// Just drop invalid messages
			log.Printf(`Listener: error parsing message: [%s]`, err)
			continue
		}

		// Find or create a session (or send a close for a non-CONNECT to an unrecognized session)
		// Sessions are supposedly guaranteed to be unique to IP addresses,
		// but it's easy enough to prevent collisions by including the IP address and port in our key.
		var session *Session
		if parsedMsg.Type == `connect` {
			// Create pre-load to keep critical section as small as possible.
			// (Alternative is a longer mutex lock to load, create, then store.
			// The downside with current approach is creating a session for redundant CONNECTs.)
			newSession := newServerSession(addr, parsedMsg.Session, l.conn, l.pool, l.quitCh)
			loadedSession, loaded := l.sessionStore.LoadOrStore(newSession.Key(), newSession)
			if loaded {
				// Existing session. Close the new one and proceed.
				newSession.Close()
				session = loadedSession.(*Session)
			} else {
				// *loadedSession == *newSession. Send to accept channel. Tear down if we can't.
				session = newSession
				select {
				case l.acceptCh <- session:
					log.Printf(`Listener: accepted session [%s]`, session.Key())
				default:
					log.Printf(`Listener: failed to accept session [%s]`, session.Key())
					// Close session and remove from store.
					// Don't ack since we dropped. Don't *send* a CLOSE so peer can retry.
					session.Close()
					l.sessionStore.Delete(session.Key())
					continue
				}
			}
			// Regardless, nothing more to do here but send an ACK. If this fails, they can always retry the CONNECT.
			if err = session.sendAck(0); err != nil {
				log.Printf(`Listener: error sending ack to %s: %s`, addr, err)
			}
			continue
		} else {
			// Not a connect. Try to load. Continue on failure.
			loadedSession, loaded := l.sessionStore.Load(fmt.Sprintf("%s-%d", addr, parsedMsg.Session))
			if !loaded {
				sendClose(parsedMsg.Session, addr, l.conn)
				continue
			}
			session = loadedSession.(*Session)
		}
		switch parsedMsg.Type {
		case `connect`:
			log.Printf(`Listener: unexpected handling of connect message for session [%s]; this should be unreachable`, session.Key())
			continue
		case `close`:
			// Close session and remove from store.
			log.Printf(`Listener: peer disconnect; closing session [%s]`, session.Key())
			session.Close()
			sendClose(parsedMsg.Session, addr, l.conn)
			l.sessionStore.Delete(session.Key())
			continue
		case `ack`, `data`:
			// Send ACK and DATA to session.
			// Don't acknowledge DATA yet, since we may drop packets here.
			select {
			case session.receiveCh <- parsedMsg:
			default:
				// Do nothing; just drop the packet.
				log.Printf(`Listener: dropped packet for session %s`, session.Key())
				l.pool.Put(parsedMsg)
			}
			continue
		default:
			log.Printf(`Listener: unexpected packet type [%s] for session [%s]`, parsedMsg.Type, session.Key())
		}
	}
}

// Accept blocks until a new Session is available, then returns it.
func (l *Listener) Accept() *Session {
	return <-l.acceptCh
}
