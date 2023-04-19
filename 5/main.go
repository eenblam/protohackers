package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"
)

const port = 3335

const tony = "7YWHMfk9JZe0LM0g1ZauHuiSxhI"

var BogusAddress = regexp.MustCompile(`^7[a-zA-Z0-9]{25,34}$`)

// Stupid regexp no lookbehind/lookahead >:(
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
	// Listen
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("Could not listen on port %d: %s", port, err)
	}
	log.Printf("Listening on :%d", port)

	for {
		// Get client
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

		// Create context and channels
		ctx, cancelCtx := context.WithCancel(context.Background())
		// Go handle client
		go toClient(ctx, cancelCtx, client, server)
		go toServer(ctx, cancelCtx, client, server)
	}
}

func toClient(ctx context.Context, cancelCtx context.CancelFunc, client net.Conn, server net.Conn) {
	scanner := bufio.NewScanner(server)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			log.Printf("toClient: unexpected scanner error from server: %s", err)
			cancelCtx()
			return
		}
		got := scanner.Text()
		out := got
		// If it's from server, it has a ] if-and-only-if it's a user-sent message. Split on the first.
		before, message, isMessage := strings.Cut(got, "] ")
		if isMessage {
			// Don't rewrite all data, only the "message" part
			out = before + "] " + Replace(message)
		}
		log.Printf("toClient:\n\tGot [%s]\n\tOut [%s]", got, out)
		_, err := client.Write([]byte(out + "\n"))
		if err != nil {
			cancelCtx()
			return
		}

		select {
		case <-ctx.Done():
			log.Println("toClient closed by context")
			return
		default:
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("toClient: unexpected scanner error from server: %s", err)
	} else {
		log.Println("toServer: socket closed, exiting gracefully")
	}
}

func toServer(ctx context.Context, cancelCtx context.CancelFunc, client net.Conn, server net.Conn) {
	// Close connections here, since client is most likely to terminate under test
	defer client.Close()
	defer server.Close()
	defer log.Println("Connections closed")
	scanner := bufio.NewScanner(client)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			log.Printf("toServer: unexpected scanner error from client: %s", err)
			cancelCtx()
			return
		}
		got := scanner.Text()
		out := Replace(got)
		log.Printf("toServer:\n\tGot [%s]\n\tOut [%s]", got, out)
		_, err := server.Write([]byte(out + "\n"))
		if err != nil {
			cancelCtx()
			return
		}

		select {
		case <-ctx.Done():
			log.Println("toServer closed by context")
			return
		default:
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("toServer: unexpected scanner error from client: %s", err)
	} else {
		log.Println("toServer: socket closed, exiting gracefully")
	}
}

/*
// Lookbehind, lookahead for space
// var middle = regexp.MustCompile(`(?<= )(7[a-zA-Z0-9]{25,34})(?= )`)
// If you match the spaces on both ends, it'll be consumed and mess up ReplaceAll
// Can also do ` (7[a-zA-Z0-9]{25,34})(?= )` using only lookahead
// Sadly, no lookbehind or lookahead in Go's regexp :'(
var middle = regexp.MustCompile(` (7[a-zA-Z0-9]{25,34})(?= )`)
// Just us non-capturing groups for begin and end
var begin = regexp.MustCompile(`^(7[a-zA-Z0-9]{25,34})((?: .*)?)$`)
var end = regexp.MustCompile(`^((?:.* )?)(7[a-zA-Z0-9]{25,34})$`)

func ReplaceOld(s string) string {
	out := begin.ReplaceAllString(s, tony+"${2}")
	out = end.ReplaceAllString(out, "${1}"+tony)
	return middle.ReplaceAllString(out, " "+tony)
}
*/
