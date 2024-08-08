package main

import (
	"errors"
	"fmt"
	"strconv"
)

/* Supported message formats:
/connect/SESSION/
/data/SESSION/POS/DATA/
/ack/SESSION/LENGTH/
/close/SESSION/
*/

// "Numeric field values must be smaller than 2147483648."
const maxInt = 2147483647

// "LRCP messages must be smaller than 1000 bytes.
// You might have to break up data into multiple data
// messages in order to fit it below this limit."
const maxMessageSize = 999

type Msg struct {
	Type    string
	Session int
	// Note that Pos and Length could be int32, given our maxInt constraint.
	// type:data
	Pos  int
	Data []byte
	// type:ack
	Length int
}

func (m *Msg) Validate() error {
	if m.Session > maxInt {
		return fmt.Errorf("session ID is too large (%d > %d)", m.Session, maxInt)
	}

	switch m.Type {
	case "data":
		if m.Pos > maxInt {
			return fmt.Errorf("pos is too large (%d > %d)", m.Pos, maxInt)
		}
		// do pos and len(data) match?
		totalData := m.Pos + len(m.Data)
		if totalData > maxInt {
			return fmt.Errorf("total data length is too large (%d > %d)", totalData, maxInt)
		}
	case "ack":
		if m.Length > maxInt {
			return fmt.Errorf("length %d is too large", m.Length)
		}
	}
	return nil
}

// encode will write the message to the provided buffer, returning the number of bytes written.
// An error will be returned if the message is of an unknown type.
func (m *Msg) encode(buf []byte) (int, error) {
	var data []byte
	switch m.Type {
	case "connect":
		data = []byte(fmt.Sprintf("/connect/%d/", m.Session))
	case "data":
		data = []byte(fmt.Sprintf("/data/%d/%d/%s/", m.Session, m.Pos, m.Data))
	case "ack":
		data = []byte(fmt.Sprintf("/ack/%d/%d/", m.Session, m.Length))
	case "close":
		data = []byte(fmt.Sprintf("/close/%d/", m.Session))
	default:
		return 0, fmt.Errorf("cannot encode message of unknown type %s", m.Type)
	}
	return copy(buf, data), nil
}

// pack will copy data into the message's Data slice, returning the number of bytes copied from the input,
// NOT the total size of the LRCP message.
// The number of bytes that can be copied will depend on the lengths of the string representations
// of the session ID and pos.
// pack does *not* handle escaping slashes or validation.
// The caller *must* first escape, and the packed Msg must be then validated.
func (m *Msg) pack(data []byte) int {
	// /data/SESSION/POS/DATA/
	// So 9 bytes for /data////, plus len(string(Session)), plus len(string(Pos))
	// Subtracting from maxMsgSize, we get the max length of Data we can use.
	maxCopy := maxMessageSize - len(fmt.Sprintf("/data/%d/%d//", m.Session, m.Pos))
	copySize := min(maxCopy, len(data))
	// In case we want to reuse an existing Msg
	if m.Data == nil || len(m.Data) < copySize {
		m.Data = make([]byte, copySize)
	}
	copy(m.Data, data[:copySize])
	m.Data = m.Data[:copySize]
	return copySize
}

