/*
 * Copyright (c) 2015, Yawning Angel <yawning at schwanenlied dot me>
 * All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions are met:
 *
 *  * Redistributions of source code must retain the above copyright notice,
 *    this list of conditions and the following disclaimer.
 *
 *  * Redistributions in binary form must reproduce the above copyright notice,
 *    this list of conditions and the following disclaimer in the documentation
 *    and/or other materials provided with the distribution.
 *
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
 * AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
 * IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
 * ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
 * LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
 * CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
 * SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
 * INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
 * CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
 * ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
 * POSSIBILITY OF SUCH DAMAGE.
 */

package socks5

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"
)

func tcpAddrsEqual(a, b *net.TCPAddr) bool {
	return a.IP.Equal(b.IP) && a.Port == b.Port
}

// testReadWriter is a bytes.Buffer backed io.ReadWriter used for testing.  The
// Read and Write routines are to be used by the component being tested.  Data
// can be written to and read back via the writeHex and readHex routines.
type testReadWriter struct {
	readBuf  bytes.Buffer
	writeBuf bytes.Buffer
}

func (c *testReadWriter) Read(buf []byte) (n int, err error) {
	return c.readBuf.Read(buf)
}

func (c *testReadWriter) Write(buf []byte) (n int, err error) {
	return c.writeBuf.Write(buf)
}

func (c *testReadWriter) writeHex(str string) {
	var buf []byte
	var err error

	if buf, err = hex.DecodeString(str); err != nil {
		panic("writeHex: malformed hex: " + err.Error())
	}
	if _, err = c.readBuf.Write(buf); err != nil {
		panic("writeHex: buffered write failed: " + err.Error())
	}
}

func (c *testReadWriter) readHex() string {
	return hex.EncodeToString(c.writeBuf.Bytes())
}

func (c *testReadWriter) toBufio() *bufio.ReadWriter {
	return bufio.NewReadWriter(bufio.NewReader(c), bufio.NewWriter(c))
}

func (c *testReadWriter) toRequest() *Request {
	req := new(Request)
	req.rw = c.toBufio()
	return req
}

func (c *testReadWriter) reset(req *Request) {
	c.readBuf.Reset()
	c.writeBuf.Reset()
	req.rw = c.toBufio()
}

// TestAuthInvalidVersion tests auth negotiation with an invalid version.
func TestAuthInvalidVersion(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()

	// VER = 03, NMETHODS = 01, METHODS = [00]
	c.writeHex("030100")
	if _, err := req.negotiateAuth(); err == nil {
		t.Error("negotiateAuth(InvalidVersion) succeded")
	}
}

// TestAuthInvalidNMethods tests auth negotiaton with no methods.
func TestAuthInvalidNMethods(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()
	var err error
	var method byte

	// VER = 05, NMETHODS = 00
	c.writeHex("0500")
	if method, err = req.negotiateAuth(); err != nil {
		t.Error("negotiateAuth(No Methods) failed:", err)
	}
	if method != authNoAcceptableMethods {
		t.Error("negotiateAuth(No Methods) picked unexpected method:", method)
	}
	if msg := c.readHex(); msg != "05ff" {
		t.Error("negotiateAuth(No Methods) invalid response:", msg)
	}
}

// TestAuthNoneRequired tests auth negotiaton with NO AUTHENTICATION REQUIRED.
func TestAuthNoneRequired(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()
	var err error
	var method byte

	// VER = 05, NMETHODS = 01, METHODS = [00]
	c.writeHex("050100")
	if method, err = req.negotiateAuth(); err != nil {
		t.Error("negotiateAuth(None) failed:", err)
	}
	if method != authNoneRequired {
		t.Error("negotiateAuth(None) unexpected method:", method)
	}
	if msg := c.readHex(); msg != "0500" {
		t.Error("negotiateAuth(None) invalid response:", msg)
	}
}

