package main

import (
	"fmt"
	"unsafe"

	"aqib.builds/utils"
)

func zsetClear(zset *utils.ZSet) {
	if zset != nil {
		zset.HMap.Newer = nil
		zset.HMap.Older = nil
		zset.Root = nil
	}
}

func entryDelSync(entry *utils.Entry) {
	if entry.Type == utils.T_ZSET {
		zsetClear(entry.ZSet)
	}
}

func entryDel(entry *utils.Entry) {
	entrySetTTL(entry, -1)

	var setSize uint64 = 0
	if entry.Type == utils.T_ZSET {
		setSize = utils.HmapSize(&entry.ZSet.HMap)
	}

	const K_LARGE_CONTAINER_SIZE = 1000
	if setSize > K_LARGE_CONTAINER_SIZE {
		Gdata.pool.submit(func() { entryDelSync(entry) })
	} else {
		entryDelSync(entry)
	}
}

func lookUpOrCreate(key string) *utils.Entry {
	entry := &utils.Entry{Key: key, Type: utils.T_ZSET, HeapIndex: -1}
	entry.Node.Hcode = utils.HashKey(key)

	from := utils.HmapLookup(&Gdata.DB, &entry.Node, utils.NodeEq)
	if from != nil {
		return (*utils.Entry)(unsafe.Pointer(*from))
	}

	utils.HmapInsert(&Gdata.DB, &entry.Node)
	return entry
}

func lookUpExisting(key string, expectedType utils.EntryType) (*utils.Entry, error) {
	entry := &utils.Entry{Key: key, Type: expectedType, HeapIndex: -1}
	entry.Node.Hcode = utils.HashKey(key)

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
