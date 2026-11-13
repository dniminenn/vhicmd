package api

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"image/color"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// clientFrames presents the payload of a websocket client's masked frames as one stream.
type clientFrames struct {
	rd     *bufio.Reader
	mask   [4]byte
	remain int
	pos    int
}

func (c *clientFrames) Read(p []byte) (int, error) {
	for c.remain == 0 {
		head := make([]byte, 2)
		if _, err := io.ReadFull(c.rd, head); err != nil {
			return 0, err
		}
		c.remain = int(head[1] & 0x7f)
		if c.remain == 126 {
			ext := make([]byte, 2)
			if _, err := io.ReadFull(c.rd, ext); err != nil {
				return 0, err
			}
			c.remain = int(binary.BigEndian.Uint16(ext))
		}
		if _, err := io.ReadFull(c.rd, c.mask[:]); err != nil {
			return 0, err
		}
		c.pos = 0
	}
	if len(p) > c.remain {
		p = p[:c.remain]
	}
	n, err := c.rd.Read(p)
	for i := 0; i < n; i++ {
		p[i] ^= c.mask[c.pos%4]
		c.pos++
	}
	c.remain -= n
	return n, err
}

type keyPress struct {
	Keysym uint32
	Down   bool
}

// fakeVNC is a one-connection noVNC endpoint with a 2x1 screen. It records key events.
type fakeVNC struct {
	ln   net.Listener
	path chan string
	keys chan keyPress
}

func newFakeVNC(t *testing.T) *fakeVNC {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeVNC{ln: ln, path: make(chan string, 1), keys: make(chan keyPress, 64)}
	t.Cleanup(func() { ln.Close() })
	go f.serve()
	return f
}

