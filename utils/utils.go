package utils

import (
	"fmt"

	"golang.org/x/sys/unix"
)

const K_MAX_MESSAGE uint32 = 4096
const K_IDLE_TIMEOUT_MS uint64 = 15_000

type TagType uint8

const (
	TAG_NIL TagType = 0 // nothing / null
	TAG_ERR TagType = 1 // error: a code (int32) + a message (string)
	TAG_STR TagType = 2 // a string
	TAG_INT TagType = 3 // a 64-bit integer
	TAG_DBL TagType = 4 // a 64-bit float (you won't use this yet)
	TAG_ARR TagType = 5 // an array of any of the above
)

type ErrorType uint32

const (
	ERR_NX              ErrorType = 0
	ERR_TOO_LONG        ErrorType = 1
	ERR_INVALID_COMMAND ErrorType = 2
	ERR_INVALID_VALUE   ErrorType = 3
	ERR_INVALID_TYPE    ErrorType = 4
)

// used at the client side
func Read_Full(fd int, rbuff []byte, n_bytes uint32) (int, error) {
	remaining := int(n_bytes) // how many bytes we WANT
	readBytes := 0
	for remaining > 0 {
		n, err := unix.Read(fd, rbuff[readBytes:])
		if err != nil {
			return readBytes, err
		}
		if n == 0 {
			return readBytes, fmt.Errorf("EOF")
		}
		remaining -= n
		readBytes += n
	}
	return readBytes, nil
}

func Write_All(fd int, writeBytes []byte, n_bytes uint32) (int, error) {
	remaining := int(n_bytes) // how many bytes we WANT
	written := 0
	for remaining > 0 {
		n, err := unix.Write(fd, writeBytes[written:])
		if err != nil {
			return written, err
		}
		remaining -= n
		written += n
	}
	return written, nil
}
