package main

import (
	"testing"
)

/*
<-- 49 00 00 30 39 00 00 00 65   I 12345 101
<-- 49 00 00 30 3a 00 00 00 66   I 12346 102
<-- 49 00 00 30 3b 00 00 00 64   I 12347 100
<-- 49 00 00 a0 00 00 00 00 05   I 40960 5
<-- 51 00 00 30 00 00 00 40 00   Q 12288 16384
--> 00 00 00 65                  101
*/

func TestParseHappy(t *testing.T) {
	tests := []struct {
		Input    []byte
		Expected string
	}{
		{
			Input:    []byte{0x49, 0x00, 0x00, 0x30, 0x39, 0x00, 0x00, 0x00, 0x65},
			Expected: "I 12345 101",
		},
		{
			Input:    []byte{0x49, 0x00, 0x00, 0x30, 0x3a, 0x00, 0x00, 0x00, 0x66},
			Expected: "I 12346 102",
		},
		{
			Input:    []byte{0x49, 0x00, 0x00, 0x30, 0x3b, 0x00, 0x00, 0x00, 0x64},
			Expected: "I 12347 100",
		},
		{
			Input:    []byte{0x49, 0x00, 0x00, 0xa0, 0x00, 0x00, 0x00, 0x00, 0x05},
			Expected: "I 40960 5",
		},
		{
			Input:    []byte{0x51, 0x00, 0x00, 0x30, 0x00, 0x00, 0x00, 0x40, 0x00},
			Expected: "Q 12288 16384",
		},
	}
	for i, test := range tests {
		var m RawMessage
		err := m.Parse(test.Input)
		if err != nil {
			t.Fatalf("(Test case %d) Unexpected error %s", i, err)
		}
		got := m.Text()
		if got != test.Expected {
			t.Fatalf(`(Test case %d) Expected %s
			     Got %s`, i, test.Expected, got)
		}
	}
}

func TestParseBadLengths(t *testing.T) {
	tests := [][]byte{
		[]byte{0x49, 0x00},
		[]byte{0x49, 0x00, 0x00, 0x30, 0x3a, 0x00, 0x00, 0x00},
		[]byte{0x49, 0x00, 0x00, 0x30, 0x3b, 0x00, 0x00, 0x00, 0x64, 0x77},
	}
	for i, test := range tests {
		var m RawMessage
		err := m.Parse(test)
		if err == nil {
			t.Fatalf("(Test case %d) Expected error for array of length %d, got none", i, len(test))
		}
	}
}

func TestParseBadTypes(t *testing.T) {
	tests := [][]byte{
		[]byte{0x00, 0x00, 0x00, 0x30, 0x39, 0x00, 0x00, 0x00, 0x65},
		[]byte{0x48, 0x00, 0x00, 0x30, 0x39, 0x00, 0x00, 0x00, 0x65},
		[]byte{0x50, 0x00, 0x00, 0x30, 0x39, 0x00, 0x00, 0x00, 0x65},
		// Bad type AND bad length
		[]byte{0x52, 0x00, 0x00, 0x30, 0x39, 0x00, 0x00, 0x00},
		[]byte{0x52, 0x00, 0x00, 0x30, 0x39, 0x00, 0x00, 0x00, 0x65, 0x99},
	}
	for i, test := range tests {
		var m RawMessage
		err := m.Parse(test)
		if err == nil {
			t.Fatalf("(Test case %d) Expected error for message of type %x, got none", i, test[0])
		}
	}
}
