package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"strings"
)

const port = 3335

const tony = "7YWHMfk9JZe0LM0g1ZauHuiSxhI"

var BogusAddress = regexp.MustCompile(`^7[a-zA-Z0-9]{25,34}$`)

// Replaces addresses in s with Tony's address
func Replace(s string) string {
	words := strings.Split(s, " ")
	for i, word := range words {
		if BogusAddress.MatchString(word) {
			words[i] = tony
		}
	}
	return strings.Join(words, " ")
}

func main() {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("Could not listen on port %d: %s", port, err)
	}
	log.Printf("Listening on :%d", port)

	for {
		client, err := l.Accept()
		if err != nil {
			log.Printf("Couldn't accept connection: %s", err)
			continue
		}
		server, err := net.Dial("tcp", "chat.protohackers.com:16963")
		if err != nil {
			client.Close()
			log.Printf("Closing client connection. Couldn't connect to server: %s", err)
			continue
		}

		ctx, cancelCtx := context.WithCancel(context.Background())
		go ServerToClient(ctx, cancelCtx, client, server)
		go ClientToServer(ctx, cancelCtx, client, server)
	}
}

func ServerToClient(ctx context.Context, cancelCtx context.CancelFunc, client net.Conn, server net.Conn) {
	reader := bufio.NewReader(server)
	for {
		readbuf, err := reader.ReadBytes('\n')
		switch {
		case err == io.EOF || err == io.ErrUnexpectedEOF:
			log.Println("S2C: exiting due to EOF")
			cancelCtx()
			return
		case err != nil:
			// Not so unexpected here! C2S might close conn before S2C ends.
			log.Printf("S2C: unexpected error: %s", err)
			cancelCtx()
			return
		default:
		}
		got := strings.TrimSuffix(string(readbuf), "\n")
		out := got
		// If it's from server, it has a ] if-and-only-if it's a user-sent message. Split on the first.
		before, message, isMessage := strings.Cut(got, "] ")
		if isMessage {
			// Don't rewrite all data, only the "message" part
			out = before + "] " + Replace(message)
		}
		log.Printf("S2C:\n\tGot [%s]\n\tOut [%s]", got, out)
		_, err = client.Write([]byte(out + "\n"))
		if err != nil {
			cancelCtx()
			return
		}

		select {
		case <-ctx.Done():
			log.Println("S2C: closed by context")
			return
		default:
		}
	}
}

func ClientToServer(ctx context.Context, cancelCtx context.CancelFunc, client net.Conn, server net.Conn) {
	// Close connections here, since client is most likely to terminate under test
	defer client.Close()
	defer server.Close()
	defer log.Println("Connections closed")
	reader := bufio.NewReader(client)
	for {
		readbuf, err := reader.ReadBytes('\n')
		switch {
		case err == io.EOF || err == io.ErrUnexpectedEOF:
			log.Println("C2S: exiting due to EOF")
			cancelCtx()
			return
		case err != nil:
			// Would be a surprise for C2S
			log.Printf("C2S: unexpected error: %s", err)
			cancelCtx()
			return
		default:
		}
		got := strings.TrimSuffix(string(readbuf), "\n")
		out := Replace(got)
		log.Printf("C2S:\n\tGot [%s]\n\tOut [%s]", got, out)
		_, err = server.Write([]byte(out + "\n"))
		if err != nil {
			cancelCtx()
			return
		}

		select {
		case <-ctx.Done():
			log.Println("C2S: closed by context")
			return
		default:
		}
	}
}
