package main

import (
	"bytes"
	"strconv"
	"testing"
)

func TestParseField(t *testing.T) {
	cases := []struct {
		name     string
		in       []byte
		want     []byte
		wantRest []byte
		wantErr  bool
	}{
		{
			name:    "error on empty input",
			in:      []byte{},
			wantErr: true,
		},
		{
			name:     "parse an empty field",
			in:       []byte(`/`),
			want:     []byte(``),
			wantRest: []byte(``),
			wantErr:  false,
		},
		{
			name:     "parse a single field",
			in:       []byte(`field/`),
			want:     []byte(`field`),
			wantRest: []byte{},
			wantErr:  false,
		},
		{
			name:     "parse multiple fields",
			in:       []byte(`field1/field2/`),
			want:     []byte(`field1`),
			wantRest: []byte(`field2/`),
			wantErr:  false,
		},
		{
			name:     "ignore escaped slashes",
			in:       []byte(`fie\/ld\\1/field2/`),
			want:     []byte(`fie\/ld\\1`),
			wantRest: []byte(`field2/`),
			wantErr:  false,
		},
		{
			name:     "escaped backslash doesn't escape subsequent slash",
			in:       []byte(`field\\/rest/`),
			want:     []byte(`field\\`),
			wantRest: []byte(`rest/`),
			wantErr:  false,
		},
		{
			name:     "escaped backslash doesn't escape final slash", // funny edge case
			in:       []byte(`field\\/`),
			want:     []byte(`field\\`),
			wantRest: []byte(``),
			wantErr:  false,
		},
		{
			name:    "error on non-escape backslash",
			in:      []byte(`fie\ld/rest/`),
			wantErr: true,
		},
		{
			name:    "error on non-terminated field", // (missing /)
			in:      []byte(`field`),
			wantErr: true,
		},
		{
			name:    "error when only slash is escaped",
			in:      []byte(`field\/`),
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, rest, err := parseField(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, c.want) {
				t.Fatalf("unexpected value: got %s, want %s", got, c.want)
			}
			if !bytes.Equal(rest, c.wantRest) {
				t.Fatalf("unexpected remainder: got %s, want %s", rest, c.wantRest)
			}
		})
	}
}

func TestParseInt(t *testing.T) {
	cases := []struct {
		name    string
		in      []byte
		want    int
		wantErr bool
	}{
		{
			name:    "error on empty input",
			in:      []byte{},
			wantErr: true,
		},
		{
			name:    "success on 0",
			in:      []byte(`0`),
			want:    0,
			wantErr: false,
		},
		{
			name:    "success up to maxInt",
			in:      []byte(`2147483647`),
			want:    2147483647,
			wantErr: false,
		},
		{
			name:    "error if input exceeds maxInt",
			in:      []byte(`2147483648`),
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseInt(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("unexpected value: got %d, want %d", got, c.want)
			}
		})
	}
}

