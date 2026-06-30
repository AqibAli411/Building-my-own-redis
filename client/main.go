package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"

	"aqib.builds/utils"

	"golang.org/x/sys/unix"
)

func send_req(connFd int, writeBytes []byte) error {
	msg_length := uint32(len(writeBytes))
	if msg_length > utils.K_MAX_MESSAGE {
		panic("message too long")
	}

	// format the message before sending (so it follows our protocol)
	wbuff := make([]byte, 4+msg_length)
	binary.LittleEndian.PutUint32(wbuff[:4], uint32(msg_length))
	copy(wbuff[4:], writeBytes)
	_, err := utils.Write_All(connFd, wbuff, 4+msg_length)
	if err != nil {
		return fmt.Errorf("error writing the request: %w", err)
	}

	return nil
}

func read_res(connFd int) error {
	// read 4-byte length header
	header := make([]byte, 4)
	_, err := utils.Read_Full(connFd, header, 4)
	if err != nil {
		return fmt.Errorf("error reading header: %w", err)
	}

	res_len := binary.LittleEndian.Uint32(header[:4])

	if res_len > utils.K_MAX_MESSAGE {
		return fmt.Errorf("message too long: %d", res_len)
	}

	// read the body
	body := make([]byte, res_len)
	_, err = utils.Read_Full(connFd, body, res_len)
	if err != nil {
		return fmt.Errorf("error reading body: %w", err)
	}

	// parse and print the TLV response
	_, err = printTLV(body, 0)
	return err
}

func printTLV(body []byte, offset int) (int, error) {
	// make sure there is at least 1 byte to read (the tag)
	if offset >= len(body) {
		return offset, fmt.Errorf("unexpected end of data")
	}

	tag := utils.TagType(body[offset])
	offset += 1

	switch tag {

	case utils.TAG_NIL:
		fmt.Println("(nil)")
		return offset, nil

	case utils.TAG_INT:
		// need 8 bytes for int64
		if offset+8 > len(body) {
			return offset, fmt.Errorf("not enough bytes for int64")
		}
		val := int64(binary.LittleEndian.Uint64(body[offset : offset+8]))
		offset += 8
		fmt.Println(val)
		return offset, nil

	case utils.TAG_STR:
		// 4 bytes for length
		if offset+4 > len(body) {
			return offset, fmt.Errorf("not enough bytes for string length")
		}
		strLen := binary.LittleEndian.Uint32(body[offset : offset+4])
		offset += 4
		// then strLen bytes for the actual string
		if offset+int(strLen) > len(body) {
			return offset, fmt.Errorf("not enough bytes for string data")
		}
		str := string(body[offset : offset+int(strLen)])
		offset += int(strLen)
		fmt.Println(str)
		return offset, nil

	case utils.TAG_ERR:
		// 4 bytes for error code
		if offset+4 > len(body) {
			return offset, fmt.Errorf("not enough bytes for error code")
		}
		code := binary.LittleEndian.Uint32(body[offset : offset+4])
		offset += 4
		// 4 bytes for message length
		if offset+4 > len(body) {
			return offset, fmt.Errorf("not enough bytes for error message length")
		}
		msgLen := binary.LittleEndian.Uint32(body[offset : offset+4])
		offset += 4
		// then msgLen bytes for the message
		if offset+int(msgLen) > len(body) {
			return offset, fmt.Errorf("not enough bytes for error message")
		}
		msg := string(body[offset:])
		offset += int(msgLen)
		fmt.Printf("(error) code=%d msg=%s\n", code, msg)
		return offset, nil

	case utils.TAG_ARR:
		// 4 bytes for element count
		if offset+4 > len(body) {
			return offset, fmt.Errorf("not enough bytes for array length")
		}
		count := binary.LittleEndian.Uint32(body[offset : offset+4])
		offset += 4
		fmt.Printf("(array of %d)\n", count)
		// parse each element by calling printTLV recursively
		for i := 0; i < int(count); i++ {
			var err error
			offset, err = printTLV(body, offset)
			if err != nil {
				return offset, fmt.Errorf("error parsing array element %d: %w", i, err)
			}
		}
		return offset, nil
	case utils.TAG_DBL:
		if offset+8 <= len(body) {
			bits := binary.LittleEndian.Uint64(body[offset : offset+8])
			val := math.Float64frombits(bits)
			offset += 8
			fmt.Printf("(float) %f\n", val)
		}
		return offset, nil
	default:
		return offset, fmt.Errorf("unknown tag: %d", tag)
	}
}

func sendReq(fd int, args []string) error {
	nstr := len(args)
	total := 4
	for _, arg := range args {
		total += (4 + len(arg))
	}

	wBuff := make([]byte, total)
	offset := 0
	binary.LittleEndian.PutUint32(wBuff[offset:offset+4], uint32(nstr))
	offset += 4

	for _, arg := range args {
		if offset+4 > total {
			return fmt.Errorf("buffer overflow %d vs %d\n", offset+len(arg), total)
		}
		binary.LittleEndian.PutUint32(wBuff[offset:offset+4], uint32(len(arg)))
		offset += 4

		if offset+len(arg) > total {
			return fmt.Errorf("buffer overflow %d vs %d\n", offset+len(arg), total)
		}

		copy(wBuff[offset:offset+len(arg)], []byte(arg))
		offset += len(arg)
	}

	return send_req(fd, wBuff)
}

func main() {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		panic(err)
	}

	addr := &unix.SockaddrInet4{
		Port: 1234,
		Addr: [4]byte{127, 0, 0, 1},
	}

	err = unix.Connect(fd, addr)
	if err != nil {
		log.Fatal("error while connecting to server")
		panic(err)
	}
	defer unix.Close(fd)

	query_lists := [][]string{
		// {"set", "foo1", "bar"},
		// {"expire", "foo1", "100"},
		{"ttl", "foo1"},
		// {"set", "foo2", "bar"},
		// {"expire", "foo2", "100"},
		{"ttl", "foo2"},
	}
	// send responses
	for _, query_list := range query_lists {
		err = sendReq(fd, query_list)
		if err != nil {
			unix.Close(fd)
			log.Fatal("error while sending request. Error : ", err)
		}
	}
	// server reponses them therefore read
	for range query_lists {
		err = read_res(fd)
		if err != nil {
			unix.Close(fd)
			log.Fatal("error : ", err)
		}
	}
}
