package api

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// consoleIOTimeout bounds every read and write on a console session.
const consoleIOTimeout = 30 * time.Second

// X11 keysyms for the keys that are not printable ASCII.
const (
	keyBackSpace = 0xff08
	keyTab       = 0xff09
	keyEnter     = 0xff0d
	keyEscape    = 0xff1b
	keyShift     = 0xffe1
	keyControl   = 0xffe3
	keyAlt       = 0xffe9
)

var namedKeys = map[string]uint32{
	"enter": keyEnter, "return": keyEnter, "tab": keyTab, "esc": keyEscape, "escape": keyEscape,
	"backspace": keyBackSpace, "space": ' ', "delete": 0xffff, "insert": 0xff63,
	"home": 0xff50, "left": 0xff51, "up": 0xff52, "right": 0xff53, "down": 0xff54,
	"pageup": 0xff55, "pagedown": 0xff56, "end": 0xff57,
	"f1": 0xffbe, "f2": 0xffbf, "f3": 0xffc0, "f4": 0xffc1, "f5": 0xffc2, "f6": 0xffc3,
	"f7": 0xffc4, "f8": 0xffc5, "f9": 0xffc6, "f10": 0xffc7, "f11": 0xffc8, "f12": 0xffc9,
}

var modifierKeys = map[string]uint32{"ctrl": keyControl, "shift": keyShift, "alt": keyAlt}

// shiftedKeys are the printable characters a US keyboard only produces with Shift held.
const shiftedKeys = `~!@#$%^&*()_+{}|:"<>?`

// Console is a VNC session to a VM's console, carried over the noVNC websocket.
type Console struct {
	conn   net.Conn
	rd     *bufio.Reader
	Width  int
	Height int
	Name   string
	// KeyDelay is the pause after every key event; the guest drops keys sent faster than it reads them.
	KeyDelay time.Duration
}

