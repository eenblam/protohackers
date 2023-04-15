package main

import (
	"bufio"
	"net"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	go main()
	// Let main warm up. Gotta compute some primes!
	time.Sleep(3 * time.Second)
	// Run tests
	status := m.Run()
	os.Exit(status)
}

func TestServerHappy(t *testing.T) {
	addr := net.TCPAddr{
		net.ParseIP("127.0.0.1"),
		port, // from main.go
		""}

	tests := []struct {
		Name  string
		Input string
		Want  string
	}{
		{
			Name:  "7 is prime",
			Input: `{"method":"isPrime","number":7}`,
			// Response will be unspaced like this.
			// We could be more robust by actually parsing JSON in tests, but this is for fun so...
			Want: `{"method":"isPrime","prime":true}`,
		},
		{
			Name:  "-7 is not prime",
			Input: `{"method":"isPrime","number":-7}`,
			Want:  `{"method":"isPrime","prime":false}`,
		},
		{
			Name:  "0 is not prime",
			Input: `{"method":"isPrime","number":0}`,
			Want:  `{"method":"isPrime","prime":false}`,
		},
		{
			Name:  "1 is not prime",
			Input: `{"method":"isPrime","number":1}`,
			Want:  `{"method":"isPrime","prime":false}`,
		},
		{
			Name:  "321631 is prime",
			Input: `{"method":"isPrime","number":321631}`,
			// They hit me with 321631
			Want: `{"method":"isPrime","prime":true}`,
		},
		{
			Name:  "321632 is not prime",
			Input: `{"method":"isPrime","number":321621}`,
			Want:  `{"method":"isPrime","prime":false}`,
		},
	}

	conn, err := net.DialTCP("tcp", nil, &addr)
	if err != nil {
		t.Fatalf("Couldn't dial TCP: %s\n", err)
		return
	}
	defer conn.Close()

	scanner := bufio.NewScanner(conn)

	for _, testCase := range tests {
		// Weird scope footgun. Don't want testCase to refer to the previous scope.
		testCase := testCase
		conn := conn
		scanner := scanner
		// Run these *in serial* on the same connection
		t.Run(testCase.Name, func(t *testing.T) {
			_, err := conn.Write([]byte(testCase.Input + "\n"))
			if err != nil {
				t.Fatalf("Error writing to conn: %s\n", err)
			}
			if scanner.Scan() {
				got := scanner.Text()
				if testCase.Want != got {
					t.Fatalf("Want %s, Got %s", testCase.Want, got)
				}
			}
			if scanner.Err() != nil {
				t.Fatalf("Unexpected scanner error: %s", err)
				return
			}
		})
	}
}

// All of these tests should just respond with a shrug and close the conn
func TestMainMalformed(t *testing.T) {
	addr := net.TCPAddr{
		net.ParseIP("127.0.0.1"),
		port, // from main.go
		""}

	t.Parallel()
	tests := []struct {
		Name  string
		Input string
	}{
		{
			Name:  "seven",
			Input: `{"method":"isPrime","number":seven}`,
		},
		{
			Name:  "malformed",
			Input: `malformed`,
		},
		{
			Name:  "unquoted key",
			Input: `{method:"isPrime","number":321621}`,
		},
		{
			Name:  "method missing",
			Input: `{"number":321621}`,
		},
		{
			Name:  "number missing",
			Input: `{"method":"isPrime"}`,
		},
		{
			Name:  "wrong method",
			Input: `{"method":"isntPrime","number":12}`,
		},
	}

	malformed := `¯\_(ツ)_/¯`
	for _, testCase := range tests {
		// Weird scope footgun. Don't want testCase to refer to the previous scope.
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			// Execute in parallel since conn will be killed each time anyway
			t.Parallel()
			// Need a fresh connection each time since we expect it to close
			conn, err := net.DialTCP("tcp", nil, &addr)
			if err != nil {
				t.Fatalf("Couldn't dial TCP: %s\n", err)
				return
			}
			defer conn.Close()
			scanner := bufio.NewScanner(conn)
			_, err = conn.Write([]byte(testCase.Input + "\n"))
			if err != nil {
				t.Fatalf("Error writing to conn: %s\n", err)
			}
			if scanner.Scan() {
				got := scanner.Text()
				if malformed != got {
					t.Fatalf("Want %s, Got %s", malformed, got)
				}
			}
			if scanner.Err() != nil {
				t.Fatalf("Unexpected scanner error: %s", err)
			}
			//TODO Scanner should now be closed!
			// Not sure how to test this conveniently without first setting a deadline
			//if scanner.Scan() {
			//	t.Fatal("Expected connection to be closed")
			//}
		})
	}
}
