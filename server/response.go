package main

import (
	"encoding/binary"
	"math"

	"aqib.builds/utils"
)

func outNil(out []byte) []byte {
	out = append(out, byte(utils.TAG_NIL))
	return out
}

func outStr(out []byte, s string) []byte {
	out = append(out, byte(utils.TAG_STR))
	length := uint32(len(s))
	out = append(out, []byte{0, 0, 0, 0}...)
	binary.LittleEndian.PutUint32(out[len(out)-4:], length)
	out = append(out, []byte(s)...)
	return out
}

func outInt(out []byte, i int64) []byte {
	out = append(out, byte(utils.TAG_INT))
	out = append(out, []byte{0, 0, 0, 0, 0, 0, 0, 0}...)
	binary.LittleEndian.PutUint64(out[len(out)-8:], uint64(i))
	return out
}

func outFloat(out []byte, val float64) []byte {
	out = append(out, byte(utils.TAG_DBL))
	out = append(out, []byte{0, 0, 0, 0, 0, 0, 0, 0}...)
	bits := math.Float64bits(val)
	binary.LittleEndian.PutUint64(out[len(out)-8:], bits)
	return out
}

func outArr(out []byte, n uint32) []byte {
	out = append(out, byte(utils.TAG_ARR))
	out = append(out, []byte{0, 0, 0, 0}...)
	binary.LittleEndian.PutUint32(out[len(out)-4:], n)
	return out
}

func outArrBegin(out *[]byte) int {
	*out = append(*out, byte(utils.TAG_ARR))
	headPos := len(*out)
	*out = append(*out, []byte{0, 0, 0, 0}...)
	return headPos
}

func outArrEnd(out *[]byte, headPos int, length uint32) {
	binary.LittleEndian.PutUint32((*out)[headPos:headPos+4], uint32(length))
}

func outErr(out []byte, errCode int32, msg string) []byte {
	out = append(out, byte(utils.TAG_ERR))
	out = append(out, []byte{0, 0, 0, 0}...)
	binary.LittleEndian.PutUint32(out[len(out)-4:], uint32(errCode))
	for range len(msg) {
		out = append(out, 0)
	}
	binary.LittleEndian.PutUint32(out[len(out)-len(msg):], uint32(len(msg)))
	out = append(out, []byte(msg)...)
	return out
}

func appendUint32(buf []byte, val uint32) []byte {
	var temp [4]byte
	binary.LittleEndian.PutUint32(temp[:], val)
	buf = append(buf, temp[:]...)
	return buf
}

func responseBegin(buf []byte) ([]byte, int) {
	headPos := len(buf)
	buf = appendUint32(buf, 0)
	return buf, headPos
}

func responseEnd(buf []byte, headPos int) error {
	msgLen := uint32(len(buf) - headPos - 4)
	if msgLen > utils.K_MAX_MESSAGE {
		buf = buf[:headPos+4]
		buf = outErr(buf, int32(utils.ERR_TOO_LONG), "response too large")
		msgLen = uint32(len(buf) - headPos - 4)
	}
	binary.LittleEndian.PutUint32(buf[headPos:headPos+4], msgLen)
	return nil
}
