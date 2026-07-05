package utils

import (
	"fmt"
	"hash/fnv"
	"log"
)

// value can also hold a sorted set or a string
// for any key
// to identify which one is it -> use a type field
// the type field is used to determine how to interpret the value

//	|------> string -> str
//	|
//
// value
//
//	|
//	|------> zset -> zset
type EntryType uint8

const (
	T_STR             EntryType = 1
	T_ZSET            EntryType = 2
	T_ANY             EntryType = 3
	K_MAX_LOAD_FACTOR           = 8   // max keys per slot on average before resize
	K_RESIZING_WORK             = 128 // nodes to migrate per operation

)

type Entry struct {
	Node      HNode // FIRST field — this is critical
	Key       string
	Type      EntryType
	ZSet      *ZSet
	Str       string
	HeapIndex int
}

type HNode struct {
	next  *HNode
	Hcode uint64 // hash value of the key, copied exactly during migration if needed
}

// a single hashtable( an array of buckets where each entry is a head of a linkedlist)
// a slice of HNode pointers (the slots) and a count of items
type HTable struct {
	Slots []*HNode
	Mask  uint64
	Size  uint64
}

// 3. The outer hashtable that manages progressive resizing
// holds two HTabs — the active one and the one being migrated into
type HMap struct {
	Newer      *HTable // the current (larger) table
	Older      *HTable // the old table being drained during resize(active)
	MigratePos uint64  // which slot in Older we're migrating next
}

// Trigger resize when:
// hmap.Newer.Size >= (hmap.Newer.Mask + 1) * K_MAX_LOAD_FACTOR
// i.e., load factor exceeded

func HashKey(key string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(key))
	return h.Sum64()
}

// this function initializes a hashtable with the given size
func htabInit(tab *HTable, n uint64) error {
	// n : size of the table
	// it must be a power of 2
	// for n = 4 => 100 & 011 -> 000 -> 0 for power of 2
	if (n & (n - 1)) != 0 {
		return fmt.Errorf("n must be a power of 2")
	}

	tab.Slots = make([]*HNode, n)
	tab.Mask = n - 1
	tab.Size = 0

	return nil
}

func htabInsert(table *HTable, node *HNode) {
	// index = node.Hcode % table_size
	slot := node.Hcode & table.Mask // which bucket?
	node.next = table.Slots[slot]   // prepend to chain
	table.Slots[slot] = node        // new head
	table.Size++                    // number of elements
}

func htabLookup(table *HTable, keyNode *HNode, eq func(*HNode, *HNode) bool) **HNode {
	if table == nil || len(table.Slots) == 0 {
		return nil
	}

	slot := keyNode.Hcode & table.Mask
	curr := &table.Slots[slot]

	for *curr != nil {
		if eq(*curr, keyNode) {
			return curr
		}
		// we are passing the address of next pointer (not the node, next points to)
		curr = &(*curr).next
	}
	return nil
}

// from is the result of htabLookup: a pointer to the slot or Next field
func htabDetach(htable *HTable, from **HNode) *HNode {
	if *from == nil {
		return nil
	}

	// *from -> a pointer that points to same node as next node
	var nodeTobeDeleted *HNode = *from
	*from = nodeTobeDeleted.next
	htable.Size -= 1
	return nodeTobeDeleted
}

func hmapStartResize(hmap *HMap) error {
	// newer becomes old one, and newer becomes latest one
	hmap.Older = hmap.Newer
	// 1+hmap.Newer.Mask => size of old table * 2 (new table has double size of older one)
	new_size := (1 + hmap.Newer.Mask) * 2
	hmap.Newer = &HTable{}
	err := htabInit(hmap.Newer, new_size)
	if err != nil {
		return fmt.Errorf("failed to init newer table: %w", err)
	}
	// start from oth slot of older table
	hmap.MigratePos = 0
	return nil
}

// The Migration Step
// This is the function that runs on every hashtable operation.
// It moves a fixed number of keys from Older to Newer
func HmapMigrate(hmap *HMap) {
	if hmap.Older == nil {
		return
	}

	workDone := 0
	// stop work when 128 of slots have been migrated or there are no more entries in the older table
	for workDone < K_RESIZING_WORK && hmap.Older.Size > 0 {
		if hmap.MigratePos >= hmap.Older.Mask+1 {
			break // scanned all slots
		}
		from := &hmap.Older.Slots[hmap.MigratePos]

		if *from == nil {
			// delete from a slot until there are no entries left on that slot
			hmap.MigratePos += 1
			continue
		}

		// delete node from older table and insert it into the newer table
		node := htabDetach(hmap.Older, from)
		htabInsert(hmap.Newer, node)
		// move to next entity in the linkedlist at the position we're currently migrating from migratePos
		workDone++
	}

	if hmap.Older != nil && hmap.Older.Size == 0 {
		hmap.Older = nil
	}
}

func HmapLookup(hmap *HMap, key *HNode, eq func(*HNode, *HNode) bool) **HNode {
	// do some migration
	HmapMigrate(hmap)

	result := htabLookup(hmap.Newer, key, eq)
	if result == nil {
		result = htabLookup(hmap.Older, key, eq)
	}

	return result
}

// hmap -> nil
func HmapInsert(hmap *HMap, node *HNode) {
	// there is no newer table
	if hmap.Newer == nil {
		hmap.Newer = &HTable{}
		err := htabInit(hmap.Newer, 4)
		if err != nil {
			log.Printf("failed to init newer table: %v", err)
			return
		}
	}
	htabInsert(hmap.Newer, node)

	if hmap.Older == nil {
		// no resize currently happening — check if we need to start one
		// if number of entities at each slot exceed max load factor (8)
		if (hmap.Newer.Size) >= (hmap.Newer.Mask+1)*K_MAX_LOAD_FACTOR {
			err := hmapStartResize(hmap)
			if err != nil {
				log.Printf("failed to resize: %v", err)
				return
			}
		}
	}
	// at last because we need to take in account if user is inserting at the first time
	HmapMigrate(hmap)
}

func HmapDetach(hmap *HMap, node *HNode, eq func(*HNode, *HNode) bool) *HNode {
	HmapMigrate(hmap)

	// check in newer and delete
	from := htabLookup(hmap.Newer, node, eq)
	if from != nil {
		return htabDetach(hmap.Newer, from)
	}
	// check in older and delete
	from = htabLookup(hmap.Older, node, eq)
	if from != nil {
		return htabDetach(hmap.Older, from)
	}

	// key not found
	return nil
}

func HmapForEach(hmap *HMap, callback func(*HNode) bool) {
	if hmap == nil {
		return
	}

	for _, slot := range hmap.Newer.Slots {
		for head := slot; head != nil; head = head.next {
			if !callback(head) {
				return
			}
		}
	}

	if hmap.Older == nil {
		return
	}
	for _, slot := range hmap.Older.Slots {
		for head := slot; head != nil; head = head.next {
			if !callback(head) {
				return
			}
		}
	}
}

func HmapSize(hmap *HMap) uint64 {
	var size uint64
	if hmap.Newer != nil {
		size += hmap.Newer.Size
	}
	if hmap.Older != nil {
		size += hmap.Older.Size
	}
	return size
}
