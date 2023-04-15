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

	for _, testCase := range tests {
		// Weird scope footgun
		// Don't want testCase to refer to the global
		testCase := testCase
		t.Run(testCase, func(t *testing.T) {
			t.Parallel()
			conn, err := net.DialTCP("tcp", nil, &addr)
			if err != nil {
				t.Fatalf("Couldn't dial TCP: %s\n", err)
				return
			}
			defer conn.Close()

			// Write the test string 10000 times, just to have some concurrent load
			times := 10000
			written := 0
			for j := 0; j < times; j++ {
				w, err := conn.Write([]byte(testCase))
				if err != nil {
					t.Fatalf("Error writing to conn: %s\n", err)
				}
				written += w
			}
			err = conn.CloseWrite()
			if err != nil {
				t.Fatalf("Failed to close write: %s\n", err)
			}

			// Read and compare one byte at a time
			var buf [1]byte
			read := 0
			for {
				r, err := conn.Read(buf[:])
				//TODO Test that conn is closed with brief timeout
				if err != nil {
					if err == io.EOF {
						conn.Close()
						break
					}
					t.Fatalf("Failed to read: %s\n", err)
				}
				read += r
				got := buf[0]
				exp_index := (read - 1) % len(testCase)
				want := testCase[exp_index]
				if got != want {
					t.Fatalf("Want %b got %b at byte %d\n", want, got, read)
				}
			}
			// Test total is correct
			if read != len(testCase)*times {
				t.Fatalf("Want %d bytes, got %d\n", len(testCase)*times, read)
			}
		})
	}
}
