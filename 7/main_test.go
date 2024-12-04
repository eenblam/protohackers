package main

import (
	"bufio"
	"bytes"
	"context"
	cryptoRand "crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	localAddr = "127.0.0.1"
	go main()
	time.Sleep(50 * time.Millisecond)
	v := m.Run()
	os.Exit(v)
}

func TestBasic(t *testing.T) {
	t.Parallel()

	raddr := &net.UDPAddr{
		IP:   net.ParseIP(localAddr),
		Port: 4321,
	}

	// One of several random strings from debugging; kept here for easy debugging of larger messages later.
	dbg, err := hex.DecodeString("32cd1e59865a4764ef70817c4f1bddcd4b2b65f4afa467b1e9d9a8edf5d79f7379c173fa257e6aab5ef9b85c2ff1bba241dc9c44e060810e7ebdd73935fc005347b7f3ac6beb6caa393e0d866db5e73c5c2ff2fc194914c9d0d3db30747ef7d3bf2db3cccb1c863a28917c59e8464007c88e872c71368870c7f08f7561d75695db8567a3a858ed1e68c3ec95446dab4a54cf7cea76c7eb071ed4e41bacc41165e99bbf19cd814a2b011ad5fe07af5edeeb5684db7e025ff1007cbe5499837294aa3e9032125c5cfd53cd400919aa1448d4620ea324eb13cfc23c72631c478b47b9b083681550c5f6969034f2a3c12a37d6abf3166dd8aacb95a060680943d570873118e889b0e89eb6e4b34f0be0463f7de295454be8ef490871581c46b744622abab245bbba724f77c7f8d68f741e5c2f50718b9954db215c5cffa4e71987eb51f5b262683b8353b3f0c0d51c900ebc18a319053da6bce3e66b37185c5c089942f7d5fe803989f17bd9f8ab59649039a1d6ce93a8a8f0cfcb212167577be7fe0c54aea5a49abd339bb13ce95891b63ea989b539d13d72d6f2f418a653c4e1c19e49805dee73e3ce61ded478f034d058446845a3476ebc6a051dbfa9ad1c49fdfd4f6334343a6f33d11c9bd71d1c1fd923856649b125284ab4c398227bc86af5df27a7d61ea6a80781ec9da4f5e45f6294a9a7aea3edf0cc99e8246af0b0740c0b2ae132f3fe62557c5bbb2d34bd9ba06c25ccf254a32be368b49e634cc6d35464208a9679676771530990f5d989b7e216efa06551daf7d54acfdfd3695106d521447baaa533fa45da76d670dfbcea70001db86dceef9ab3eefe77c34abd343ded9317c358048fa84c9cb2773907f19c8ced7a7998c565b71e804c56c51e67141e6aab6a8ac85bec5f6418a482cd1d11f6db239c5bc9aa798c77edeb708e7a4f31cd170374b45d58523ee6bbf9d393bacd3f57b7e2e7b0e9ea752a8273af3bc7178ac922bae5bfeefc1f2a54ac82a6aa2cb1378dd6d85fa73f72e958f71f4c0c1c3941f6fc83a2218c53358a28fbd993bc0d905c2fe9c7b8ad9d82b00be1d8efd69509f70be3102efa50d8e35f81c059aa35b1cc738231d0639f919409c176305c2f6426dc7efaf280c672b79dab71219e615e8faac5379fde22d50309d6d770252b795edd851edeb9d3ca9b11baceb140162de7743fba4834cdf1a621921cb612185f6f8b379f962d16e4e72e8d61d387b1ca0cdb98cc93408ad79960529b85a45e90cb74c7a1f7ee6eaee53c1dbfa61aaf155309c98007def17b65367413e5d4736d467ef7e1f1daf1d337f94d5c5c3b280e8bf67a602a917e9afc168260b9500e4c0bb6f124813a93c21ee2d2095c2f")
	if err != nil {
		t.Fatal(err)
	}
	dbgWant := slices.Clone(dbg)
	slices.Reverse(dbgWant)
	dbg = append(dbg, '\n')
	dbgWant = append(dbgWant, '\n')

	cases := []struct {
		Name  string
		Input []byte
		Want  []byte
	}{
		{
			Name: "Simple test",
			// Use "" (interpreted literal) not `` (raw literal) here for proper newlines
			// (otherwise we have to literally create a linebreak)
			Input: []byte("asdf\nqwer\n\n"),
			Want:  []byte("fdsa\nrewq\n\n"),
		},
		{
			Name:  "Test escapes", // Shouldn't interpret as \n
			Input: []byte("asdf\\nqwer\n"),
			Want:  []byte("rewqn\\fdsa\n"),
		},
		{
			Name:  "Test forward /",
			Input: []byte("asdf/nqwer\n"),
			Want:  []byte("rewqn/fdsa\n"),
		},
		{
			// Server shouldn't strip carriage returns; protocol is newline only.
			// (Default behavior for a Scanner is to strip \r from `LINE\r\n`)
			Name:  "Test carriage return isn't dropped",
			Input: []byte("abcd\r\nabcd\n"),
			Want:  []byte("\rdcba\ndcba\n"),
		},
		{
			Name:  "Test debug string",
			Input: dbg,
			Want:  dbgWant,
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
				t.Fatalf("unequal bytes; got [%x] != want [%x]", buf, testCase.Want)
			}
			s.Close()
		})

	}
}

