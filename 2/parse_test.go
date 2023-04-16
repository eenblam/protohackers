package main

import (
	"fmt"
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
		Input []byte
		Want  string
	}{
		{
			Input: []byte{0x49, 0x00, 0x00, 0x30, 0x39, 0x00, 0x00, 0x00, 0x65},
			Want:  "I 12345 101",
		},
		{
			Input: []byte{0x49, 0x00, 0x00, 0x30, 0x3a, 0x00, 0x00, 0x00, 0x66},
			Want:  "I 12346 102",
		},
		{
			Input: []byte{0x49, 0x00, 0x00, 0x30, 0x3b, 0x00, 0x00, 0x00, 0x64},
			Want:  "I 12347 100",
		},
		{
			Input: []byte{0x49, 0x00, 0x00, 0xa0, 0x00, 0x00, 0x00, 0x00, 0x05},
			Want:  "I 40960 5",
		},
		{
			Input: []byte{0x51, 0x00, 0x00, 0x30, 0x00, 0x00, 0x00, 0x40, 0x00},
			Want:  "Q 12288 16384",
		},
	}
	for _, test := range tests {
		t.Run(test.Want, func(t *testing.T) {
			kind, a, b, err := Parse(test.Input)
			if err != nil {
				t.Fatalf("Unexpected error %s", err)
			}
			got := fmt.Sprintf("%c %d %d", kind, a, b)
			if test.Want != got {
				t.Fatalf(`Want %s
			     Got %s`, test.Want, got)
			}
		})
	}
}

func TestParseBadLengths(t *testing.T) {
	tests := [][]byte{
		[]byte{0x49, 0x00},
		[]byte{0x49, 0x00, 0x00, 0x30, 0x3a, 0x00, 0x00, 0x00},
		[]byte{0x49, 0x00, 0x00, 0x30, 0x3b, 0x00, 0x00, 0x00, 0x64, 0x77},
	}
	for i, test := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			_, _, _, err := Parse(test)
			if err == nil {
				t.Fatalf("Expected error for array of length %d, got none", len(test))
			}
		})
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
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			_, _, _, err := Parse(test)
			if err == nil {
				t.Fatalf("Expected error for message of type %x, got none", test[0])
			}
		})
	}
}
