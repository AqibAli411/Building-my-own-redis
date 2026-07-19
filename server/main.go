package main

import (
	"fmt"
	"runtime"

	"aqib.builds/utils"
	"golang.org/x/sys/unix"
)

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

	fd2Conn := make(map[int32]*Conn)
	fd_set_nb(fd)

	Gdata.IdleList = *utils.DlistInit(&Gdata.IdleList)
	Gdata.pool = NewThreadPool(runtime.NumCPU())

	var poll_args []unix.PollFd

	for {
		poll_args = make([]unix.PollFd, 0)

		poll_args = append(poll_args, unix.PollFd{
			Fd:     int32(fd),
			Events: unix.POLLIN,
		})

		for _, conn := range fd2Conn {
			if conn == nil {
				continue
			}
			var events int16 = unix.POLLERR
			if conn.state == STATE_REQ {
				events |= unix.POLLIN
			} else {
				events |= unix.POLLOUT
			}
			poll_args = append(poll_args, unix.PollFd{
				Fd:     int32(conn.fd),
				Events: events,
			})
		}

		timeout := nextTimerMs()
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
			connUpdateTimer(conn)
			connectionIO(conn)
			if conn.state == STATE_END {
				destroyConn(conn, fd2Conn)
			}
		}

		if poll_args[0].Revents != 0 {
			err := accept_conn(int(poll_args[0].Fd), fd2Conn)
			if err != nil {
				fmt.Println("accept_conn() error:", err)
			}
		}
	}
}
