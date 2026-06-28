package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unsafe"

	"aqib.builds/utils"
)

type ServerState struct {
	DB       utils.HMap
	IdleList utils.Dlist // dummy head of idle connections list
	heap     []utils.HeapItem
}

var Gdata ServerState

func entryFromHeapRef(ref *int) *utils.Entry {
	offset := unsafe.Offsetof(utils.Entry{}.HeapIndex)
	ptr := uintptr(unsafe.Pointer(ref)) - offset
	return (*utils.Entry)(unsafe.Pointer(ptr))
}

func entrySetTTL(entry *utils.Entry, ttlMs int64) {
	if ttlMs < 0 {
		// if there is no timer
		if entry.HeapIndex == -1 {
			return
		}
		utils.DeleteHeap(&Gdata.heap, entry.HeapIndex)
		fmt.Println("entry : ", entry.Key)
		entry.HeapIndex = -1
	} else {

		expiresAt := getMonotonicMs() + uint64(ttlMs)

		item := utils.HeapItem{Val: expiresAt, Ref: &entry.HeapIndex}

		utils.HeapUpsert(&Gdata.heap, entry.HeapIndex, item)
	}
}

func entryDel(entry *utils.Entry) {
	if entry.ZSet != nil {
		// remove the zset
		entry.ZSet = nil
	}
	fmt.Println("before deleting  : ", entry.HeapIndex)

	entrySetTTL(entry, -1)
}

// check if entry on the given key in the gdata hashmap exists if not create one and return
// if it exists then return that
func lookUpOrCreate(key string) *utils.Entry {
	// create entry
	entry := &utils.Entry{Key: key, Type: utils.T_ZSET, HeapIndex: -1}
	entry.Node.Hcode = utils.HashKey(key)

	from := utils.HmapLookup(&Gdata.DB, &entry.Node, utils.NodeEq)

	if from != nil {
		return (*utils.Entry)(unsafe.Pointer(*from))
	}
	// it doesn't exist
	// so we create one and insert
	utils.HmapInsert(&Gdata.DB, &entry.Node)
	return entry
}

func lookUpExisting(key string, expectedType utils.EntryType) (*utils.Entry, error) {
	entry := &utils.Entry{Key: key, Type: expectedType, HeapIndex: -1}
	entry.Node.Hcode = utils.HashKey(key)

	// utils.NodeEq -> finds element by comparing keys
	node := utils.HmapLookup(&Gdata.DB, &entry.Node, utils.NodeEq)

	if node == nil {
		return nil, nil
	}

	entry = (*utils.Entry)(unsafe.Pointer(*node))

	if expectedType != utils.T_ANY && expectedType != entry.Type {
		return nil, fmt.Errorf("expected type %v, got %v", expectedType, entry.Type)
	}

	return entry, nil
}

// zadd key score name
func doZadd(cmd []string, out *[]byte) {
	if len(cmd) != 4 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}

	entry := lookUpOrCreate(cmd[1])
	score, err := strconv.ParseFloat(cmd[2], 64)
	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_VALUE), "invalid value")
		return
	}

	// extract the name of member from the command
	name := cmd[3]
	if entry.ZSet == nil {
		entry.ZSet = &utils.ZSet{}
	}

	utils.ZSetInsert(entry.ZSet, name, score)
	*out = outStr(*out, "OK")
}

// zscore key name
// cmd[0] cmd[1] cmd[2]
func doZscore(cmd []string, out *[]byte) {
	if len(cmd) != 3 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}

	key := cmd[1]
	entry, err := lookUpExisting(key, utils.T_ZSET)
	if entry == nil && err == nil {
		*out = outErr(*out, int32(utils.ERR_NX), "key not found")
		return
	}

	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_VALUE), "invalid input")
		return
	}

	name := cmd[2]
	found := utils.ZSetLookup(entry.ZSet, name)
	if found == nil {
		*out = outErr(*out, int32(utils.ERR_NX), "not found")
		return
	}

	*out = outFloat(*out, found.Score)
}

// zrem key name
// cmd[0] cmd[1] cmd[2]
func doZrem(cmd []string, out *[]byte) {
	if len(cmd) != 3 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}
	key := cmd[1]
	entry, err := lookUpExisting(key, utils.T_ZSET)
	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_VALUE), "invalid input")
		return
	}
	// looking into the container
	name := cmd[2]
	found := utils.ZSetLookup(entry.ZSet, name)
	if found == nil {
		// the member doesn't exist
		*out = outInt(*out, 0)
		return
	}

	utils.ZSetDelete(entry.ZSet, found)
	*out = outInt(*out, 1)
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

