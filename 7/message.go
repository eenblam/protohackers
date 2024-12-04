package main

import (
	"bytes"
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
const maxInt = 2147483647 // 2**31 - 1

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
// of the session ID and pos, and on the number of slashes that must be escaped.
// pack does *not* handle validation! Call Validate() after calling pack.
func (m *Msg) pack(data []byte) int {
	// /data/SESSION/POS/DATA/
	// So 9 bytes for /data////, plus len(string(Session)), plus len(string(Pos))
	// Subtracting from maxMsgSize, we get the max length of Data we can use.
	maxCopy := maxMessageSize - len(fmt.Sprintf("/data/%d/%d//", m.Session, m.Pos))

	// Count slashes to get length of escaped data
	slashes := 0
	for _, c := range data {
		if c == '/' || c == '\\' {
			slashes++
		}
	}

	copySize := min(maxCopy, len(data)+slashes)

	// In case we want to reuse an existing Msg. This Msg is likely reused via a pool.
	if m.Data == nil || len(m.Data) < copySize {
		m.Data = make([]byte, copySize)
	} else {
		m.Data = m.Data[:copySize]
	}

	// Copy bytes into the message, escaping slashes as we go
	// j counts original bytes copied (and indexes into data)
	j := 0
	// i counts bytes written, including escape characters
	for i := 0; i < copySize && j < len(data); i++ {
		if data[j] == '/' || data[j] == '\\' {
			// Don't try to escape if we can't fit both the escape and the character
			if i+1 >= copySize {
				// Trim final byte, since we can't copy it
				m.Data = m.Data[:i]
				break
			}
			m.Data[i] = '\\'
			i++
		}
		m.Data[i] = data[j]
		j++
	}

	return j
}

func parseMessage(bs []byte) (*Msg, error) {
	msg := &Msg{}
	if len(bs) == 0 {
		return nil, errors.New("empty message")
	}
	if bs[0] != byte('/') {
		return nil, errors.New("missing leading /")
	}

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
	// Track if a previous backslash \ was escaped
	// Don't track if forward slash escaped - /\ shouldn't escape the /, but // should
	escape := false
	for i := range bs {
		if escape { // Previous byte was an unescaped \
			escape = false // Next byte can't be escaped if this one was
			if bs[i] == '\\' || bs[i] == '/' {
				continue
			}
			// Else error
			return nil, nil, fmt.Errorf("previous byte was \\, but this byte [%x] is unescapable", bs[i])
		}
		if bs[i] == '\\' {
			escape = true
			continue
		}
		if bs[i] == '/' { // Unescaped /
			return bs[:i], bs[i+1:], nil
		}
	}
	return nil, nil, fmt.Errorf("no / found in input [%x]", bs)
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
	// Just return a copy if no slashes found
	for i := range bs {
		if bs[i] == '\\' || bs[i] == '/' {
			goto ESCAPED
		}
	}
	return bytes.Clone(bs), nil

ESCAPED:
	// Unescape / and \ by populating a fresh array
	out := make([]byte, 0, len(bs))

	var escape bool
	for i := range bs {
		switch {
		case bs[i] == '\\' && escape, bs[i] == '/' && escape:
			escape = false
			out = append(out, bs[i])
		case bs[i] == '\\' && !escape:
			escape = true
		case bs[i] == '/' && !escape:
			return nil, fmt.Errorf("unescaped forward slash at index [%d]", i)
		case escape:
			return nil, fmt.Errorf("illegally escaped byte [%x] at index [%d]", bs[i], i)
		default:
			out = append(out, bs[i])
		}
	}
	if escape {
		// We encountered an unescaped \ at the end, then set escape.
		return nil, fmt.Errorf("unescaped backslash at final byte index [%d]", len(bs)-1)
	}
	return out, nil
}
