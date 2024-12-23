package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// How long to wait before retransmitting unacknowledged data messages.
// "retransmission timeout: the time to wait before retransmitting a message.
// Suggested default value: 3 seconds."
const RetransmissionTimeout = 500 * time.Millisecond

// How long to wait for a new message before timing out.
// "session expiry timeout: the time to wait before accepting that a peer has disappeared,
// in the event that no responses are being received. Suggested default value: 60 seconds."
const ReadTimeout = 60 * time.Second

// Size of Session's receive buffer. Tuned to the rates at which Listener.listen
// can process incoming packets and Session.readWorker can process a Msg.
// Running TestBadLink with only 1% failure rate (near-full throughput):
// 1 => ~8s
// 16 => ~0.5-1.5s
// At 25% failure:
// 1 => ~16s
// 16 => ~7s
// I didn't test super rigorously yet, but it seemed to fall off around there on my machine.
const ReceiveBufferSize = 16

type Session struct {
	// Synchronizes Session.Read and Session.readWorker
	readLock sync.Mutex
	// Synchronizes Session.Write and Session.writeWorker
	writeLock sync.Mutex
	// Eliminates a race condition on Close
	closeLock sync.Mutex

	// The peer's address.
	Addr net.Addr
	// The session's unique ID used in LRCP messages (e.g. SESSION in /data/SESSION/POS/DATA/).
	ID int

	// The UDP connection to send messages on.
	// Incoming messages are de-muxed by the listener.
	conn *net.UDPConn

	// Context for closing the session.
	ctx    context.Context
	cancel context.CancelFunc

	// cleanup is a callback function for the session to call when closed.
	cleanup func(s *Session)

	// receiveCh is a channel to receive messages from the listener
	receiveCh chan *Msg
	// readCh signals that data is available for reading.
	// This channel should be buffered to allow .Read and .readWorker to communicate without blocking.
	readCh chan bool

	// readBuffer is the session's received data.
	readBuffer []byte
	// readIndex is the index of the next byte to read from the session data. Used to implement io.Reader.
	readIndex int64
	// lastAck is the length that was last acknowledged by the peer.
	// atomic.Int32 used to allow lock-free access and modification.
	// (Int32 works since ints must be smaller than 2147483648=2^31.)
	// Since this is signed, -1 indicates a client awaiting an ack (0) of its connect message.
	lastAck atomic.Int32

	// maxAckable is the maximum length we will accept an ack for.
	maxAckable atomic.Int32

	// writeBuffer is the session's data to be sent.
	writeBuffer []byte

	// isClient distinguishes server and client sessions
	isClient bool
}

// newServerSession instantiates the state needed to handle an LRCP session and kicks off read and write workers.
func newServerSession(addr net.Addr, id int, conn *net.UDPConn, cleanup func(s *Session)) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		Addr:        addr,
		ID:          id,
		conn:        conn,
		cleanup:     cleanup,
		receiveCh:   make(chan *Msg, ReceiveBufferSize),
		readCh:      make(chan bool, 1),
		ctx:         ctx,
		cancel:      cancel,
		readBuffer:  make([]byte, 0, 1024),
		writeBuffer: make([]byte, 0, 1024),
		isClient:    false,
	}
	go s.readWorker()
	go s.writeWorker()
	return s
}

// newClientSession instantiates the state needed to handle an LRCP session and kicks off read and write workers.
func newClientSession(addr net.Addr, id int, conn *net.UDPConn, cleanup func(s *Session)) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		Addr:        addr,
		ID:          id,
		conn:        conn,
		cleanup:     cleanup,
		receiveCh:   make(chan *Msg, ReceiveBufferSize),
		readCh:      make(chan bool, 1),
		ctx:         ctx,
		cancel:      cancel,
		readBuffer:  make([]byte, 0, 1024),
		writeBuffer: make([]byte, 0, 1024),
		isClient:    true,
	}
	// We're still waiting for ack 0 while attempting to connect
	s.lastAck.Store(-1)
	go s.readWorker()
	go s.writeWorker()
	return s
}

// Key returns the string key of the session for lookup and logging.
func (s *Session) Key() string {
	return fmt.Sprintf("%s-%d", s.Addr, s.ID)
}