// {"zrange", "board", "100.0", "", "0", "10"},
// zquery key score name offset limit
// cmd[0] cmd[1] cmd[2] cmd[3] cmd[4] cmd[5]
func doZrange(cmd []string, out *[]byte) {
	if len(cmd) != 6 || strings.ToLower(cmd[0]) != "zrange" {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}

	key := cmd[1]
	score, err := strconv.ParseFloat(cmd[2], 64)
	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_VALUE), "invalid input")
		return
	}

	name := cmd[3]
	offset, err := strconv.ParseInt(cmd[4], 10, 64)
	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_VALUE), "invalid input")
		return
	}

	limit, err := strconv.ParseInt(cmd[5], 10, 64)
	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_VALUE), "invalid input")
		return
	}

	// find the container
	entry, err := lookUpExisting(key, utils.T_ZSET)
	if entry == nil && err != nil {
		*out = outErr(*out, int32(utils.ERR_NX), "key not found")
		return
	}

	// find the next greater member inside the container
	znode := utils.ZSetSeekGE(entry.ZSet, score, name)
	if znode == nil {
		*out = outErr(*out, int32(utils.ERR_NX), "member not found")
		return
	}

	avlNode := utils.AvlOffset(&znode.Tree, offset)

	headPos := outArrBegin(out)
	var cnt int64 = 0

	for avlNode != nil && cnt < 2*limit {
		znode = (*utils.ZNode)(unsafe.Pointer(avlNode))
		*out = outStr(*out, znode.Name)
		*out = outFloat(*out, znode.Score)
		fmt.Printf("DEBUG: znode: %v\n", znode.Name)

		avlNode = utils.AvlOffset(avlNode, 1)
		cnt += 2
	}
	outArrEnd(out, headPos, uint32(cnt))
}

func doGet(cmd []string, out *[]byte) {
	if len(cmd) != 2 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}

	keyEntry, err := lookUpExisting(cmd[1], utils.T_STR)
	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_TYPE),
			fmt.Sprintf("same name exists for a sorted set. Error. %v", err))
		return
	}

	if keyEntry == nil && err == nil {
		*out = outErr(*out, int32(utils.ERR_NX), "key not found")
		return
	}
	*out = outStr(*out, keyEntry.Str)
}

func doKeys(cmd []string, out *[]byte) {
	if len(cmd) != 1 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}
	*out = outArr(*out, uint32(utils.HmapSize(&Gdata.DB)))
	utils.HmapForEach(&Gdata.DB, func(node *utils.HNode) bool {
		// cast back to entry from node and collect the key
		entry := (*utils.Entry)(unsafe.Pointer(node))
		*out = outStr(*out, entry.Key)
		return true
	})
}

func doSet(cmd []string, out *[]byte) {
	if len(cmd) != 3 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}

	keyEntry, err := lookUpExisting(cmd[1], utils.T_STR)
	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_TYPE), "same name exists for a sorted set. Error")
		return
	}

	if keyEntry == nil && err == nil {
		// HeapIndex MUST be -1 (sentinel for "not in heap").
		// Go's zero value is 0, which HeapUpsert would misread as
		// "already in heap at slot 0", silently corrupting the heap.
		entry := &utils.Entry{Key: cmd[1], Str: cmd[2], Type: utils.T_STR, HeapIndex: -1}
		entry.Node.Hcode = utils.HashKey(cmd[1])
		utils.HmapInsert(&Gdata.DB, &entry.Node)
	} else if keyEntry != nil {
		// update only value if the entry already exists
		keyEntry.Str = cmd[2]
	}
	*out = outNil(*out)
}

func doDel(cmd []string, out *[]byte) {
	if len(cmd) != 2 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}
	keyEntry := &utils.Entry{}
	keyEntry.Key = cmd[1]
	keyEntry.Node.Hcode = utils.HashKey(cmd[1])

	from := utils.HmapDetach(&Gdata.DB, &keyEntry.Node, utils.NodeEq)
	if from == nil {
		*out = outInt(*out, 0)
	} else {
		*out = outInt(*out, 1)
	}
}

// Sets or updates a TTL on an existing key.
// EXPIRE key ttlMs
func doExpire(cmd []string, out *[]byte) {
	if len(cmd) != 3 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}

	ttlMs, err := strconv.ParseInt(cmd[2], 10, 64)
	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}

	entry, err := lookUpExisting(cmd[1], utils.T_ANY)

	if err == nil && entry == nil {
		*out = outInt(*out, 0)
		return
	}

	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}

	entrySetTTL(entry, ttlMs)
	*out = outInt(*out, 1)
}

// Returns milliseconds remaining until expiry. Returns -1 if the key has no TTL, -2 if the key doesn't exist.
// -1 for expired key and -2 for non-existent key
func doTTL(cmd []string, out *[]byte) {
	if len(cmd) != 2 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}

	entry, err := lookUpExisting(cmd[1], utils.T_ANY)
	if err == nil && entry == nil {
		*out = outInt(*out, -2)
		return
	}

	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}

	if entry.HeapIndex == -1 {
		fmt.Println("hey there")
		*out = outInt(*out, -1)
		return
	}

	fmt.Print("entry.HeapIndex: ", entry.HeapIndex)
	expiresAt := Gdata.heap[entry.HeapIndex].Val
	now := getMonotonicMs()
	remaining := int64(expiresAt) - int64(now)
	if remaining < 0 {
		*out = outInt(*out, 0)
		return
	}
	*out = outInt(*out, remaining)
}

