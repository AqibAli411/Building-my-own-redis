package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"unsafe"

	"aqib.builds/utils"
	"golang.org/x/sys/unix"
)

const (
	STATE_REQ = 0 // 0
	STATE_RES = 1 // 1
	STATE_END = 2 // 2
)

type Conn struct {
	fd    int
	state int
	// buffer for reading
	rbuf_size int
	rbuf      [4 + utils.K_MAX_MESSAGE]byte
	// buffer for writing
	wbuf_sent    int
	wbuf_size    int
	wbuf         [4 + utils.K_MAX_MESSAGE]byte
	LastActiveMs uint64      // when did this connection last do IO?
	IdleNode     utils.Dlist // intrusive list node — embedded
}

func fd_set_nb(fd int) {
	// FcntlInt(fb, command, arg)
	// F_GETFL -> syscall for geting the flags
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil {
		panic(err)
	}

	// set the file descriptor to non-blocking mode
	// by attaching the flags
	flags |= unix.O_NONBLOCK
	_, err = unix.FcntlInt(uintptr(fd), unix.F_SETFL, flags)
	if err != nil {
		panic(err)
	}
}

func connectionIO(conn *Conn) {
	//check if it is for response or request
	if conn.state == STATE_REQ {
		// handle request
		state_req(conn)
	} else if conn.state == STATE_RES {
		state_res(conn)
	}
	// for reading
	// read and chekc
}

func state_req(conn *Conn) {
	for try_fill_buffer(conn) {
	}
}

// Read data from the socket into the read buffer in non-blocking manner
func try_fill_buffer(conn *Conn) bool {
	var n int
	var err error
	// find the first successfull outcome then break
	// it breaks on the first successful outcome or any other error
	for {
		// first time rbuf_size would be zero
		// it would increase as data arives from tcp conneciton
		n, err = unix.Read(conn.fd, conn.rbuf[conn.rbuf_size:])
		if n >= 0 || err != unix.EINTR {
			break
		}
	}

	// error is EAGAIN, which means the socket is not ready to read
	if n < 0 && err == unix.EAGAIN {
		return false
	}

	// another error
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
	// see the user can send multiple data chunks at the same time after some latency
	// so we need to handle them one by one until the buffer is empty
	for try_one_request(conn) {
	}

	return true
}

func try_one_request(conn *Conn) bool {
	// checking if the buffer has enough data to read the header
	if conn.rbuf_size < 4 {
		return false
	}

	var length uint32 = binary.LittleEndian.Uint32(conn.rbuf[:4])

	if length > utils.K_MAX_MESSAGE {
		log.Fatal("message length too large")
		// delete this connection
		conn.state = STATE_END
		return false
	}

	// check if full data is recevied
	if length+4 > uint32(conn.rbuf_size) {
		return false
	}

	fmt.Printf("client says: len=%d msg=%s\n", length, string(conn.rbuf[4:4+length]))

	// send the response
	dataBytes := conn.rbuf[4 : 4+length]
	parsed, err := ParseReq(dataBytes)
	if err != nil {
		log.Fatal("error. parsing the request: ", err)
		return false
	}
	var buf []byte
	buf, headPos := responseBegin(buf)

	DoRequest(parsed, &buf)

	err = responseEnd(buf, headPos)
	if err != nil {
		log.Fatal("error. sending the response: ", err)
		return false
	}

	copy(conn.wbuf[:], buf)
	conn.wbuf_size = len(buf)

	// conn.wbuf_size is exact size of the response sent
	// get val1get val2
	remaining := conn.rbuf_size - int(length) - 4
	if remaining != 0 {
		// moving remaining data to the beginning of the buffer
		copy(conn.rbuf[:remaining], conn.rbuf[4+int(length):4+int(length)+remaining])
	}
	conn.rbuf_size = remaining
	conn.state = STATE_RES

	state_res(conn)

	return (conn.state == STATE_REQ)
}