// Read implements the io.Reader interface on the session's data buffer.
func (s *Session) Read(b []byte) (int, error) {
	select {
	case <-s.ctx.Done():
		// If we're closed AND we've read all the data, return EOF.
		s.readLock.Lock()
		defer s.readLock.Unlock()
		if s.readIndex >= int64(len(s.readBuffer)) {
			return 0, io.EOF
		}
		// Otherwise, proceed as normal. It's fine to read from a closed session.
	case <-s.readCh:
		// Data is available for reading.
		s.readLock.Lock()
		defer s.readLock.Unlock()
	}
	if s.readIndex >= int64(len(s.readBuffer)) {
		// A read was signaled, but there's nothing to copy out
		return 0, nil
	}
	n := copy(b, s.readBuffer[s.readIndex:])
	s.readIndex += int64(n)
	return n, nil
}

// appendRead appends incoming data to the session, returning final length of all written data and an error.
// Error is non-nil if pos is invalid, exceeds length of previously received data, or exceeds max transmission size.
func (s *Session) appendRead(pos int, b []byte) (int, error) {
	s.readLock.Lock()
	defer s.readLock.Unlock()
	// This one is tricky! Do we want to allow appending to a closed session?
	// i.e. do we continue to accept data after the session is closed?
	// This could be due to packets arriving out of order, and close beats out previously sent data.
	// On the other hand, if they've sent a close, it's reasonable to assume their last packet has been ACK'd.
	select {
	case <-s.ctx.Done():
		return len(s.readBuffer), fmt.Errorf("session %s is closed", s.Key())
	default:
	}

	if pos < 0 {
		return len(s.readBuffer), fmt.Errorf("invalid position %d < 0", pos)
	}
	if pos != len(s.readBuffer) {
		return len(s.readBuffer), fmt.Errorf("position %d != current data length %d", pos, len(s.readBuffer))
	}
	if total := pos + len(b); total > maxInt {
		return len(s.readBuffer), fmt.Errorf("total data length %d exceeds max transmission size %d", total, maxInt)
	}
	log.Printf("Session[%s].appendRead: appending %d-bytes at pos %d for total %d", s.Key(), len(b), pos, pos+len(b))
	s.readBuffer = append(s.readBuffer, b...)
	return len(s.readBuffer), nil
}

// Write data to the buffer, returning number of bytes written and an error.
// Currently errors if the total data length would exceed maxInt.
func (s *Session) Write(b []byte) (int, error) {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()
	select {
	case <-s.ctx.Done():
		// No point in writing to a closed session.
		return len(s.writeBuffer), fmt.Errorf("session %s is closed", s.Key())
	default:
	}
	total := len(s.writeBuffer) + len(b)
	if total > maxInt {
		return len(s.writeBuffer), fmt.Errorf("total data length %d exceeds max transmission size %d", total, maxInt)
	}
	s.writeBuffer = append(s.writeBuffer, b...)
	return len(b), nil
}

// Abort closes a Session's goroutines without notifying its peer or cleaning
// up resources (see Session.Close().) Useful when a Session has been spawned
// but should be discarded before use.
func (s *Session) Abort() {
	// Don't compete for Close lock. That race condition is actually acceptable
	// here; if the Session internally decides to Close, we shouldn't try to
	// prevent that by acquiring the lock first.
	s.cancel()
}

// Close current session. Can be safely called multiple times.
// Signals other goroutines to close, informs peer of disconnect, and signals
// Listener to reap this session.
//
// A few things are handled by s.cleanup that shouldn't be handled here, since
// they vary by client and server implementation.
// * Removing the session from the session store
// * Closing the session's connection (server shares one conn, clients get their own)
func (s *Session) Close() {

	// Needed for a race condition: it's possible for two calls to Close to enter the default
	// case before one completes a call to s.cancel().
	s.closeLock.Lock()
	defer s.closeLock.Unlock()

	// Only run cleanup once - if we haven't canceled yet.
	select {
	case <-s.ctx.Done():
	default:
		// This needs to be inside the select.
		s.cancel()
		s.SendClose()
		// cleanup must be last since we can't sendClose if the UDPConn is cleaned up.
		s.cleanup(s)
	}

}