func parseMessage(bs []byte) (*Msg, error) {
	if len(bs) == 0 {
		return nil, errors.New("empty message")
	}
	if bs[0] != byte('/') {
		return nil, errors.New("missing leading /")
	}

	msg := &Msg{}
	// Parse type
	t, rest, err := parseField(bs[1:]) // Skip leading /
	if err != nil {
		return nil, fmt.Errorf("error parsing type: %w", err)
	}
	msg.Type = string(t)
	if !(msg.Type == "connect" || msg.Type == "data" || msg.Type == "ack" || msg.Type == "close") {
		return nil, fmt.Errorf(`unknown type "%s"`, msg.Type)
	}

	// Parse session
	session, rest, err := parseField(rest)
	if err != nil {
		return nil, fmt.Errorf("error parsing session: %w", err)
	}
	sessionInt, err := parseInt(session)
	if err != nil {
		return nil, fmt.Errorf("error parsing session int: %w", err)
	}
	msg.Session = sessionInt

	switch string(msg.Type) {
	case "connect":
		// /connect/SESSION/
		if len(rest) != 0 {
			return nil, fmt.Errorf("extra data after Session on Connect: %s", rest)
		}
		return msg, nil
	case "data":
		// /data/SESSION/POS/DATA/
		// Parse Pos
		rawPos, rest, err := parseField(rest)
		if err != nil {
			return nil, fmt.Errorf("error parsing Pos field: %w", err)
		}
		parsedPos, err := parseInt(rawPos)
		if err != nil {
			return nil, fmt.Errorf("error parsing Pos value: %w", err)
		}
		msg.Pos = parsedPos
		// Parse Data
		rawData, rest, err := parseField(rest)
		if err != nil {
			return nil, fmt.Errorf("error parsing Data field: %w", err)
		}
		if len(rest) != 0 {
			return nil, fmt.Errorf("extra data after Data field: %s", rest)
		}
		parsedData, err := parseData(rawData)
		if err != nil {
			return nil, fmt.Errorf("error parsing Data value: %w", err)
		}
		msg.Data = parsedData
		return msg, nil
	case "ack":
		// /ack/SESSION/LENGTH/
		rawLength, rest, err := parseField(rest)
		if err != nil {
			return nil, fmt.Errorf("error parsing Pos field: %w", err)
		}
		if len(rest) != 0 {
			return nil, fmt.Errorf("extra data after Length field: %s", rest)
		}
		parsedLength, err := parseInt(rawLength)
		if err != nil {
			return nil, fmt.Errorf("error parsing Length value: %w", err)
		}
		msg.Length = parsedLength
		return msg, nil
	case "close":
		// /close/SESSION/
		if len(rest) != 0 {
			return nil, fmt.Errorf("extra data after Session on Close: %s", rest)
		}
		return msg, nil
	default:
	}
	return nil, fmt.Errorf(`unknown type "%s"`, t)
}

// parseField will scan to the next unescaped /, returning the parsed field and any remaining bytes after the /.
// Returns an error if no unescaped slash is found, as all messages must end with a /.
func parseField(bs []byte) ([]byte, []byte, error) {
	var i int
	for i = 0; i < len(bs); i++ {
		if bs[i] != byte('/') {
			continue
		}
		if i != 0 && bs[i-1] == byte('\\') { // This slash was escaped
			continue
		}
		break
	}
	if i == len(bs) {
		return nil, nil, fmt.Errorf("no / found in input %s", string(bs))
	}
	before, after := bs[:i], bs[i+1:]
	return before, after, nil
}

// parseInt parses a field to an int
func parseInt(bs []byte) (int, error) {
	i, err := strconv.Atoi(string(bs))
	if err != nil {
		return 0, fmt.Errorf("error parsing int: %w", err)
	}
	if i > maxInt {
		return 0, fmt.Errorf("int %d is too large", i)
	}
	return i, nil
}

// parseData parses a Data field, unescaping any forward or backward slashes
func parseData(bs []byte) ([]byte, error) {
	if len(bs) == 0 {
		return []byte{}, nil
	}

	// Unescape / and \ by populating a fresh array
	out := make([]byte, len(bs))
	j := 0                           // Index into output
	for i := 0; i < len(bs)-1; i++ { // Iterate up to next-to-last byte
		this, next := bs[i], bs[i+1]
		// Catch escaped slashes
		if this == byte('\\') && (next == byte('/') || next == byte('\\')) {
			out[j] = next
			i++ // skip next
		} else if this == byte('\\') || this == byte('/') {
			// This isn't an escaping backslash, and escaped slashes are handled above, so error.
			return nil, fmt.Errorf(`unescaped character "%c" at position %d in data "%s"`, this, i, string(bs))
		} else if i == len(bs)-2 {
			// This is the last step, so we need to handle the last byte
			if next == byte('\\') || next == byte('/') {
				// This isn't an escaping backslash, and escaped slashes are handled above, so error.
				return nil, fmt.Errorf(`unescaped character "%c" at position %d in data "%s"`, next, i+1, string(bs))
			}
			out[j] = this
			out[j+1] = next
		} else {
			out[j] = this
		}
		// We want to increment this even on the last step to get our final bound correct
		j++
	}
	return out[:j+1], nil
}