// PERSIST key
func doPersist(cmd []string, out *[]byte) {
	if len(cmd) != 2 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}

	key := cmd[1]
	entry, err := lookUpExisting(key, utils.T_ANY)
	if err != nil && entry == nil {
		*out = outInt(*out, 0)
		return
	}

	if entry.HeapIndex == -1 {
		*out = outInt(*out, 0)
		return
	}

	entrySetTTL(entry, -1)
	*out = outInt(*out, 1)
}

func DoRequest(cmd []string, out *[]byte) {
	if len(cmd) == 0 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}

	switch strings.ToLower(cmd[0]) {
	case "keys":
		doKeys(cmd, out)
	case "get":
		doGet(cmd, out)
	case "set":
		doSet(cmd, out)
	case "del":
		doDel(cmd, out)
	case "zscore":
		doZscore(cmd, out)
	case "zadd":
		doZadd(cmd, out)
	case "zrem":
		doZrem(cmd, out)
	case "zrange":
		doZrange(cmd, out)
	case "ttl":
		doTTL(cmd, out)
	case "expire":
		doExpire(cmd, out)
	case "persist":
		doPersist(cmd, out)
	default:
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
	}
}

// ┌─────┐
// │ tag │       just 1 byte — no length, no value needed
// └─────┘
func outNil(out []byte) []byte {
	out = append(out, byte(utils.TAG_NIL))
	return out
}

// ┌─────┬─────┬──────┐
// │ tag │ len │ data │   1 byte tag + 4 byte length + actual bytes
// └─────┴─────┴──────┘
func outStr(out []byte, s string) []byte {
	out = append(out, byte(utils.TAG_STR))
	length := uint32(len(s))
	out = append(out, []byte{0, 0, 0, 0}...)
	binary.LittleEndian.PutUint32(out[len(out)-4:], length)
	out = append(out, []byte(s)...)
	return out
}

// ┌─────┬──────────┐
// │ tag │  int64   │   1 byte tag + 8 bytes value
// └─────┴──────────┘
func outInt(out []byte, i int64) []byte {
	out = append(out, byte(utils.TAG_INT))
	out = append(out, []byte{0, 0, 0, 0, 0, 0, 0, 0}...)
	binary.LittleEndian.PutUint64(out[len(out)-8:], uint64(i))
	fmt.Printf("out: %v\n", out)
	return out
}

// ┌─────┬─────┬──────────────────────┐
// │ tag │ len │ element element ...  │   tag + count + each element (recursive)
// └─────┴─────┴──────────────────────┘
func outArr(out []byte, n uint32) []byte {
	out = append(out, byte(utils.TAG_ARR))
	out = append(out, []byte{0, 0, 0, 0}...)
	binary.LittleEndian.PutUint32(out[len(out)-4:], n)
	return out
}

func outErr(out []byte, errCode int32, msg string) []byte {
	out = append(out, byte(utils.TAG_ERR)) // 1byte
	out = append(out, []byte{0, 0, 0, 0}...)
	binary.LittleEndian.PutUint32(out[len(out)-4:], uint32(errCode))
	for range len(msg) {
		out = append(out, 0)
	}
	binary.LittleEndian.PutUint32(out[len(out)-len(msg):], uint32(len(msg)))
	out = append(out, []byte(msg)...)
	return out
}

func outFloat(out []byte, val float64) []byte {
	out = append(out, byte(utils.TAG_DBL))
	out = append(out, []byte{0, 0, 0, 0, 0, 0, 0, 0}...)
	bits := math.Float64bits(val)
	binary.LittleEndian.PutUint64(out[len(out)-8:], bits)
	return out
}

func appendUint32(buf []byte, val uint32) []byte {
	var temp [4]byte
	binary.LittleEndian.PutUint32(temp[:], val)
	buf = append(buf, temp[:]...)
	return buf
}

func responseBegin(buf []byte) ([]byte, int) {
	headPos := len(buf) // initally zero
	buf = appendUint32(buf, 0)
	return buf, headPos
}

func responseEnd(buf []byte, headPos int) error {
	msgLen := uint32(len(buf) - headPos - 4)
	fmt.Printf("DEBUG: msgLen: %v, buf_size: %v\n", msgLen, len(buf))
	if msgLen > utils.K_MAX_MESSAGE {
		// response grew too large
		// truncate back to just the placeholder
		buf = buf[:headPos+4]
		// ↑ slice tricks: cut everything after the placeholder
		// then write an error response instead
		buf = outErr(buf, int32(utils.ERR_TOO_LONG), "response too large")
		msgLen = uint32(len(buf) - headPos - 4)
	}

	binary.LittleEndian.PutUint32(buf[headPos:headPos+4], msgLen)
	return nil
}
