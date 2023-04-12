package main

import (
	"encoding/binary"
	"fmt"
)

/*
<-- 49 00 00 30 39 00 00 00 65   I 12345 101
<-- 49 00 00 30 3a 00 00 00 66   I 12346 102
<-- 49 00 00 30 3b 00 00 00 64   I 12347 100
<-- 49 00 00 a0 00 00 00 00 05   I 40960 5
<-- 51 00 00 30 00 00 00 40 00   Q 12288 16384
--> 00 00 00 65                  101
*/

type MessageType byte

const (
	insert MessageType = 0x49
	query              = 0x51
)

func (m MessageType) Text() string {
	switch m {
	case insert:
		return "I"
	case query:
		return "Q"
	default:
		return "U" // UNDEFINED
	}
}

type RawMessage struct {
	Type MessageType
	A    uint32
	B    uint32
}

func (m *RawMessage) Parse(bs []byte) error {
	if len(bs) != 9 {
		return fmt.Errorf("Expected 9 bytes, got %d", len(bs))
	}
	// Parse message type
	mtype := MessageType(bs[0])
	if !(mtype == insert || mtype == query) {
		return fmt.Errorf("Expected %x or %x, got %x", insert, query, bs[0])
	}
	// Update internals
	m.Type = mtype
	m.A = binary.BigEndian.Uint32(bs[1:5])
	m.B = binary.BigEndian.Uint32(bs[5:9])
	return nil
}

func (m *RawMessage) Text() string {
	switch m.Type {
	case insert:
		//return fmt.Sprintf("%s %d %d", m.Text(), ts(m.A), asset(m.B))
		return fmt.Sprintf("%s %d %d", m.Type.Text(), m.A, m.B)
	case query:
		//return fmt.Sprintf("%s %d %d", m.Text(), ts(m.A), ts(m.B))
		return fmt.Sprintf("%s %d %d", m.Type.Text(), m.A, m.B)
	default:
		return fmt.Sprintf("%s", m.Text())
	}
}