// readWorker is a per-session goroutine that receive messages, appends their
// data to the session's readBuffer, and signals to Session.Read that data is
// available.
func (s *Session) readWorker() {
	timeoutTimer := time.NewTimer(ReadTimeout)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timeoutTimer.C:
			log.Printf(`Session[%s].readWorker: no reply from peer; alerting timeout`, s.Key())
			s.Close()
			return
		case msg := <-s.receiveCh:
			// Reset session timeout
			if !timeoutTimer.Stop() { // Must Stop timer and drain the channel before a Reset
				<-timeoutTimer.C
			}
			timeoutTimer.Reset(ReadTimeout)

			switch msg.Type {
			case `ack`:
				// If the ack'd length is greater than what we've sent, close the session.
				maxAckable := int(s.maxAckable.Load())
				if msg.Length > maxAckable {
					log.Printf(`Session[%s].readWorker: peer ack length [%d] greater than maxAckable [%d]; closing session`, s.Key(), msg.Length, maxAckable)
					s.Close()
					return
				}

				// As long as ack'd length > session.lastAck, try to update session.lastAck
				for {
					lastAck := s.lastAck.Load()
					if msg.Length > int(lastAck) {
						if s.lastAck.CompareAndSwap(lastAck, int32(msg.Length)) { // success
							break
						}
					} else { // ack <= session.lastAck; ignore
						break
					}
				}
			case `data`:
				n, err := s.appendRead(msg.Pos, msg.Data)
				// Always send an ack *of current length*, regardless of error.
				s.SendAck(n)
				if err != nil {
					log.Printf(`Session[%s].readWorker: error appending data: %s`, s.Key(), err)
					continue
				}
				// Notify reader that data is available.
				// readCh is 1-buffered. As long as *something* is queued, we can move on. No need to block.
				select {
				case s.readCh <- true:
				default:
				}
			case `connect`, `close`:
				log.Printf(`Session[%s].readWorker: unexpected [%s] message forwarded to reader`, s.Key(), msg.Type)
			default:
				log.Printf(`Session[%s].readWorker: unexpected message type [%s]`, s.Key(), msg.Type)
			}
		}
	}
}

// Receive is a non-blocking method for passing ACK or DATA messages to a Session's readWorker.
// If the readWorker is busy and the internal receive channel is full, an error is returned.
// Other message types will also produce an error.
func (s *Session) Receive(msg *Msg) error {
	if msg.Type != "ack" && msg.Type != "data" {
		return fmt.Errorf("session will only receive ack or data messages (got [%s])", msg.Type)
	}
	select {
	case s.receiveCh <- msg:
		return nil
	default:
		return errors.New("receive channel full")
	}
}

// writeWorker is a per-session goroutine that sends data from the session's writeBuffer.
func (s *Session) writeWorker() {
	retransmissionTicker := time.NewTicker(RetransmissionTimeout)
	writeIndex := 0

	// Select on a time.Ticker for N seconds, close channel, or default
	// close: exit.
	// ticker: reset writeIndex to current lastAck
	// default: send from current writeIndex, incrementing as we go.

	// Reuse a single message for packing
	msg := &Msg{Type: `data`, Session: s.ID}
	// Buffer for encoding messages
	buf := make([]byte, maxMessageSize)

	// Wrapping this in a function for easy defer semantics.
	tryWrite := func() {
		buf = buf[:cap(buf)] // Re-extend for full length writes

		s.writeLock.Lock()
		defer s.writeLock.Unlock()
		if writeIndex >= len(s.writeBuffer) {
			// Nothing to send
			return
		}
		// Send from current writeIndex, incrementing as we go.
		msg.Pos = writeIndex
		packedN := msg.pack(s.writeBuffer[writeIndex:])
		if err := msg.Validate(); err != nil {
			log.Printf(`Session[%s].writeWorker: error validating message [%+v]: %s`, s.Key(), msg, err)
			return
		}
		encodedN, err := msg.encode(buf)
		if err != nil {
			log.Printf(`Session[%s].writeWorker: error encoding message: %s`, s.Key(), err)
			return
		}
		log.Printf(`Session[%s].writeWorker: sending [%d]-byte message with [%d]-packed bytes from write index [%d]`,
			s.Key(), encodedN, packedN, writeIndex)
		_, err = s.SendData(buf[:encodedN])
		if err != nil {
			// For now, we ignore the number of bytes sent on error,
			// since we can always resend them anyway if we bail out here.
			log.Printf(`Session[%s].writeWorker: error sending data message: %s`, s.Key(), err)
			return
		}
		writeIndex += packedN
		// Update maxAckable if we've sent more data than it.
		for { // loop until we don't need to update
			maxAckable := s.maxAckable.Load()
			if writeIndex > int(maxAckable) {
				if s.maxAckable.CompareAndSwap(maxAckable, int32(writeIndex)) { // success
					break
				}
			} else { // writeIndex <= maxAckable; ignore
				break
			}
		}
	}

	for {
		// Room for improvement: this spins a bit. Could signal from Write instead of using a default case.
		select {
		case <-s.ctx.Done():
			log.Printf(`Session[%s].writeWorker closed`, s.Key())
			return
		case <-retransmissionTicker.C:
			// Reset writeIndex to lastAck
			writeIndex = int(s.lastAck.Load())
			// If we're a client and have never been ack'd, resend initial connect
			if writeIndex < 0 {
				err := s.SendConnect()
				if err != nil {
					log.Printf(`Session[%s].writeWorker failed to resend connect: %v`, s.Key(), err)
				}
			}
			continue
		default:
			// Room for improvement: instead of a default case, use another channel here to avoid spinning through tryWrite.
			// Just shove the buffer into the channel, and use a sync.Pool of buffers instead of a single shared buffer
			// Note: this solution means that we don't try to eagerly send data before our connect is ACK'd.
			if writeIndex >= 0 { // -1 until we get initial ack
				tryWrite()
			}
		}
	}
}

