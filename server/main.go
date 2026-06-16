package main

import (
	"encoding/binary"
	"fmt"
	"golang.org/x/sys/unix"
)

const (
	STATE_REQ = 0
	STATE_RES = 1
	STATE_END = 2
)

type Conn struct {
	fd        int
	state     int
	rbuf_size int
	rbuf      [4 + 4096]byte
	wbuf_sent int
	wbuf_size int
	wbuf      [4 + 4096]byte
}

func fb_set_nb(fb int) {
	flags, _ := unix.FcntlInt(uintptr(fb), unix.F_GETFL, 0)
	flags |= unix.O_NONBLOCK
	unix.FcntlInt(uintptr(fb), unix.F_SETFL, flags)
}

func main() {
	fd, _ := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	defer unix.Close(fd)
	unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
	addr := &unix.SockaddrInet4{Port: 1234, Addr: [4]byte{0, 0, 0, 0}}
	unix.Bind(fd, addr)
	unix.Listen(fd, unix.SOMAXCONN)
	
	fd2Conn := make(map[int32]*Conn)
	fb_set_nb(fd)
	
	var poll_args []unix.PollFd
	for {
		poll_args = []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		for _, conn := range fd2Conn {
			var events int16 = unix.POLLERR
			if conn.state == STATE_REQ {
				events |= unix.POLLIN
			} else {
				events |= unix.POLLOUT
			}
			poll_args = append(poll_args, unix.PollFd{Fd: int32(conn.fd), Events: events})
		}
		
		unix.Poll(poll_args, -1)
		for i := 1; i < len(poll_args); i++ {
			if poll_args[i].Revents == 0 {
				continue
			}
			conn := fd2Conn[poll_args[i].Fd]
			if conn == nil {
				continue
			}
			if conn.state == STATE_REQ {
				n, err := unix.Read(conn.fd, conn.rbuf[conn.rbuf_size:])
				if n > 0 {
					conn.rbuf_size += n
					for try_one_request(conn) {}
				} else if n < 0 && err == unix.EAGAIN {
					continue
				} else {
					conn.state = STATE_END
				}
			} else {
				n, _ := unix.Write(conn.fd, conn.wbuf[conn.wbuf_sent:conn.wbuf_size])
				if n > 0 {
					conn.wbuf_sent += n
					if conn.wbuf_sent == conn.wbuf_size {
						conn.state = STATE_REQ
						conn.wbuf_size = 0
						conn.wbuf_sent = 0
					}
				} else {
					conn.state = STATE_END
				}
			}
			if conn.state == STATE_END {
				unix.Close(conn.fd)
				delete(fd2Conn, int32(conn.fd))
			}
		}
		if poll_args[0].Revents != 0 {
			connFd, _, err := unix.Accept(fd)
			if err == nil {
				fb_set_nb(connFd)
				fd2Conn[int32(connFd)] = &Conn{fd: connFd, state: STATE_REQ}
			}
		}
	}
}

func try_one_request(conn *Conn) bool {
	if conn.rbuf_size < 4 {
		return false
	}
	length := binary.LittleEndian.Uint32(conn.rbuf[:4])
	if length > 4096 {
		conn.state = STATE_END
		return false
	}
	if length+4 > uint32(conn.rbuf_size) {
		return false
	}
	
	parsed, err := ParseReq(conn.rbuf[4 : 4+length])
	if err == nil && len(parsed) > 0 {
		var out []byte
		DoRequest(parsed, &out)
		// prepend 4-byte response length
		resp := make([]byte, 4+len(out))
		binary.LittleEndian.PutUint32(resp[:4], uint32(len(out)))
		copy(resp[4:], out)
		copy(conn.wbuf[:], resp)
		conn.wbuf_size = len(resp)
		conn.state = STATE_RES
	}
	
	remaining := conn.rbuf_size - int(length) - 4
	if remaining > 0 {
		copy(conn.rbuf[:remaining], conn.rbuf[4+int(length):])
	}
	conn.rbuf_size = remaining
	return false
}
