package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"unsafe"

	"aqib.builds/utils"
	"golang.org/x/sys/unix"
)

func fd_set_nb(fd int) {
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil {
		panic(err)
	}
	flags |= unix.O_NONBLOCK
	_, err = unix.FcntlInt(uintptr(fd), unix.F_SETFL, flags)
	if err != nil {
		panic(err)
	}
}

func connectionIO(conn *Conn) {
	if conn.state == STATE_REQ {
		state_req(conn)
	} else if conn.state == STATE_RES {
		state_res(conn)
	}
}

func state_req(conn *Conn) {
	for try_fill_buffer(conn) {
	}
}

func try_fill_buffer(conn *Conn) bool {
	var n int
	var err error
	for {
		n, err = unix.Read(conn.fd, conn.rbuf[conn.rbuf_size:])
		if n >= 0 || err != unix.EINTR {
			break
		}
	}

	if n < 0 && err == unix.EAGAIN {
		return false
	}
	if n < 0 {
		fmt.Println("read() error")
		conn.state = STATE_END
		return false
	}
	if n == 0 {
		if conn.rbuf_size > 0 {
			fmt.Println("unexpected EOF")
		} else {
			fmt.Println("EOF")
		}
		conn.state = STATE_END
		return false
	}

	conn.rbuf_size += n
	for try_one_request(conn) {
	}
	return true
}

func try_one_request(conn *Conn) bool {
	if conn.rbuf_size < 4 {
		return false
	}

	var length uint32 = binary.LittleEndian.Uint32(conn.rbuf[:4])
	if length > utils.K_MAX_MESSAGE {
		log.Fatal("message length too large")
		conn.state = STATE_END
		return false
	}
	if length+4 > uint32(conn.rbuf_size) {
		return false
	}

	dataBytes := conn.rbuf[4 : 4+length]
	parsed, err := ParseReq(dataBytes)
	if err != nil {
		log.Fatal("error parsing the request: ", err)
		return false
	}

	var buf []byte
	buf, headPos := responseBegin(buf)
	DoRequest(parsed, &buf)
	err = responseEnd(buf, headPos)
	if err != nil {
		log.Fatal("error sending the response: ", err)
		return false
	}

	copy(conn.wbuf[:], buf)
	conn.wbuf_size = len(buf)

	remaining := conn.rbuf_size - int(length) - 4
	if remaining != 0 {
		copy(conn.rbuf[:remaining], conn.rbuf[4+int(length):4+int(length)+remaining])
	}
	conn.rbuf_size = remaining
	conn.state = STATE_RES
	state_res(conn)

	return (conn.state == STATE_REQ)
}

func try_write_buffer(conn *Conn) bool {
	var n int
	var err error
	for {
		n, err = unix.Write(conn.fd, conn.wbuf[conn.wbuf_sent:conn.wbuf_size])
		if n >= 0 || err != unix.EINTR {
			break
		}
	}

	if n < 0 && err == unix.EAGAIN {
		return false
	}
	if n < 0 {
		fmt.Println("write() error")
		conn.state = STATE_END
		return false
	}
	if n == 0 {
		if conn.wbuf_size > 0 {
			fmt.Println("unexpected EOF")
		} else {
			fmt.Println("EOF")
		}
		conn.state = STATE_END
		return false
	}

	conn.wbuf_sent += n
	if conn.wbuf_sent == conn.wbuf_size {
		conn.state = STATE_REQ
		conn.wbuf_size = 0
		conn.wbuf_sent = 0
		return false
	}
	return true
}

func state_res(conn *Conn) {
	for try_write_buffer(conn) {
	}
}

func accept_conn(fd int, fd2Conn map[int32]*Conn) error {
	connFd, _, err := unix.Accept(fd)
	if err != nil {
		fmt.Println("accept() error:", err)
		return err
	}

	fd_set_nb(connFd)

	conn := &Conn{
		fd:        connFd,
		state:     STATE_REQ,
		rbuf_size: 0,
		wbuf_size: 0,
		wbuf_sent: 0,
		rbuf:      [4 + utils.K_MAX_MESSAGE]byte{},
		wbuf:      [4 + utils.K_MAX_MESSAGE]byte{},
	}

	conn.LastActiveMs = getMonotonicMs()
	utils.DlistInsertBefore(&Gdata.IdleList, &conn.IdleNode)
	fd2Conn[int32(connFd)] = conn
	return nil
}

func DlistToConn(node *utils.Dlist) *Conn {
	offset := unsafe.Offsetof(Conn{}.IdleNode)
	conn := (*Conn)(unsafe.Pointer(uintptr(unsafe.Pointer(node)) - offset))
	return conn
}

func destroyConn(conn *Conn, fd2Conn map[int32]*Conn) {
	unix.Close(conn.fd)
	utils.DlistDetach(&conn.IdleNode)
	delete(fd2Conn, int32(conn.fd))
	conn = nil
}