func (f *fakeVNC) serve() {
	conn, err := f.ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	rd := bufio.NewReader(conn)
	req, err := http.ReadRequest(rd)
	if err != nil {
		return
	}
	f.path <- req.URL.RequestURI()
	sum := sha1.Sum([]byte(req.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Accept: "+base64.StdEncoding.EncodeToString(sum[:])+"\r\n\r\n")

	send := func(b []byte) { conn.Write(append([]byte{0x82, byte(len(b))}, b...)) }
	in := &clientFrames{rd: rd}
	read := func(n int) []byte {
		buf := make([]byte, n)
		if _, err := io.ReadFull(in, buf); err != nil {
			return nil
		}
		return buf
	}

	send([]byte("RFB 003.008\n"))
	read(12)
	send([]byte{1, 1})
	read(1)
	send([]byte{0, 0, 0, 0})
	read(1)
	// A ping in the middle of the stream must be answered and skipped.
	conn.Write([]byte{0x89, 0})
	send(append([]byte{0, 2, 0, 1, 32, 24, 0, 1, 0, 255, 0, 255, 0, 255, 16, 8, 0, 0, 0, 0, 0, 0, 0, 4}, "fake"...))

	for {
		kind := read(1)
		if kind == nil {
			return
		}
		switch kind[0] {
		case 0:
			read(19)
		case 2:
			head := read(3)
			read(4 * int(binary.BigEndian.Uint16(head[1:])))
		case 3:
			read(9)
			send([]byte{2}) // bell first
			update := []byte{0, 0, 0, 1, 0, 0, 0, 0, 0, 2, 0, 1, 0, 0, 0, 0}
			update = append(update, 3, 2, 1, 0, 30, 20, 10, 0) // two pixels, blue first
			send(update)
		case 4:
			msg := read(7)
			f.keys <- keyPress{binary.BigEndian.Uint32(msg[3:]), msg[0] == 1}
		}
	}
}

func (f *fakeVNC) url() string {
	return "http://" + f.ln.Addr().String() + "/vnc_auto.html?path=websockify%3Ftoken%3Dabc"
}

func (f *fakeVNC) pressed(n int) []keyPress {
	got := make([]keyPress, n)
	for i := range got {
		got[i] = <-f.keys
	}
	return got
}

func TestConsoleTypeAndKeys(t *testing.T) {
	f := newFakeVNC(t)
	c, err := DialConsole(f.url())
	if err != nil {
		t.Fatalf("DialConsole: %v", err)
	}
	defer c.Close()
	c.KeyDelay = 0

	if path := <-f.path; path != "/websockify?token=abc" {
		t.Errorf("websocket path = %q", path)
	}
	if c.Width != 2 || c.Height != 1 || c.Name != "fake" {
		t.Errorf("screen = %dx%d %q", c.Width, c.Height, c.Name)
	}

	if err := c.Type("aB|\n"); err != nil {
		t.Fatalf("Type: %v", err)
	}
	want := []keyPress{
		{'a', true}, {'a', false},
		{keyShift, true}, {'B', true}, {'B', false}, {keyShift, false},
		{keyShift, true}, {'|', true}, {'|', false}, {keyShift, false},
		{keyEnter, true}, {keyEnter, false},
	}
	if got := f.pressed(len(want)); !reflect.DeepEqual(got, want) {
		t.Errorf("typed keys = %v, want %v", got, want)
	}

	if err := c.PressKey("ctrl-alt-delete"); err != nil {
		t.Fatalf("PressKey: %v", err)
	}
	want = []keyPress{
		{keyControl, true}, {keyAlt, true}, {0xffff, true}, {0xffff, false}, {keyAlt, false}, {keyControl, false},
	}
	if got := f.pressed(len(want)); !reflect.DeepEqual(got, want) {
		t.Errorf("pressed keys = %v, want %v", got, want)
	}
}

func TestConsoleTypeRejectsUntypeable(t *testing.T) {
	f := newFakeVNC(t)
	c, err := DialConsole(f.url())
	if err != nil {
		t.Fatalf("DialConsole: %v", err)
	}
	defer c.Close()

	if err := c.Type("ok é"); err == nil {
		t.Fatal("expected an error for a non-ASCII character")
	}
	select {
	case k := <-f.keys:
		t.Errorf("key %v was sent before the bad character was noticed", k)
	default:
	}
}

func TestConsoleScreenshot(t *testing.T) {
	f := newFakeVNC(t)
	c, err := DialConsole(f.url())
	if err != nil {
		t.Fatalf("DialConsole: %v", err)
	}
	defer c.Close()

	img, err := c.Screenshot()
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if got := img.RGBAAt(0, 0); got != (color.RGBA{1, 2, 3, 255}) {
		t.Errorf("pixel 0 = %v", got)
	}
	if got := img.RGBAAt(1, 0); got != (color.RGBA{10, 20, 30, 255}) {
		t.Errorf("pixel 1 = %v", got)
	}
}

func TestConsoleSocket(t *testing.T) {
	cases := []struct {
		in, host, path string
		tls            bool
	}{
		{"https://panel:6080/vnc_auto.html?path=websockify%3Ftoken%3Dabc&token=x", "panel:6080", "/websockify?token=abc", true},
		{"https://panel/vnc_lite.html?token=abc", "panel:443", "/websockify?token=abc", true},
		{"http://panel:6080/vnc.html?path=%3Ftoken%3Dabc", "panel:6080", "/?token=abc", false},
	}
	for _, tc := range cases {
		host, useTLS, path, err := consoleSocket(tc.in)
		if err != nil || host != tc.host || path != tc.path || useTLS != tc.tls {
			t.Errorf("consoleSocket(%q) = %q, %v, %q, %v", tc.in, host, useTLS, path, err)
		}
	}
	for _, bad := range []string{"ftp://panel/x?token=a", "https://panel/vnc.html"} {
		if _, _, _, err := consoleSocket(bad); err == nil {
			t.Errorf("consoleSocket(%q) did not fail", bad)
		}
	}
}

func TestParseKey(t *testing.T) {
	keysym, mods, err := parseKey("Ctrl-C")
	if err != nil || keysym != 'c' || !reflect.DeepEqual(mods, []uint32{keyControl}) {
		t.Errorf("parseKey(Ctrl-C) = %#x, %v, %v", keysym, mods, err)
	}
	if keysym, _, err := parseKey("-"); err != nil || keysym != '-' {
		t.Errorf("parseKey(-) = %#x, %v", keysym, err)
	}
	for _, bad := range []string{"super-c", "nosuchkey", ""} {
		if _, _, err := parseKey(bad); err == nil || !strings.Contains(err.Error(), "unknown") {
			t.Errorf("parseKey(%q) error = %v", bad, err)
		}
	}
}