// TestAuthUsernamePassword tests auth negotiation with USERNAME/PASSWORD.
func TestAuthUsernamePassword(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()
	var err error
	var method byte

	// VER = 05, NMETHODS = 01, METHODS = [02]
	c.writeHex("050102")
	if method, err = req.negotiateAuth(); err != nil {
		t.Error("negotiateAuth(UsernamePassword) failed:", err)
	}
	if method != authUsernamePassword {
		t.Error("negotiateAuth(UsernamePassword) unexpected method:", method)
	}
	if msg := c.readHex(); msg != "0502" {
		t.Error("negotiateAuth(UsernamePassword) invalid response:", msg)
	}
}

// TestAuthBoth tests auth negotiation containing both NO AUTHENTICATION
// REQUIRED and USERNAME/PASSWORD.
func TestAuthBoth(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()
	var err error
	var method byte

	// VER = 05, NMETHODS = 02, METHODS = [00, 02]
	c.writeHex("05020002")
	if method, err = req.negotiateAuth(); err != nil {
		t.Error("negotiateAuth(Both) failed:", err)
	}
	if method != authUsernamePassword {
		t.Error("negotiateAuth(Both) unexpected method:", method)
	}
	if msg := c.readHex(); msg != "0502" {
		t.Error("negotiateAuth(Both) invalid response:", msg)
	}
}

// TestAuthUnsupported tests auth negotiation with a unsupported method.
func TestAuthUnsupported(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()
	var err error
	var method byte

	// VER = 05, NMETHODS = 01, METHODS = [01] (GSSAPI)
	c.writeHex("050101")
	if method, err = req.negotiateAuth(); err != nil {
		t.Error("negotiateAuth(Unknown) failed:", err)
	}
	if method != authNoAcceptableMethods {
		t.Error("negotiateAuth(Unknown) picked unexpected method:", method)
	}
	if msg := c.readHex(); msg != "05ff" {
		t.Error("negotiateAuth(Unknown) invalid response:", msg)
	}
}

// TestAuthUnsupported2 tests auth negotiation with supported and unsupported
// methods.
func TestAuthUnsupported2(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()
	var err error
	var method byte

	// VER = 05, NMETHODS = 03, METHODS = [00,01,02]
	c.writeHex("0503000102")
	if method, err = req.negotiateAuth(); err != nil {
		t.Error("negotiateAuth(Unknown2) failed:", err)
	}
	if method != authUsernamePassword {
		t.Error("negotiateAuth(Unknown2) picked unexpected method:", method)
	}
	if msg := c.readHex(); msg != "0502" {
		t.Error("negotiateAuth(Unknown2) invalid response:", msg)
	}
}

// TestRFC1929InvalidVersion tests RFC1929 auth with an invalid version.
func TestRFC1929InvalidVersion(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()

	// VER = 03, ULEN = 5, UNAME = "ABCDE", PLEN = 5, PASSWD = "abcde"
	c.writeHex("03054142434445056162636465")
	if err := req.authenticate(authUsernamePassword); err == nil {
		t.Error("authenticate(InvalidVersion) succeded")
	}
	if msg := c.readHex(); msg != "0101" {
		t.Error("authenticate(InvalidVersion) invalid response:", msg)
	}
}

// TestRFC1929InvalidUlen tests RFC1929 auth with an invalid ULEN.
func TestRFC1929InvalidUlen(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()

	// VER = 01, ULEN = 0, UNAME = "", PLEN = 5, PASSWD = "abcde"
	c.writeHex("0100056162636465")
	if err := req.authenticate(authUsernamePassword); err == nil {
		t.Error("authenticate(InvalidUlen) succeded")
	}
	if msg := c.readHex(); msg != "0101" {
		t.Error("authenticate(InvalidUlen) invalid response:", msg)
	}
}