// It's the non-blocking equivalent of write()
func try_write_buffer(conn *Conn) bool {
	var n int
	var err error
	// find the first successfull outcome then break
	// it breaks on the first successful outcome or any other error
	for {
		n, err = unix.Write(conn.fd, conn.wbuf[conn.wbuf_sent:conn.wbuf_size])
		fmt.Printf("write %d bytes, err %v\n", n, err)
		// any error other than EINTR means we can't proceed
		// EINTR means the write was interrupted by a signal, we should retry
		// EAGAIN means the write would block, we should wait for the socket to be writable
		// n >= 0 means the write was successful, we can break
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
	// write completely
	if conn.wbuf_sent == conn.wbuf_size {
		// Back to STATE_REQ (wait for next request)
		conn.state = STATE_REQ
		// Total bytes to send (set in try_one_request)
		conn.wbuf_size = 0
		// Bytes sent so far (reset after write completes)
		conn.wbuf_sent = 0
		return false
	}

	// continue to write the buffer if it's not empty
	return true
}

func state_res(conn *Conn) {
	for try_write_buffer(conn) {
	}
}

// create a conn object and append it to the map
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

	// Since you always move active connections to the back, the front of the list always holds the connection
	// that was active the longest ago.
	conn.LastActiveMs = getMonotonicMs()
	// adding the connection at the end of the linkedlist
	utils.DlistInsertBefore(&Gdata.IdleList, &conn.IdleNode)

	fd2Conn[int32(connFd)] = conn
	return nil
}

// now := time.Now()  // Returns wall clock time
// Can jump forward, backward, or stand still
// Affected by NTP, timezone changes, daylight saving
// this is Monotonic time that only moves forward — it's just a counter that started when the machine booted.

// Wall clock
// NTP (Network time protocol)
// is a protocol that computer uses to get accurate time and synchronize its clock with time sources
// over the internet

func DlistToConn(node *utils.Dlist) *Conn {
	offset := unsafe.Offsetof(Conn{}.IdleNode)
	conn := (*Conn)(unsafe.Pointer(uintptr(unsafe.Pointer(node)) - offset))
	return conn
}

func getMonotonicMs() uint64 {
	var ts unix.Timespec
	err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts)
	if err != nil {
		return 0
	}
	// converted s -> ms and ns -> ms
	return uint64(ts.Sec*1000) + uint64(ts.Nsec/1_000_000)
}

// nextTimerMs returns the time of the oldest connection that will expire
func nextTimerMs() int32 {

	// get current time
	now := getMonotonicMs()

	nextMs := uint64(math.MaxUint64)

	if !utils.DlistEmpty(&Gdata.IdleList) {
		conn := DlistToConn(Gdata.IdleList.Next)
		expiresAt := conn.LastActiveMs + utils.K_IDLE_TIMEOUT_MS
		nextMs = min(expiresAt, nextMs)
	}

	if len(Gdata.heap) > 0 {
		root := Gdata.heap[0]
		nextMs = min(root.Val, nextMs)
	}

	if nextMs == math.MaxUint64 {
		return -1 // no timers
	}

	// check expire date
	if nextMs <= now {
		fmt.Printf("expired: nextMs=%d now=%d\n", nextMs, now)
		return 0 // already expired
	}

	// how much time is left
	return int32(nextMs - now)
}

func destroyConn(conn *Conn, fd2Conn map[int32]*Conn) {
	unix.Close(conn.fd)
	// remove it from the list
	utils.DlistDetach(&conn.IdleNode)
	// remove it from the fd2Conn because it is dead
	delete(fd2Conn, int32(conn.fd))
	conn = nil
}