func TestParseData(t *testing.T) {
	cases := []struct {
		name    string
		in      []byte
		want    []byte
		wantErr bool
	}{
		{
			name:    "parse empty data",
			in:      []byte(``),
			want:    []byte{},
			wantErr: false,
		},
		{
			name:    "parse non-empty data",
			in:      []byte(`data`),
			want:    []byte(`data`),
			wantErr: false,
		},
		{
			name:    "parse escaped slash and backslash",
			in:      []byte(`d\\a\/ta`),
			want:    []byte(`d\a/ta`),
			wantErr: false,
		},
		{
			name:    "parse consecutive escaped slashes",
			in:      []byte(`d\\\\\/a\/ta`),
			want:    []byte(`d\\/a/ta`),
			wantErr: false,
		},
		{
			name:    "error on unescaped slash",
			in:      []byte(`da/ta`),
			wantErr: true,
		},
		{
			name:    "error on uneven escaping",
			in:      []byte(`da\\\ta`),
			wantErr: true,
		},
		{
			name:    "error on unescaped backslash",
			in:      []byte(`da\ta`),
			wantErr: true,
		},
		{
			name:    "error on final unescaped backslash",
			in:      []byte(`data\`),
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseData(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, c.want) {
				t.Fatalf("unexpected value: got %v, want %v", got, c.want)
			}
		})
	}
}

func TestParseMessage(t *testing.T) {
	cases := []struct {
		name    string
		in      []byte
		want    *Msg
		wantErr bool
	}{
		{
			name:    "error on empty input",
			in:      []byte{},
			wantErr: true,
		},
		{
			name:    "error on missing leading slash",
			in:      []byte(`connect/1234/`),
			wantErr: true,
		},
		{
			name:    "error on missing trailing slash",
			in:      []byte(`/connect/1234`),
			wantErr: true,
		},
		{
			name:    "error on missing delimiting slash / bad type",
			in:      []byte(`/connect1234`),
			wantErr: true,
		},
		{
			name:    "error on non-numeric session",
			in:      []byte(`/connect/abc/`),
			wantErr: true,
		},
		{
			name:    "error on missing data",
			in:      []byte(`field/1/`),
			wantErr: true,
		},
		{
			name:    "parse connect",
			in:      []byte(`/connect/1234/`),
			want:    &Msg{Type: "connect", Session: 1234},
			wantErr: false,
		},
		{
			name:    "parse ack",
			in:      []byte(`/ack/1234/10/`),
			want:    &Msg{Type: "ack", Session: 1234, Length: 10},
			wantErr: false,
		},
		{
			name:    "parse close",
			in:      []byte(`/close/1234/`),
			want:    &Msg{Type: "close", Session: 1234},
			wantErr: false,
		},
		{
			name:    "parse data with single byte",
			in:      []byte(`/data/1234/10/a/`),
			want:    &Msg{Type: "data", Session: 1234, Pos: 10, Data: []byte(`a`)},
			wantErr: false,
		},
		{
			name:    "parse data",
			in:      []byte(`/data/1234/10/abc/`),
			want:    &Msg{Type: "data", Session: 1234, Pos: 10, Data: []byte(`abc`)},
			wantErr: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseMessage(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Type != c.want.Type {
				t.Fatalf("unexpected type: got %s, want %s", got.Type, c.want.Type)
			}
			if got.Session != c.want.Session {
				t.Fatalf("unexpected session: got %d, want %d", got.Session, c.want.Session)
			}
			switch c.want.Type {
			case "ack":
				if got.Length != c.want.Length {
					t.Fatalf("unexpected length: got %d, want %d", got.Length, c.want.Length)
				}
			case "data":
				if got.Pos != c.want.Pos {
					t.Fatalf("unexpected pos: got %d, want %d", got.Pos, c.want.Pos)
				}
				if !bytes.Equal(got.Data, c.want.Data) {
					t.Fatalf(`unexpected data: got "%s", want "%s"`, got.Data, c.want.Data)
				}
			}
		})
	}
}

func TestMessageValidate(t *testing.T) {
	cases := []struct {
		name    string
		msg     *Msg
		wantErr bool
	}{
		{
			name: "error when data limit exceeded",
			// maxInt-2 to maxInt+1
			msg: &Msg{
				Type:    "data",
				Session: 1234,
				Pos:     maxInt - 2,
				Data:    []byte("abc"),
				Length:  0,
			},
			wantErr: true,
		},
		{
			name: "error when data Pos too large",
			msg: &Msg{
				Type:    "data",
				Session: 1234,
				Pos:     maxInt + 1,
				Data:    []byte(""),
				Length:  0,
			},
			wantErr: true,
		},
		{
			name: "error when ack Length too large",
			msg: &Msg{
				Type:    "ack",
				Session: 1234,
				Pos:     0,
				Data:    []byte{},
				Length:  maxInt + 1,
			},
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.msg.Validate()
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestEncode(t *testing.T) {
	cases := []struct {
		Description string
		Msg         Msg
		Data        []byte
		Want        []byte
		WantError   bool
	}{
		{
			Description: "connect",
			Msg: Msg{
				Type:    "connect",
				Session: 1234,
			},
			Want: []byte(`/connect/1234/`),
		},
		{
			Description: "ack",
			Msg: Msg{
				Type:    "ack",
				Session: 1234,
				Length:  0,
			},
			Want: []byte(`/ack/1234/0/`),
		},
		{
			Description: "data",
			Msg: Msg{
				Type:    "data",
				Session: 1234,
				Pos:     0,
				Data:    []byte(`abc`),
			},
			Want: []byte(`/data/1234/0/abc/`),
		},
		{
			Description: "Errors on unknown type",
			Msg: Msg{
				Type: "unknown",
			},
			WantError: true,
		},
	}
	for _, test := range cases {
		t.Run(test.Description, func(t *testing.T) {
			buf := make([]byte, maxMessageSize)
			n, err := test.Msg.encode(buf)
			if test.WantError {
				if err == nil {
					t.Fatalf("Expected error but got none")
				}
				return
			}
			got := buf[:n]
			if err != nil {
				t.Fatalf("Unexpected error %s", err)
			}
			if !bytes.Equal(test.Want, got) {
				t.Fatalf("Want [%s] Got [%s]", test.Want, got)
			}
		})
	}
}

func TestPack(t *testing.T) {
	aaa := func(n int) []byte {
		b := make([]byte, n)
		for i := 0; i < n; i++ {
			b[i] = 'a'
		}
		return b
	}
	cases := []struct {
		name     string
		session  int
		pos      int
		data     []byte
		wantN    int
		wantData []byte
	}{
		{
			name:    "empty data",
			session: 1234,
			pos:     0,
			data:    []byte{},
			wantN:   0,
		},
		{
			name:     "single byte",
			session:  1234,
			pos:      0,
			data:     []byte{0x01},
			wantN:    1,
			wantData: []byte{0x01},
		},
		{
			name:     "we can't actually fit a buffer of maxMessageSize",
			session:  1234, // 4
			pos:      56,   // 2
			data:     aaa(maxMessageSize),
			wantN:    maxMessageSize - 9 - 4 - 2,
			wantData: aaa(maxMessageSize - 9 - 4 - 2),
		},
		{
			name: "greatest possible metadata size",
			// Max out the lengths of string(session) and string(pos); this is the largest metadata a /data/ packet can have.
			session: maxInt,
			// pack doesn't care if we're writing beyond beyond maximal protocol capacity, that's for Validate() to decide.
			// (I.e. it's fine if *sending* this message would mean sending too many bytes.)
			pos:      maxInt,
			data:     aaa(maxMessageSize),
			wantN:    maxMessageSize - 9 - 2*len(strconv.Itoa(maxInt)),
			wantData: aaa(maxMessageSize - 9 - 2*len(strconv.Itoa(maxInt))),
		},
		{
			name:     "slashes",
			session:  1234, // 4
			pos:      56,   // 2
			data:     []byte(`abc/def/ghi\jkl\mno`),
			wantN:    19,
			wantData: []byte(`abc\/def\/ghi\\jkl\\mno`),
		},
		{
			name:     "don't write final slash if we can't escape it",
			session:  1234,                                     // 4
			pos:      56,                                       // 2
			data:     append(aaa(maxMessageSize-9-4-2-1), '/'), // 9-4-2 for metadata, -1 for final slash
			wantN:    maxMessageSize - 9 - 4 - 2 - 1,
			wantData: aaa(maxMessageSize - 9 - 4 - 2 - 1),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := Msg{Session: c.session, Pos: c.pos}
			n := m.pack(c.data)
			if !bytes.Equal(m.Data, c.wantData) {
				t.Fatalf("unexpected data: got %v, want %v", m.Data, c.wantData)
			}
			if n != c.wantN {
				t.Fatalf("unexpected number of bytes packed: got %d, want %d", n, c.wantN)
			}
		})
	}
}
