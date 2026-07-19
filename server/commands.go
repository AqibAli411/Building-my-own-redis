package main

import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"aqib.builds/utils"
)

func doKeys(cmd []string, out *[]byte) {
	if len(cmd) != 1 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}
	*out = outArr(*out, uint32(utils.HmapSize(&Gdata.DB)))
	utils.HmapForEach(&Gdata.DB, func(node *utils.HNode) bool {
		entry := (*utils.Entry)(unsafe.Pointer(node))
		*out = outStr(*out, entry.Key)
		return true
	})
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
		entry := &utils.Entry{Key: cmd[1], Str: cmd[2], Type: utils.T_STR, HeapIndex: -1}
		entry.Node.Hcode = utils.HashKey(cmd[1])
		utils.HmapInsert(&Gdata.DB, &entry.Node)
	} else if keyEntry != nil {
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
		*out = outInt(*out, -1)
		return
	}
	expiresAt := Gdata.heap[entry.HeapIndex].Val
	now := getMonotonicMs()
	remaining := int64(expiresAt) - int64(now)
	if remaining < 0 {
		*out = outInt(*out, 0)
		return
	}
	*out = outInt(*out, remaining)
}

func doPersist(cmd []string, out *[]byte) {
	if len(cmd) != 2 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}
	entry, err := lookUpExisting(cmd[1], utils.T_ANY)
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
	name := cmd[3]
	if entry.ZSet == nil {
		entry.ZSet = &utils.ZSet{}
	}
	utils.ZSetInsert(entry.ZSet, name, score)
	*out = outStr(*out, "OK")
}

func doZscore(cmd []string, out *[]byte) {
	if len(cmd) != 3 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}
	entry, err := lookUpExisting(cmd[1], utils.T_ZSET)
	if entry == nil && err == nil {
		*out = outErr(*out, int32(utils.ERR_NX), "key not found")
		return
	}
	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_VALUE), "invalid input")
		return
	}
	found := utils.ZSetLookup(entry.ZSet, cmd[2])
	if found == nil {
		*out = outErr(*out, int32(utils.ERR_NX), "not found")
		return
	}
	*out = outFloat(*out, found.Score)
}

func doZrem(cmd []string, out *[]byte) {
	if len(cmd) != 3 {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}
	entry, err := lookUpExisting(cmd[1], utils.T_ZSET)
	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_VALUE), "invalid input")
		return
	}
	found := utils.ZSetLookup(entry.ZSet, cmd[2])
	if found == nil {
		*out = outInt(*out, 0)
		return
	}
	utils.ZSetDelete(entry.ZSet, found)
	*out = outInt(*out, 1)
}

func doZrange(cmd []string, out *[]byte) {
	if len(cmd) != 6 || strings.ToLower(cmd[0]) != "zrange" {
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
		return
	}
	score, err := strconv.ParseFloat(cmd[2], 64)
	if err != nil {
		*out = outErr(*out, int32(utils.ERR_INVALID_VALUE), "invalid input")
		return
	}
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

	entry, err := lookUpExisting(cmd[1], utils.T_ZSET)
	if entry == nil && err != nil {
		*out = outErr(*out, int32(utils.ERR_NX), "key not found")
		return
	}
	znode := utils.ZSetSeekGE(entry.ZSet, score, cmd[3])
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
		avlNode = utils.AvlOffset(avlNode, 1)
		cnt += 2
	}
	outArrEnd(out, headPos, uint32(cnt))
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
	case "expire":
		doExpire(cmd, out)
	case "ttl":
		doTTL(cmd, out)
	case "persist":
		doPersist(cmd, out)
	case "zadd":
		doZadd(cmd, out)
	case "zscore":
		doZscore(cmd, out)
	case "zrem":
		doZrem(cmd, out)
	case "zrange":
		doZrange(cmd, out)
	default:
		*out = outErr(*out, int32(utils.ERR_INVALID_COMMAND), "invalid command")
	}
}