// SendAck sends an acknowledgement of a given session length.
// The session's current length isn't strictly used, since we sometimes need to
// send something else.
// For example, we should always respond to a duplicate connect with /ack/SESSION/0/
// (Unclear if *any* ack is fine in that case, but docs specify to send 0.)
func (s *Session) SendAck(length int) error {
	// Send nil addr for client session, since UDP conn is already connected
	var addr *net.UDPAddr
	if !s.isClient {
		addr = s.Addr.(*net.UDPAddr)
	}

	// Send UDP ack message to Addr
	msg := []byte(fmt.Sprintf(`/ack/%d/%d/`, s.ID, length))
	n, _, err := s.conn.WriteMsgUDP(msg, nil, addr)
	if err != nil {
		return fmt.Errorf("Session[%s].sendAck: error sending ack message: %s", s.Key(), err)
	}
	if n != len(msg) {
		return fmt.Errorf("Session[%s].sendAck: short write sending ack message: %d != %d", s.Key(), n, len(msg))
	}
	return nil
}

// SendConnect sends a connect message to the session's peer.
func (s *Session) SendConnect() error {
	// Send nil addr for client session, since UDP conn is already connected
	var addr *net.UDPAddr
	if !s.isClient {
		addr = s.Addr.(*net.UDPAddr)
	}

	msg := []byte(fmt.Sprintf(`/connect/%d/`, s.ID))
	n, _, err := s.conn.WriteMsgUDP(msg, nil, addr)
	if err != nil {
		return fmt.Errorf("Session[%s].sendConnect: error sending connect message: %s", s.Key(), err)
	}
	if n != len(msg) {
		return fmt.Errorf("Session[%s].sendConnect: short write sending connect message: %d != %d", s.Key(), n, len(msg))
	}
	return nil

}

// SendData sends a data message to the session's peer.
func (s *Session) SendData(packedData []byte) (int, error) {
	// Send nil addr for client session, since UDP conn is already connected
	var addr *net.UDPAddr
	if !s.isClient {
		addr = s.Addr.(*net.UDPAddr)
	}

	log.Printf(`Session[%s].sendData: sending [%d] bytes`, s.Key(), len(packedData))
	n, _, err := s.conn.WriteMsgUDP(packedData, nil, addr)
	return n, err
}

// SendClose sends a close message for sessionID.
func (s *Session) SendClose() error {
	// Send nil addr for client session, since UDP conn is already connected
	var addr *net.UDPAddr
	if !s.isClient {
		addr = s.Addr.(*net.UDPAddr)
	}

	msg := []byte(fmt.Sprintf(`/close/%d/`, s.ID))
	n, _, err := s.conn.WriteMsgUDP(msg, nil, addr)
	if err != nil {
		return fmt.Errorf("Session[%s].sendClose: error sending close message: %s", s.Key(), err)
	}
	if n != len(msg) {
		return fmt.Errorf("Session[%s].sendClose: short write sending close message: %d != %d", s.Key(), n, len(msg))
	}
	return nil
}

// SendClose sends a close message for the given sessionID.
// This isn't defined on Session since we may want to close a non-existent session.
// See Session.Close for closing an existing session.
func SendClose(sessionID int, addr net.Addr, conn *net.UDPConn) error {
	// Send UDP close message to Addr
	msg := []byte(fmt.Sprintf(`/close/%d/`, sessionID))
	n, _, err := conn.WriteMsgUDP(msg, nil, addr.(*net.UDPAddr))
	if err != nil {
		return fmt.Errorf("sendClose: error sending close message for session [%d]: %s", sessionID, err)
	}
	if n != len(msg) {
		return fmt.Errorf("sendClose: short write sending close message for session [%d]: %d != %d", sessionID, n, len(msg))
	}
	return nil
}
