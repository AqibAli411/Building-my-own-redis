package main

import (
	"fmt"
)

type ServerState struct {
	kv map[string]string
}

var Gdata ServerState

func init() {
	Gdata.kv = make(map[string]string)
}

func DoRequest(cmd []string, out *[]byte) {
	if len(cmd) == 0 {
		return
	}
	switch cmd[0] {
	case "get":
		if len(cmd) < 2 {
			*out = []byte("err: invalid args")
			return
		}
		val, ok := Gdata.kv[cmd[1]]
		if !ok {
			*out = []byte("nil")
		} else {
			*out = []byte(val)
		}
	case "set":
		if len(cmd) < 3 {
			*out = []byte("err: invalid args")
			return
		}
		Gdata.kv[cmd[1]] = cmd[2]
		*out = []byte("OK")
	case "del":
		if len(cmd) < 2 {
			*out = []byte("err: invalid args")
			return
		}
		delete(Gdata.kv, cmd[1])
		*out = []byte("OK")
	}
}
