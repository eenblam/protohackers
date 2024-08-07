package main

import (
	"fmt"
	"log"
	"net"
)

type Listener struct {
	conn     *net.UDPConn
	acceptCh chan *Session
}

func Listen(laddr *net.UDPAddr) (*Listener, error) {
	// Connect or die
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return nil, fmt.Errorf(`error listening on %s:%d: %s`, localAddr, localPort, err)
	}
	log.Printf(`listening on %s:%d`, laddr.IP, laddr.Port)

	l := &Listener{
		conn:     conn,
		acceptCh: make(chan *Session),
	}
	go l.listen()

	return l, nil
}

func (l *Listener) listen() {
	// Get a packet
	// Parse a message (or don't)
	// New session: Create if CONNECT, otherwise send CLOSE.
	// Not a new session: send DATA to session over buffered channel (send via select; just drop if buffer full)
	// TODO what about ACK? Should we send the whole message to the session, or send the ack'd length over a channel?

	buf := make([]byte, maxMessageSize)
	for {
		// Read a packet
		n, addr, err := l.conn.ReadFrom(buf)
		if err != nil {
			log.Fatalf(`error reading from %s: %s`, addr, err)
		}
		rawMsg := buf[:n]
		log.Printf(`got %d bytes from %s: [%s]`, n, addr.String(), string(rawMsg))

		// Parse a message
		//TODO could use a sync.Pool here to avoid repeated allocations of new Msgs
		parsedMsg, err := parseMessage(rawMsg)
		if err != nil {
			// Just drop invalid messages
			log.Printf(`error parsing message: %s`, err)
			continue
		}

		// Find or create a session (or send a close for a non-CONNECT to an unrecognized session)
		// Sessions are supposedly guaranteed to be unique to IP addresses,
		// but it's easy enough to prevent collisions by including the IP address and port in our key.
		sessionKey := fmt.Sprintf(`%s-%d`, addr.String(), parsedMsg.Session)
		//TODO handle concurrent access to sessionStore!
		session, ok := sessionStore[sessionKey]
		if !ok {
			// Unrecognized session. Create a new session for CONNECT, otherwise just send a close.
			if parsedMsg.Type == `connect` {
				// Persist session
				sessionStore[sessionKey] = NewSession(addr, parsedMsg.Session, l.conn)
				// Send session to be Accept()'d. If this fails, close and drop the session.
				select {
				case l.acceptCh <- sessionStore[sessionKey]:
				default:
					log.Printf(`failed to accept session %s, sending close`, session.Key())
					session.Close()
					sendClose(parsedMsg.Session, addr, l.conn)
					delete(sessionStore, session.Key())
				}
			} else {
				sendClose(parsedMsg.Session, addr, l.conn)
			}
			continue
		}
		switch parsedMsg.Type {
		case `connect`:
			// We've already created a session before, so just ack.
			// (This could be moved to the session on principle, but simplest to keep it here.)
			if err = sendAck(0, addr, l.conn); err != nil {
				log.Printf(`error sending ack to %s: %s`, addr, err)
			}
			continue
		case `close`:
			// Close session and remove from store.
			session.Close()
			sendClose(parsedMsg.Session, addr, l.conn)
			delete(sessionStore, session.Key())
			continue
		case `ack`, `data`:
			// Send data to session.
			// Don't ACK since we may drop packets here.
			select {
			case session.readCh <- parsedMsg:
			default:
				// Do nothing; just drop the packet.
				log.Printf(`dropped packet for session %s`, session.Key())
			}
			continue
		default:
			//TODO log unexpected packet?
		}
	}
}

func (l *Listener) Accept() *Session {
	return <-l.acceptCh
}
