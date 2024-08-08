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
const RetransmissionTimeout = 3

type Session struct {
	// Synchronizes Session.Read and Session.readWorker
	readLock sync.Mutex
	// Synchronizes Session.Write and Session.writeWorker
	writeLock sync.Mutex

	Addr net.Addr
	ID   int

	// The UDP connection to send messages on.
	// Incoming messages are de-muxed by the listener.
	conn *net.UDPConn
	// Message pool for re-use.
	pool *sync.Pool

	// Context for closing the session.
	ctx    context.Context
	cancel context.CancelFunc

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

	// writeBuffer is the session's data to be sent.
	writeBuffer []byte
}

func NewSession(addr net.Addr, id int, conn *net.UDPConn, pool *sync.Pool) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		Addr:      addr,
		ID:        id,
		conn:      conn,
		pool:      pool,
		receiveCh: make(chan *Msg, 1),
		readCh:    make(chan bool, 1),
		ctx:       ctx,
		cancel:    cancel,
		//TODO reconsider these default sizes
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
	// Count slashes to get total length; error if this would be an illegal write
	// NOTE: we could instead write until we total maxInt, then return (maxInt, error).
	slashes := 0
	for _, c := range b {
		if c == '/' || c == '\\' {
			slashes++
		}
	}
	if total := len(s.writeBuffer) + len(b) + slashes; total > maxInt {
		return len(s.writeBuffer), fmt.Errorf("total data length %d exceeds max transmission size %d", total, maxInt)
	}
	// Copy data, escaping slashes
	for _, c := range b {
		if c == '/' || c == '\\' {
			s.writeBuffer = append(s.writeBuffer, '\\')
		}
		s.writeBuffer = append(s.writeBuffer, c)
	}

	return len(s.writeBuffer), nil
}

// Close current session.
func (s *Session) Close() {
	s.cancel()
}

func (s *Session) readWorker() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-s.receiveCh:
			switch msg.Type {
			case `ack`:
				// As long as ack'd length > session.lastAck, try to update session.lastAck
				for {
					if current := s.lastAck.Load(); msg.Length > int(current) {
						if s.lastAck.CompareAndSwap(current, int32(msg.Length)) { // success
							break
						}
					} else { // ack <= session.lastAck; ignore
						break
					}
				}
			case `data`:
				n, err := s.appendRead(msg.Pos, msg.Data)
				if err != nil {
					log.Printf(`error appending data to session %s: %s`, s.Key(), err)
				}
				s.sendAck(n)
			case `connect`, `close`:
				log.Printf(`unexpected [%s] message forwarded to reader for session %s`, msg.Type, s.Key())
			default:
				log.Printf(`unexpected message type %s for session %s`, msg.Type, s.Key())
			}
			s.pool.Put(msg)
		}
	}
}

// writeWorker is a per-session goroutine that sends data from the session's writeBuffer.
func (s *Session) writeWorker() {
	ticker := time.NewTicker(RetransmissionTimeout * time.Second)
	writeIndex := 0

	// Select on a time.Ticker for N seconds, close channel, or default
	// close: exit.
	// ticker: reset writeIndex to current lastAck
	// default: send from current writeIndex, incrementing as we go.

	// TODO: instead of a default, use another channel.
	// Just shove the buffer into the channel, and use a sync.Pool of buffers instead of a single shared buffer

	// Reuse a single message for packing
	msg := &Msg{Session: s.ID}
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
		// Note that slashes should already be escaped.
		msg.Pos = writeIndex
		packedN := msg.pack(s.writeBuffer[writeIndex:])
		encodedN, err := msg.encode(buf)
		if err != nil {
			log.Printf(`error encoding message: %s`, err)
			return
		}
		_, err = s.sendData(buf[:encodedN])
		if err != nil {
			log.Printf(`error sending data message: %s`, err)
			return
		}
		//TODO check sentN against buf length? Will this ever be unequal without error?
		writeIndex += packedN
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
	msg := []byte(fmt.Sprintf(`/ack/%d/%d/`, s.ID, len(s.readBuffer)))
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
	n, _, err := s.conn.WriteMsgUDP(packedData, nil, s.Addr.(*net.UDPAddr))
	//TODO do I anticipate an issue for n!=len(packedData)?
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