// consoleSocket turns a noVNC page URL into the host, TLS flag and websocket path behind it.
func consoleSocket(consoleURL string) (host string, useTLS bool, path string, err error) {
	u, err := url.Parse(consoleURL)
	if err != nil {
		return "", false, "", fmt.Errorf("invalid console URL: %v", err)
	}
	switch u.Scheme {
	case "https", "wss":
		useTLS = true
	case "http", "ws":
	default:
		return "", false, "", fmt.Errorf("console URL has unsupported scheme %q", u.Scheme)
	}
	host = u.Host
	if u.Port() == "" {
		if useTLS {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	q := u.Query()
	switch {
	case q.Get("path") != "":
		path = "/" + strings.TrimPrefix(q.Get("path"), "/")
	case q.Get("token") != "":
		path = "/websockify?token=" + url.QueryEscape(q.Get("token"))
	default:
		return "", false, "", fmt.Errorf("console URL has neither a path nor a token")
	}
	return host, useTLS, path, nil
}

// DialConsole opens a VNC session from the URL returned by GetConsoleURL.
func DialConsole(consoleURL string) (*Console, error) {
	host, useTLS, path, err := consoleSocket(consoleURL)
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{Timeout: consoleIOTimeout}
	var conn net.Conn
	scheme := "http"
	if useTLS {
		scheme = "https"
		conn, err = tls.DialWithDialer(dialer, "tcp", host, nil)
	} else {
		conn, err = dialer.Dial("tcp", host)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the console: %v", err)
	}

	ws, err := websocketUpgrade(conn, host, path, scheme+"://"+host)
	if err != nil {
		conn.Close()
		return nil, err
	}
	c := &Console{conn: conn, rd: bufio.NewReaderSize(ws, 1<<16), KeyDelay: 20 * time.Millisecond}
	if err := c.handshake(); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

// Close ends the session.
func (c *Console) Close() error {
	return c.conn.Close()
}

func (c *Console) read(n int) ([]byte, error) {
	c.conn.SetReadDeadline(time.Now().Add(consoleIOTimeout))
	buf := make([]byte, n)
	if _, err := io.ReadFull(c.rd, buf); err != nil {
		return nil, fmt.Errorf("console read failed: %v", err)
	}
	return buf, nil
}

func (c *Console) write(b []byte) error {
	c.conn.SetWriteDeadline(time.Now().Add(consoleIOTimeout))
	if err := websocketWrite(c.conn, b); err != nil {
		return fmt.Errorf("console write failed: %v", err)
	}
	return nil
}

// handshake runs the RFB 3.8 handshake. The websocket token is the authentication,
// so the server must offer security type None.
func (c *Console) handshake() error {
	version, err := c.read(12)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(version), "RFB ") {
		return fmt.Errorf("console did not answer as a VNC server: %q", version)
	}
	if err := c.write([]byte("RFB 003.008\n")); err != nil {
		return err
	}

	count, err := c.read(1)
	if err != nil {
		return err
	}
	if count[0] == 0 {
		return fmt.Errorf("console refused the connection: %s", c.readReason())
	}
	types, err := c.read(int(count[0]))
	if err != nil {
		return err
	}
	if !strings.Contains(string(types), "\x01") {
		return fmt.Errorf("console offers no unauthenticated VNC security type (offered %v)", types)
	}
	if err := c.write([]byte{1}); err != nil {
		return err
	}
	result, err := c.read(4)
	if err != nil {
		return err
	}
	if binary.BigEndian.Uint32(result) != 0 {
		return fmt.Errorf("console rejected the VNC handshake: %s", c.readReason())
	}

	// ClientInit: shared, so a person watching the same console stays connected.
	if err := c.write([]byte{1}); err != nil {
		return err
	}
	init, err := c.read(24)
	if err != nil {
		return err
	}
	c.Width = int(binary.BigEndian.Uint16(init[0:2]))
	c.Height = int(binary.BigEndian.Uint16(init[2:4]))
	name, err := c.read(int(binary.BigEndian.Uint32(init[20:24])))
	if err != nil {
		return err
	}
	c.Name = string(name)
	return nil
}

func (c *Console) readReason() string {
	length, err := c.read(4)
	if err != nil {
		return "no reason given"
	}
	reason, err := c.read(int(binary.BigEndian.Uint32(length)))
	if err != nil {
		return "no reason given"
	}
	return string(reason)
}

func (c *Console) keyEvent(keysym uint32, down bool) error {
	msg := []byte{4, 0, 0, 0, 0, 0, 0, 0}
	if down {
		msg[1] = 1
	}
	binary.BigEndian.PutUint32(msg[4:], keysym)
	if err := c.write(msg); err != nil {
		return err
	}
	time.Sleep(c.KeyDelay)
	return nil
}

// press sends one key with the given modifiers held around it.
func (c *Console) press(keysym uint32, modifiers ...uint32) error {
	for _, m := range modifiers {
		if err := c.keyEvent(m, true); err != nil {
			return err
		}
	}
	if err := c.keyEvent(keysym, true); err != nil {
		return err
	}
	if err := c.keyEvent(keysym, false); err != nil {
		return err
	}
	for i := len(modifiers) - 1; i >= 0; i-- {
		if err := c.keyEvent(modifiers[i], false); err != nil {
			return err
		}
	}
	return nil
}

// typedKey returns the keysym for a character and whether a US keyboard needs Shift for it.
func typedKey(r rune) (keysym uint32, shift bool, err error) {
	switch {
	case r == '\n':
		return keyEnter, false, nil
	case r == '\t':
		return keyTab, false, nil
	case r < 0x20 || r > 0x7e:
		return 0, false, fmt.Errorf("cannot type %q: only printable ASCII, tab and newline are supported", r)
	}
	shift = (r >= 'A' && r <= 'Z') || strings.ContainsRune(shiftedKeys, r)
	return uint32(r), shift, nil
}

// Type types text on the VM's keyboard. A newline presses Enter.
func (c *Console) Type(text string) error {
	// Check the whole text first so that a bad character does not leave half a command typed.
	for _, r := range text {
		if _, _, err := typedKey(r); err != nil {
			return err
		}
	}
	for _, r := range text {
		keysym, shift, _ := typedKey(r)
		var err error
		if shift {
			err = c.press(keysym, keyShift)
		} else {
			err = c.press(keysym)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// parseKey reads a key name such as "enter", "f2", "ctrl-c" or "ctrl-alt-delete".
func parseKey(name string) (keysym uint32, modifiers []uint32, err error) {
	parts := strings.Split(strings.ToLower(name), "-")
	if name == "-" {
		parts = []string{"-"}
	}
	for _, p := range parts[:len(parts)-1] {
		m, ok := modifierKeys[p]
		if !ok {
			return 0, nil, fmt.Errorf("unknown modifier %q in key %q (use ctrl, shift or alt)", p, name)
		}
		modifiers = append(modifiers, m)
	}
	last := parts[len(parts)-1]
	if k, ok := namedKeys[last]; ok {
		return k, modifiers, nil
	}
	if len(last) == 1 && last[0] >= 0x21 && last[0] <= 0x7e {
		return uint32(last[0]), modifiers, nil
	}
	return 0, nil, fmt.Errorf("unknown key %q", name)
}

// PressKey presses a named key, optionally with modifiers: "enter", "ctrl-c", "ctrl-alt-delete".
func (c *Console) PressKey(name string) error {
	keysym, modifiers, err := parseKey(name)
	if err != nil {
		return err
	}
	return c.press(keysym, modifiers...)
}

// Screenshot returns the VM's screen as it is now.
func (c *Console) Screenshot() (*image.RGBA, error) {
	// 32 bits per pixel, little-endian, blue in the low byte.
	pixelFormat := []byte{0, 0, 0, 0, 32, 24, 0, 1, 0, 255, 0, 255, 0, 255, 16, 8, 0, 0, 0, 0}
	if err := c.write(pixelFormat); err != nil {
		return nil, err
	}
	// Raw encoding only.
	if err := c.write([]byte{2, 0, 0, 1, 0, 0, 0, 0}); err != nil {
		return nil, err
	}
	request := []byte{3, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(request[6:], uint16(c.Width))
	binary.BigEndian.PutUint16(request[8:], uint16(c.Height))
	if err := c.write(request); err != nil {
		return nil, err
	}

	img := image.NewRGBA(image.Rect(0, 0, c.Width, c.Height))
	for {
		kind, err := c.read(1)
		if err != nil {
			return nil, err
		}
		switch kind[0] {
		case 0: // FramebufferUpdate
			head, err := c.read(3)
			if err != nil {
				return nil, err
			}
			for n := int(binary.BigEndian.Uint16(head[1:])); n > 0; n-- {
				if err := c.readRect(img); err != nil {
					return nil, err
				}
			}
			return img, nil
		case 1: // SetColourMapEntries
			head, err := c.read(5)
			if err != nil {
				return nil, err
			}
			if _, err := c.read(6 * int(binary.BigEndian.Uint16(head[3:]))); err != nil {
				return nil, err
			}
		case 2: // Bell
		case 3: // ServerCutText
			head, err := c.read(7)
			if err != nil {
				return nil, err
			}
			if _, err := c.read(int(binary.BigEndian.Uint32(head[3:]))); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("console sent unknown VNC message type %d", kind[0])
		}
	}
}

func (c *Console) readRect(img *image.RGBA) error {
	head, err := c.read(12)
	if err != nil {
		return err
	}
	x := int(binary.BigEndian.Uint16(head[0:]))
	y := int(binary.BigEndian.Uint16(head[2:]))
	w := int(binary.BigEndian.Uint16(head[4:]))
	h := int(binary.BigEndian.Uint16(head[6:]))
	if encoding := int32(binary.BigEndian.Uint32(head[8:])); encoding != 0 {
		return fmt.Errorf("console sent VNC encoding %d, only raw was requested", encoding)
	}
	if x+w > c.Width || y+h > c.Height {
		return fmt.Errorf("console sent a %dx%d update at %d,%d outside the %dx%d screen", w, h, x, y, c.Width, c.Height)
	}
	for row := 0; row < h; row++ {
		line, err := c.read(4 * w)
		if err != nil {
			return err
		}
		out := img.Pix[img.PixOffset(x, y+row):]
		for i := 0; i < w; i++ {
			out[4*i], out[4*i+1], out[4*i+2], out[4*i+3] = line[4*i+2], line[4*i+1], line[4*i], 0xff
		}
	}
	return nil
}

// websocketUpgrade asks for a binary websocket on conn and returns a reader of the frames' payload.
func websocketUpgrade(conn net.Conn, host, path, origin string) (io.Reader, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(nonce)

	conn.SetDeadline(time.Now().Add(consoleIOTimeout))
	defer conn.SetDeadline(time.Time{})
	request := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Protocol: binary\r\n" +
		"Origin: " + origin + "\r\n\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		return nil, fmt.Errorf("console websocket request failed: %v", err)
	}

	rd := bufio.NewReader(conn)
	resp, err := http.ReadResponse(rd, &http.Request{Method: "GET"})
	if err != nil {
		return nil, fmt.Errorf("console websocket request failed: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("console refused the websocket: %s (an expired console URL does this)", resp.Status)
	}
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
		return nil, fmt.Errorf("console websocket handshake returned a wrong accept key")
	}
	return &websocketReader{conn: conn, rd: rd}, nil
}

// websocketWrite sends b as one masked binary frame, as a client must.
func websocketWrite(w io.Writer, b []byte) error {
	return websocketFrame(w, 0x2, b)
}

func websocketFrame(w io.Writer, opcode byte, b []byte) error {
	frame := []byte{0x80 | opcode}
	switch {
	case len(b) < 126:
		frame = append(frame, 0x80|byte(len(b)))
	case len(b) < 1<<16:
		frame = append(frame, 0x80|126, byte(len(b)>>8), byte(len(b)))
	default:
		frame = append(frame, 0x80|127)
		frame = binary.BigEndian.AppendUint64(frame, uint64(len(b)))
	}
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	frame = append(frame, mask...)
	for i, c := range b {
		frame = append(frame, c^mask[i%4])
	}
	_, err := w.Write(frame)
	return err
}

// websocketReader presents the payload of incoming data frames as one stream.
type websocketReader struct {
	conn   net.Conn
	rd     *bufio.Reader
	remain uint64 // unread payload bytes of the current data frame
}

func (r *websocketReader) Read(p []byte) (int, error) {
	for r.remain == 0 {
		head := make([]byte, 2)
		if _, err := io.ReadFull(r.rd, head); err != nil {
			return 0, err
		}
		opcode := head[0] & 0x0f
		length := uint64(head[1] & 0x7f)
		switch length {
		case 126:
			ext := make([]byte, 2)
			if _, err := io.ReadFull(r.rd, ext); err != nil {
				return 0, err
			}
			length = uint64(binary.BigEndian.Uint16(ext))
		case 127:
			ext := make([]byte, 8)
			if _, err := io.ReadFull(r.rd, ext); err != nil {
				return 0, err
			}
			length = binary.BigEndian.Uint64(ext)
		}
		if head[1]&0x80 != 0 {
			return 0, fmt.Errorf("server sent a masked websocket frame")
		}
		switch opcode {
		case 0x0, 0x1, 0x2: // continuation, text, binary
			r.remain = length
		case 0x8: // close
			return 0, io.EOF
		case 0x9, 0xa: // ping, pong
			payload := make([]byte, length)
			if _, err := io.ReadFull(r.rd, payload); err != nil {
				return 0, err
			}
			if opcode == 0x9 {
				if err := websocketFrame(r.conn, 0xa, payload); err != nil {
					return 0, err
				}
			}
		default:
			return 0, fmt.Errorf("server sent websocket opcode %#x", opcode)
		}
	}
	if uint64(len(p)) > r.remain {
		p = p[:r.remain]
	}
	n, err := r.rd.Read(p)
	r.remain -= uint64(n)
	return n, err
}
