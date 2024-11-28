package main

import (
	"context"
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
const RetransmissionTimeout = 3 * time.Second

// How long to wait for a new message before timing out.
// "session expiry timeout: the time to wait before accepting that a peer has disappeared,
// in the event that no responses are being received. Suggested default value: 60 seconds."
const ReadTimeout = 60 * time.Second

type Session struct {
	// Synchronizes Session.Read and Session.readWorker
	readLock sync.Mutex
	// Synchronizes Session.Write and Session.writeWorker
	writeLock sync.Mutex

	// The peer's address.
	Addr net.Addr
	// The session's unique ID used in LRCP messages (e.g. SESSION in /data/SESSION/POS/DATA/).
	ID int

	// The UDP connection to send messages on.
	// Incoming messages are de-muxed by the listener.
	conn *net.UDPConn
	// Message pool for re-use.
	pool *sync.Pool

	// Context for closing the session.
	ctx    context.Context
	cancel context.CancelFunc

	// quitCh signals to the Listener that the Session is exiting, so Listener can reap it from the session store.
	// This occurs when a peer times out, misbehaves, etc.
	quitCh chan *Session

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
	lastAck atomic.Int32

	// maxAckable is the maximum length we will accept an ack for.
	maxAckable atomic.Int32

	// writeBuffer is the session's data to be sent.
	writeBuffer []byte
}

func NewSession(addr net.Addr, id int, conn *net.UDPConn, pool *sync.Pool, timeoutCh chan *Session) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		Addr:        addr,
		ID:          id,
		conn:        conn,
		pool:        pool,
		quitCh:      timeoutCh,
		receiveCh:   make(chan *Msg, 1),
		readCh:      make(chan bool, 1),
		ctx:         ctx,
		cancel:      cancel,
		readBuffer:  make([]byte, 0, 1024),
		writeBuffer: make([]byte, 0, 1024),
	}
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
	if total := pos + len(s.readBuffer); total > maxInt {
		return len(s.readBuffer), fmt.Errorf("total data length %d exceeds max transmission size %d", total, maxInt)
	}
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

// Close current session.
func (s *Session) Close() {
	s.cancel()
}

func (s *Session) readWorker() {
	timeoutTimer := time.NewTimer(ReadTimeout)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timeoutTimer.C:
			log.Printf(`Sesssion[%s].readWorker: no reply from peer; alerting timeout`, s.Key())
			// This is a unbuffered channel, but we're done so it's fine to just block here.
			s.quitCh <- s
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
					sendClose(s.ID, s.Addr, s.conn)
					s.Close()
					s.quitCh <- s
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
				s.sendAck(n)
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
				log.Printf(`Session[%s].readWorker: unexpected [%s] message forwarded to reader `, s.Key(), msg.Type)
			default:
				log.Printf(`Session[%s].readWorker: unexpected message type [%s]`, s.Key(), msg.Type)
			}
			s.pool.Put(msg)
		}
	}
}

// writeWorker is a per-session goroutine that sends data from the session's writeBuffer.
func (s *Session) writeWorker() {
	ticker := time.NewTicker(RetransmissionTimeout)
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
			log.Printf(`Session[%s].writeWorker: error validating message [%v+]: %s`, s.Key(), msg, err)
			return
		}
		encodedN, err := msg.encode(buf)
		if err != nil {
			log.Printf(`Session[%s].writeWorker: error encoding message: %s`, s.Key(), err)
			return
		}
		_, err = s.sendData(buf[:encodedN])
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
			} else { // ack <= session.lastAck; ignore
				break
			}
		}
	}

	for {
		select {
		case <-s.ctx.Done():
			log.Printf(`Session[%s].writeWorker closed`, s.Key())
			return
		case <-ticker.C:
			// Reset writeIndex to lastAck
			writeIndex = int(s.lastAck.Load())
			continue
		default:
			// Room for improvement: instead of a default case, use another channel here to avoid spinning through tryWrite.
			// Just shove the buffer into the channel, and use a sync.Pool of buffers instead of a single shared buffer
			tryWrite()
		}
	}
}

// sendAck sends an acknowledgement of a given session length.
// The session's current length isn't strictly used, since we sometimes need to send something else.
// For example, we should always respond to a duplicate connect with /ack/SESSION/0/
// (Unclear if *any* ack is fine in that case, but docs specify to send 0.)
func (s *Session) sendAck(length int) error {
	// Send UDP ack message to Addr
	msg := []byte(fmt.Sprintf(`/ack/%d/%d/`, s.ID, length))
	n, _, err := s.conn.WriteMsgUDP(msg, nil, s.Addr.(*net.UDPAddr))
	if err != nil {
		return fmt.Errorf("error sending ack message: %s", err)
	}
	if n != len(msg) {
		return fmt.Errorf("short write sending ack message: %d != %d", n, len(msg))
	}
	return nil
}

// sendData sends a data message to the session's peer.
func (s *Session) sendData(packedData []byte) (int, error) {
	log.Printf(`Session[%s] sending data: %s`, s.Key(), packedData)
	n, _, err := s.conn.WriteMsgUDP(packedData, nil, s.Addr.(*net.UDPAddr))
	return n, err

}

// sendClose sends a close message for sessionID.
// This isn't defined on Session since we may want to close a non-existent session.
// See Session.Close for closing an existing session.
func sendClose(sessionID int, addr net.Addr, conn *net.UDPConn) error {
	// Send UDP close message to Addr
	msg := []byte(fmt.Sprintf(`/close/%d/`, sessionID))
	n, _, err := conn.WriteMsgUDP(msg, nil, addr.(*net.UDPAddr))
	if err != nil {
		return fmt.Errorf("error sending close message: %s", err)
	}
	if n != len(msg) {
		return fmt.Errorf("short write sending close message: %d != %d", n, len(msg))
	}
	return nil
}