// TestRFC1929InvalidPlen tests RFC1929 auth with an invalid PLEN.
func TestRFC1929InvalidPlen(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()

	// VER = 01, ULEN = 5, UNAME = "ABCDE", PLEN = 0, PASSWD = ""
	c.writeHex("0105414243444500")
	if err := req.authenticate(authUsernamePassword); err == nil {
		t.Error("authenticate(InvalidPlen) succeded")
	}
	if msg := c.readHex(); msg != "0101" {
		t.Error("authenticate(InvalidPlen) invalid response:", msg)
	}
}

// TestRequestInvalidHdr tests SOCKS5 requests with invalid VER/CMD/RSV/ATYPE
func TestRequestInvalidHdr(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()

	// VER = 03, CMD = 01, RSV = 00, ATYPE = 01, DST.ADDR = 127.0.0.1, DST.PORT = 9050
	c.writeHex("030100017f000001235a")
	if err := req.readCommand(); err == nil {
		t.Error("readCommand(InvalidVer) succeded")
	}
	if msg := c.readHex(); msg != "05010001000000000000" {
		t.Error("readCommand(InvalidVer) invalid response:", msg)
	}
	c.reset(req)

	// VER = 05, CMD = 05, RSV = 00, ATYPE = 01, DST.ADDR = 127.0.0.1, DST.PORT = 9050
	c.writeHex("050500017f000001235a")
	if err := req.readCommand(); err == nil {
		t.Error("readCommand(InvalidCmd) succeded")
	}
	if msg := c.readHex(); msg != "05070001000000000000" {
		t.Error("readCommand(InvalidCmd) invalid response:", msg)
	}
	c.reset(req)

	// VER = 05, CMD = 01, RSV = 30, ATYPE = 01, DST.ADDR = 127.0.0.1, DST.PORT = 9050
	c.writeHex("050130017f000001235a")
	if err := req.readCommand(); err == nil {
		t.Error("readCommand(InvalidRsv) succeded")
	}
	if msg := c.readHex(); msg != "05010001000000000000" {
		t.Error("readCommand(InvalidRsv) invalid response:", msg)
	}
	c.reset(req)

	// VER = 05, CMD = 01, RSV = 01, ATYPE = 05, DST.ADDR = 127.0.0.1, DST.PORT = 9050
	c.writeHex("050100057f000001235a")
	if err := req.readCommand(); err == nil {
		t.Error("readCommand(InvalidAtype) succeded")
	}
	if msg := c.readHex(); msg != "05080001000000000000" {
		t.Error("readCommand(InvalidAtype) invalid response:", msg)
	}
	c.reset(req)
}

// TestRequestIPv4 tests IPv4 SOCKS5 requests.
func TestRequestIPv4(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()

	// VER = 05, CMD = 01, RSV = 00, ATYPE = 01, DST.ADDR = 127.0.0.1, DST.PORT = 9050
	c.writeHex("050100017f000001235a")
	if err := req.readCommand(); err != nil {
		t.Error("readCommand(IPv4) failed:", err)
	}
	addr, err := net.ResolveTCPAddr("tcp", req.Target)
	if err != nil {
		t.Error("net.ResolveTCPAddr failed:", err)
	}
	if !tcpAddrsEqual(addr, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9050}) {
		t.Error("Unexpected target:", addr)
	}
}

// TestRequestIPv6 tests IPv4 SOCKS5 requests.
func TestRequestIPv6(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()

	// VER = 05, CMD = 01, RSV = 00, ATYPE = 04, DST.ADDR = 0102:0304:0506:0708:090a:0b0c:0d0e:0f10, DST.PORT = 9050
	c.writeHex("050100040102030405060708090a0b0c0d0e0f10235a")
	if err := req.readCommand(); err != nil {
		t.Error("readCommand(IPv6) failed:", err)
	}
	addr, err := net.ResolveTCPAddr("tcp", req.Target)
	if err != nil {
		t.Error("net.ResolveTCPAddr failed:", err)
	}
	if !tcpAddrsEqual(addr, &net.TCPAddr{IP: net.ParseIP("0102:0304:0506:0708:090a:0b0c:0d0e:0f10"), Port: 9050}) {
		t.Error("Unexpected target:", addr)
	}
}