func TestBadLink(t *testing.T) {

	// Goal: generate a lot of data, write it, reverse it, scan it back

	maxData := maxInt >> 16

	// Generate data
	log.Println(`TestBadLink: generating data`)
	lines := make([][]byte, 0)
	scanner := bufio.NewScanner(&RandReader{})
	var bs, line []byte
	for i := 0; i < maxData; {
		scanner.Scan()
		bs = scanner.Bytes()
		// Don't collect more than maxData bytes
		if i+len(bs)+1 > maxData { // +1 to account for missing newline
			break
		}
		// Underlying array of bs may be overwritten by scan.
		// Make a copy and add \n to the end.
		line = make([]byte, len(bs)+1)
		copy(line, bs)
		line[len(line)-1] = '\n'
		i += len(line)
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf(`unexpected scanner error: %v`, err)
	}

	// Connect to server via proxy
	log.Println(`TestBadLink: connecting to proxy`)
	proxyAddr := &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 9876,
	}
	serverAddr := &net.UDPAddr{
		IP:   net.ParseIP(localAddr),
		Port: 4321,
	}
	_, err := NewBadProxy(
		serverAddr,
		proxyAddr,
		25, // 25% failure rate
	)
	if err != nil {
		t.Fatalf(`failed to create proxy server: %v`, err)
	}
	time.Sleep(500 * time.Millisecond)
	session, err := DialLRCP("lrcp", nil, proxyAddr)
	// If you want to bypass the proxy...
	//session, err := DialLRCP("lrcp", nil, serverAddr)
	if err != nil {
		t.Fatalf(`failed to dial proxy server: %v`, err)
	}

	writeStatusCh := make(chan bool)
	// Write data
	go func() {
		log.Println(`TestBadLink: writing data`)
		var wrote int
		for _, line := range lines {
			wrote = 0
			for wrote < len(line) {
				n, err := session.Write(line[wrote:])
				if err != nil {
					// Can't t.Fatalf in separate goroutine
					log.Printf(`TestBadLink: failed to write to proxy server: %v`, err)
					writeStatusCh <- false
					return
				}
				wrote += n
			}

		}
		writeStatusCh <- true
	}()

	log.Println(`TestBadLink: receiving and checking results`)
	scanner = bufio.NewScanner(session)
	scanner.Buffer(make([]byte, 65536), maxInt)
	scanner.Split(ScanLinesNoCR)
	var want []byte
	for i := 0; i < len(lines) && scanner.Scan(); i++ {
		want = lines[i]
		want = want[:len(want)-1] // Strip newline
		got := scanner.Bytes()
		slices.Reverse(want)
		if !bytes.Equal(want, got) {
			t.Fatalf(`unequal bytes; got [%x] != want [%x]`, got, want)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf(`unexpected scanner error: %v`, err)
	}
	writeStatusTimeout := time.NewTicker(1 * time.Second)
	select {
	case <-writeStatusTimeout.C:
		t.Fatalf(`timed out waiting for write goroutine to finish`)
	case writeStatus := <-writeStatusCh:
		if !writeStatus {
			t.Fatalf(`received error from write goroutine`)
		}
	}
}

// RandReader provides a random Read() method in order to provide
// a struct we can pass to a scanner
type RandReader struct{}

func (r *RandReader) Read(p []byte) (int, error) {
	//TODO replace this with math/rand/v2/ChaCha8.Read after updating to go 1.23
	// for deterministic output
	return cryptoRand.Read(p)
}

