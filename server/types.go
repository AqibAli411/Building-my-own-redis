package main

import (
	"aqib.builds/utils"
)

const (
	STATE_REQ = 0
	STATE_RES = 1
	STATE_END = 2
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

type ServerState struct {
	DB       utils.HMap
	IdleList utils.Dlist // dummy head of idle connections list
	heap     []utils.HeapItem
	pool     *ThreadPool
}

var Gdata ServerState