// this function tends to remove the expired connections from the idle list
func processTimers(fd2Conn map[int32]*Conn) {
	now := getMonotonicMs()

	for !utils.DlistEmpty(&Gdata.IdleList) {
		conn := DlistToConn(Gdata.IdleList.Next)
		expiresAt := conn.LastActiveMs + utils.K_IDLE_TIMEOUT_MS

		if expiresAt >= now {
			break
		}

		log.Println("closing idle connection")

		destroyConn(conn, fd2Conn)
	}

	workDone := 0

	for len(Gdata.heap) > 0 && Gdata.heap[0].Val <= now && workDone < 2000 {
		entry := entryFromHeapRef(Gdata.heap[0].Ref)
		// Remove from the heap FIRST and mark HeapIndex = -1.
		// This must happen before entryDel, because entryDel calls
		// entrySetTTL(-1) → DeleteHeap. If we let DeleteHeap run twice
		// it swaps a different entry into the root slot and corrupts
		// that entry's HeapIndex, causing an out-of-bounds panic.
		utils.DeleteHeap(&Gdata.heap, 0)
		entry.HeapIndex = -1
		utils.HmapDetach(&Gdata.DB, &entry.Node, utils.NodeEq) // remove from hashtable
		entryDel(entry)                                        // free/clean the entry (HeapIndex is -1, so entrySetTTL is a no-op)
		workDone++
	}

}

func connUpdateTimer(conn *Conn) {
	conn.LastActiveMs = getMonotonicMs()
	utils.DlistDetach(&conn.IdleNode)
	utils.DlistInsertBefore(&Gdata.IdleList, &conn.IdleNode)
}

func main() {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		panic(err)
	}

	defer unix.Close(fd)

	val := 1
	err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, val)
	if err != nil {
		panic(err)
	}

	addr := &unix.SockaddrInet4{
		Port: 1234,
		Addr: [4]byte{0, 0, 0, 0},
	}

	err = unix.Bind(fd, addr)
	if err != nil {
		panic(err)
	}

	err = unix.Listen(fd, unix.SOMAXCONN)
	if err != nil {
		panic(err)
	}

	fmt.Println("Server running on :1234")

	//If in STATE_REQ → monitor for POLLIN (data to read)
	// If in STATE_RES → monitor for POLLOUT (can write)

	// var fd2Conn []*Conn
	fd2Conn := make(map[int32]*Conn)
	fd_set_nb(fd)

	var poll_args []unix.PollFd

	Gdata.IdleList = *utils.DlistInit(&Gdata.IdleList)

	// event loop
	for {
		// clean the poll args
		poll_args = make([]unix.PollFd, 0)

		// append PoolFd to poll_args
		poll_args = append(poll_args, unix.PollFd{
			Fd:      int32(fd),
			Events:  unix.POLLIN,
			Revents: 0,
		})

		// setup all the connections so we can pass it to poll
		for _, conn := range fd2Conn {
			if conn == nil {
				continue
			}

			var events int16 = unix.POLLERR
			if conn.state == STATE_REQ {
				events |= unix.POLLIN // Need to read
			} else {
				events |= unix.POLLOUT // Need to write
			}

			poll_args = append(poll_args, unix.PollFd{
				Fd:     int32(conn.fd),
				Events: events,
			})
		}

		timeout := nextTimerMs()
		// n is the number of file descriptors that have events ready (i.e., how many FDs have revents set)
		// Revents = "Returned Events" - it's what poll() gives back to tell you what actually happened on each file descriptor.
		// only blocking code in whole event loop
		n, err := unix.Poll(poll_args, int(timeout))
		if err != nil {
			panic(err)
		}

		processTimers(fd2Conn)

		if n == 0 {
			continue
		}

		for i := 1; i < len(poll_args); i++ {
			if poll_args[i].Revents == 0 {
				continue
			}

			conn := fd2Conn[(poll_args[i].Fd)]
			if conn == nil {
				continue
			}
			// update the timer for the connection
			connUpdateTimer(conn)

			// handle the connection -> it can be both for writing and reading
			connectionIO(conn)

			if conn.state == STATE_END {
				destroyConn(conn, fd2Conn)
			}
		}

		if poll_args[0].Revents != 0 {
			// accept the connection
			err := accept_conn(int(poll_args[0].Fd), fd2Conn)
			if err != nil {
				fmt.Println("accept_conn() error:", err)
			}
		}

	}
}