type BadProxy struct {
	ListenAddr *net.UDPAddr
	ServerAddr *net.UDPAddr
	FailRate   int
	Clients    sync.Map
	BufferPool sync.Pool
}

func NewBadProxy(serverAddr, listenAddr *net.UDPAddr, failRate int) (*BadProxy, error) {
	if failRate > 99 {
		return nil, fmt.Errorf("proxy has failure rate [%d] > 99; no traffic can pass.", failRate)
	} else if failRate < 1 {
		return nil, fmt.Errorf("proxy has failure rate [%d] < 1; Intn will panic.", failRate)
	}
	b := &BadProxy{
		ListenAddr: listenAddr,
		ServerAddr: serverAddr,
		FailRate:   failRate,
		BufferPool: sync.Pool{
			New: func() interface{} {
				b := make([]byte, 65535) // Max UDP packet size of 2**16
				return &b
			},
		},
	}
	go b.listen()
	return b, nil
}

// badProxy will listen on two addresses, fowarding packets between the two,
// dropping an average of (failRate/100) packets at random.
// Currently a dumb proxy that only supports a single client for simplicity.
// TODO context to cancel all goroutines
func (b *BadProxy) listen() {
	listenConn, err := net.ListenUDP("udp", b.ListenAddr)
	if err != nil {
		panic(err)
	}
	defer listenConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Forward to server
	forward := func(ctx context.Context, serverConn *net.UDPConn, clientAddr *net.UDPAddr, ch chan *[]byte) {
		defer serverConn.Close()
		defer b.Clients.Delete(clientAddr.String())

		//TODO I need to signal to other goroutine if I exit
		for {
			select {
			case <-ctx.Done():
				return
			case bufPtr := <-ch:
				// Forward to server
				for n := len(*bufPtr); n > 0; {
					wrote, err := serverConn.Write(*bufPtr)
					if err != nil {
						log.Printf(`badProxy: write error to [%v]: %v`, serverConn.RemoteAddr().String(), err)
						break
					}
					n -= wrote
				}
				b.BufferPool.Put(bufPtr)
				continue
			}
		}
	}
	reverse := func(ctx context.Context, serverConn *net.UDPConn, clientAddr *net.UDPAddr) {
		defer serverConn.Close()

		buf := make([]byte, 65535) // Max UDP size (2**16)
		// Listen for packets from server
		// Server (connected) to proxy client (not connected)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			// Get a packet
			buf = buf[:cap(buf)]
			n, err := serverConn.Read(buf)
			if err != nil {
				log.Printf(`badProxy: read error from [%v]: %v`, b.ServerAddr, err)
			}
			buf = buf[:n]

			// Roll the dice (1-100)
			if rand.Intn(100)+1 <= b.FailRate {
				continue
			}
			// Forward
			for n > 0 {
				wrote, err := listenConn.WriteTo(buf, clientAddr)
				if err != nil {
					log.Printf(`badProxy: write error to [%v]: %v`, clientAddr, err)
				}
				n -= wrote
			}
		}

	}

	// Get a packet
	// Pass packet to forward goroutine
	var buf *[]byte
	for {
		buf = b.BufferPool.Get().(*[]byte)
		*buf = (*buf)[:65535] //TODO move to const
		// Read a packet
		n, clientAddr, err := listenConn.ReadFrom(*buf)
		if err != nil {
			b.BufferPool.Put(&buf)
			log.Printf(`badProxy: read error from [%v]: %v`, clientAddr, err)
			continue
		}
		*buf = (*buf)[:n]
		// Roll the dice (1-100)
		if rand.Intn(100)+1 <= b.FailRate {
			b.BufferPool.Put(buf)
			continue
		}
		// Check client map
		ch := make(chan *[]byte, 1) //TODO could use another pool for these
		actualCh, loaded := b.Clients.LoadOrStore(clientAddr.String(), ch)
		if !loaded { // Kick off goroutines
			serverConn, err := net.DialUDP("udp", nil, b.ServerAddr)
			if err != nil {
				log.Println(err)
				return
			}
			go forward(ctx, serverConn, clientAddr.(*net.UDPAddr), actualCh.(chan *[]byte))
			go reverse(ctx, serverConn, clientAddr.(*net.UDPAddr))
		}
		// Forward packet to handler to be written to server
		actualCh.(chan *[]byte) <- buf
	}
}