// TestRequestFQDN tests FQDN (DOMAINNAME) SOCKS5 requests.
func TestRequestFQDN(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()

	// VER = 05, CMD = 01, RSV = 00, ATYPE = 04, DST.ADDR = example.com, DST.PORT = 9050
	c.writeHex("050100030b6578616d706c652e636f6d235a")
	if err := req.readCommand(); err != nil {
		t.Error("readCommand(FQDN) failed:", err)
	}
	if req.Target != "example.com:9050" {
		t.Error("Unexpected target:", req.Target)
	}
}

// TestResponseNil tests nil address SOCKS5 responses.
func TestResponseNil(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()

	if err := req.Reply(ReplySucceeded); err != nil {
		t.Error("Reply(ReplySucceeded) failed:", err)
	}
	if msg := c.readHex(); msg != "05000001000000000000" {
		t.Error("Reply(ReplySucceeded) invalid response:", msg)
	}
}

func TestRequestUDPAssociate(t *testing.T) {
	c := new(testReadWriter)
	req := c.toRequest()

	// bind to a local UDP socket
	tgt := ListenUDP()
	ap, _ := netip.ParseAddrPort(tgt.LocalAddr().String())
	var cmd [4 + 4 + 2]byte
	var err error
	cmd[0] = version
	cmd[1] = UDPAssociateCmd
	cmd[3] = atypIPv4
	ip4 := ap.Addr().As4()
	copy(cmd[4:8], ip4[:])
	binary.BigEndian.PutUint16(cmd[8:10], ap.Port())
	if _, err := c.readBuf.Write(cmd[:]); err != nil {
		t.Error("readBuf.Write failed:", err)
	}

	if err := req.readCommand(); err != nil {
		t.Error("readCommand(UDPAssociate) failed:", err)
	}

	// socks5 server process starts a UDP listener
	req.Conn = ListenUDP()
	if err := req.Reply(ReplySucceeded); err != nil {
		t.Error("req.Reply(ReplySucceeded) failed:", err)
	}

	// verify that there is a listening UDP socket
	buf := c.writeBuf.Bytes()

	if buf[0] != version || buf[1] != 0 || buf[2] != rsv {
		t.Error("Invalid response header")
	}

	// verify that the address
	var ip net.IP
	var port uint16

	switch buf[3] {
	case atypIPv4:
		if len(buf) != 4+4+2 {
			t.Error("Reply invalid length")
		}
		ip = net.IP(buf[4:8])
		port = binary.BigEndian.Uint16(buf[8:10])
	case atypIPv6:
		if len(buf) != 4+16+2 {
			t.Error("Reply invalid length")
		}
		port = binary.BigEndian.Uint16(buf[20:22])
		ip = net.IP(buf[4:20])
	default:
		t.Error("Invalid address type")
	}

	// Test that the UDP socket returned is correct by
	// writing/reading data
	payload := make([]byte, 420)
	if _, err := io.ReadFull(rand.Reader, payload); err != nil {
		t.Error(err)
	}

	addr := fmt.Sprintf("%s:%d", ip.String(), port)
	t.Log("Dialing", addr)

	// start waiting for bytes on the listening UDP socket
	errch := make(chan error)
	go func() {
		recvbuf := make([]byte, len(payload))
		if err := req.Conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			panic(err)
		}
		if _, err := req.Conn.Read(recvbuf); err != nil {
			panic(err)
		}
		if !bytes.Equal(recvbuf, payload) {
			errch <- fmt.Errorf("Received UDP payload differed! %x %x", recvbuf, payload)
		}
		close(errch)
	}()

	uconn, err := net.Dial("udp", addr)
	if err != nil {
		t.Error("Dialing failure:", err)
	}

	_, err = uconn.Write(payload)
	if err != nil {
		t.Error("writing to UDP socket failed")
	}

	// wait for ead
	err, ok := <-errch
	if ok {
		t.Error(err)
	}
}

var _ io.ReadWriter = (*testReadWriter)(nil)
