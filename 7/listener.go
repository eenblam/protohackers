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
	// timeoutCh allows sessions to signal that their peer has timed out.
	timeoutCh chan *Session
	// *Msg pool for incoming messages
	pool *sync.Pool
}

func Listen(laddr *net.UDPAddr) (*Listener, error) {
	// Connect or die
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return nil, fmt.Errorf(`error listening on %s:%d: %s`, localAddr, localPort, err)
	}
	log.Printf(`listening on %s:%d`, laddr.IP, laddr.Port)

	l := &Listener{
		conn:      conn,
		acceptCh:  make(chan *Session),
		timeoutCh: make(chan *Session),
		pool:      &sync.Pool{New: func() any { return &Msg{} }},
	}
	go l.listen()

	return l, nil
}

func (l *Listener) listen() {
	// Get a packet
	// Parse a message (or don't)
	// New session: Create if CONNECT, otherwise send CLOSE.
	// Not a new session: send ACK and DATA to session over buffered channel (send via select; just drop if buffer full)

	sessionStore := make(map[string]*Session)
	buf := make([]byte, maxMessageSize)
	for {
		// Handle any timed-out sessions
		select {
		case session := <-l.timeoutCh:
			log.Printf(`session [%s] timed out`, session.Key())
			session.Close()
			sendClose(session.ID, session.Addr, l.conn)
			delete(sessionStore, session.Key())
		default:
		}

		// Read a packet
		n, addr, err := l.conn.ReadFrom(buf)
		if err != nil {
			log.Fatalf(`error reading from %s: %s`, addr, err)
		}
		rawMsg := buf[:n]
		log.Printf(`got %d bytes from %s: [%s]`, n, addr.String(), string(rawMsg))

		// Parse a message; pull from pool since we'd otherwise be allocating a lot of these.
		parsedMsg := l.pool.Get().(*Msg)
		if err = parseMessageInto(parsedMsg, rawMsg); err != nil {
			// Just drop invalid messages
			log.Printf(`error parsing message: %s`, err)
			continue
		}

		// Find or create a session (or send a close for a non-CONNECT to an unrecognized session)
		// Sessions are supposedly guaranteed to be unique to IP addresses,
		// but it's easy enough to prevent collisions by including the IP address and port in our key.
		sessionKey := fmt.Sprintf(`%s-%d`, addr.String(), parsedMsg.Session)
		session, ok := sessionStore[sessionKey]
		if !ok {
			// Unrecognized session. Create a new session for CONNECT, otherwise just send a close.
			if parsedMsg.Type == `connect` {
				// Persist session
				sessionStore[sessionKey] = NewSession(addr, parsedMsg.Session, l.conn, l.pool, l.timeoutCh)
				// Send session to be Accept()'d. If this fails, close and drop the session.
				select {
				case l.acceptCh <- sessionStore[sessionKey]:
					// On success, send ACK
					if err = sessionStore[sessionKey].sendAck(0); err != nil {
						log.Printf(`error sending ack to %s: %s`, addr, err)
					}
				default:
					log.Printf(`failed to accept session %s, sending close`, sessionKey)
					session.Close()
					sendClose(parsedMsg.Session, addr, l.conn)
					delete(sessionStore, sessionKey)
				}
			} else {
				log.Printf(`unrecognized session [%s]; sending close`, sessionKey)
				sendClose(parsedMsg.Session, addr, l.conn)
			}
			continue
		}
		switch parsedMsg.Type {
		case `connect`:
			// We've already created a session before, so just ack.
			// (This could be moved to the session on principle, but simplest to keep it here.)
			if err = session.sendAck(0); err != nil {
				log.Printf(`error sending ack to %s: %s`, addr, err)
			}
			continue
		case `close`:
			// Close session and remove from store.
			log.Printf(`peer disconnect; closing session [%s]`, session.Key())
			session.Close()
			sendClose(parsedMsg.Session, addr, l.conn)
			delete(sessionStore, session.Key())
			continue
		case `ack`, `data`:
			// Send data to session.
			// Don't ACK since we may drop packets here.
			select {
			case session.receiveCh <- parsedMsg:
			default:
				// Do nothing; just drop the packet.
				log.Printf(`dropped packet for session %s`, session.Key())
			}
			continue
		default:
			log.Printf(`unexpected packet type [%s] for session [%s]`, parsedMsg.Type, session.Key())
		}
	}
}

func (l *Listener) Accept() *Session {
	return <-l.acceptCh
}
