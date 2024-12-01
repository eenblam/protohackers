package main

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func TestMain(t *testing.T) {
	go main()
	time.Sleep(50 * time.Millisecond)
	t.Parallel()

	raddr := &net.UDPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: 4321,
	}

	cases := []struct {
		Name  string
		Input []byte
		Want  []byte
	}{
		{
			Name: "Foo",
			// Use "" (interpreted literal) not `` (raw literal) here for proper newlines
			// (otherwise we have to literally create a linebreak)
			Input: []byte("asdf\nqwer\n"),
			Want:  []byte("fdsa\nrewq\n"),
		},
	}

	for _, testCase := range cases {
		testCase := testCase // Avoid scope footgun

		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()
			s, err := DialLRCP("lrcp", nil, raddr)
			if err != nil {
				t.Fatalf("unexpected dial error: %v", err)
			}
			n, err := s.Write(testCase.Input)
			if n != len(testCase.Input) {
				t.Fatalf("expected to write [%d] bytes, wrote [%d]", len(testCase.Input), n)
			}
			if err != nil {
				t.Fatalf("unexpected write error: %v", err)
			}
			buf := make([]byte, len(testCase.Want)*2) // Allocate extra to test we don't receive extra
			bytesRead := 0
			for {
				n, err = s.Read(buf[bytesRead:])
				bytesRead += n
				if err == io.EOF {
					break
				} else if err != nil {
					t.Fatalf("unexpected read error: %v", err)
				}
				if bytesRead >= len(testCase.Want) {
					break
				}
			}
			buf = buf[:bytesRead]
			if !bytes.Equal(buf, testCase.Want) {
				t.Fatalf("unequal bytes; got [%v] != want [%v]", buf, testCase.Want)
			}
			s.Close()
		})

	}
}
