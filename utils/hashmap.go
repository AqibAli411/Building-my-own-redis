package utils

import (
	"fmt"
	"hash/fnv"
)

type HNode struct {
	next  *HNode
	Hcode uint64
}

type HTable struct {
	Slots []*HNode
	Mask  uint64
	Size  uint64
}

func HashKey(key string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(key))
	return h.Sum64()
}

func htabInit(tab *HTable, n uint64) error {
	if (n & (n - 1)) != 0 {
		return fmt.Errorf("n must be a power of 2")
	}
	tab.Slots = make([]*HNode, n)
	tab.Mask = n - 1
	tab.Size = 0
	return nil
}

func htabInsert(table *HTable, node *HNode) {
	slot := node.Hcode & table.Mask
	node.next = table.Slots[slot]
	table.Slots[slot] = node
	table.Size++
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
		curr = &(*curr).next
	}
	return nil
}

func htabDetach(htable *HTable, from **HNode) *HNode {
	if *from == nil {
		return nil
	}
	nodeTobeDeleted := *from
	*from = nodeTobeDeleted.next
	htable.Size--
	return nodeTobeDeleted
}
