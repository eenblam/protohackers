package main

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestMain(t *testing.T) {
	go main()
	// Let main warm up
	time.Sleep(50 * time.Millisecond)

	t.Parallel()
	addr := net.TCPAddr{
		net.ParseIP("127.0.0.1"),
		9999,
		""}

	tests := []string{
		"asdf\nfdsa",
		"ÀÁÂÃÄÅÆÇÈÉÊËÌÍÎÏ",
		"",
		"The quick brown fox jumped over the lazy dog.",
		"!!!!!!!!!!!!!!!!!!!!!!!!!!!! @@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@",
		"@@@@@@@@@@@@@@@@@@@@@@@@@@@@ #####################################",
		"############################ $$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$",
		"$$$$$$$$$$$$$$$$$$$$$$$$$$$$ %%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%",
		"%%%%%%%%%%%%%%%%%%%%%%%%%%%% ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^",
		"^^^^^^^^^^^^^^^^^^^^^^^^^^^^ &&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&",
		"&&&&&&&&&&&&&&&&&&&&&&&&&&&& *************************************",
		"**************************** (((((((((((((((((((((((((((((((((((((",
	}

	for i, testCase := range tests {
		// Weird scope footgun
		i, testCase := i, testCase
		t.Run(testCase, func(t *testing.T) {
			t.Parallel()
			conn, err := net.DialTCP("tcp", nil, &addr)
			if err != nil {
				t.Fatalf("TEST %d: couldn't dial TCP: %s\n", i, err)
				return
			}
			defer conn.Close()

			// Write the test string 10000 times, just to have some concurrent load
			times := 10000
			written := 0
			for j := 0; j < times; j++ {
				w, err := conn.Write([]byte(testCase))
				if err != nil {
					t.Fatalf("TEST %d: error writing to conn: %s\n", i, err)
				}
				written += w
			}
			err = conn.CloseWrite()
			if err != nil {
				t.Fatalf("TEST %d: failed to close write: %s\n", i, err)
			}

			// Read and compare one byte at a time
			var buf [1]byte
			read := 0
			for {
				r, err := conn.Read(buf[:])
				//TODO Test that conn is closed with brief timeout
				if err != nil {
					if err == io.EOF {
						//log.Printf("TEST %d: received EOF from server", i)
						conn.Close()
						break
					}
					t.Fatalf("TEST %d: failed to read: %s\n", i, err)
				}
				read += r
				got := buf[0]
				exp_index := (read - 1) % len(testCase)
				expected := testCase[exp_index]
				if got != expected {
					t.Fatalf("TEST %d: got %b expected %b at byte %d\n", i, got, expected, read)
				}
			}
			// Test total is correct
			if read != len(testCase)*times {
				t.Fatalf("TEST %d: expected %d bytes, got %d\n", i, len(testCase)*times, read)
			}
		})
	}
}
