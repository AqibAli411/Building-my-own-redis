package utils

import (
	"fmt"
	"unsafe"
)

// One Member of a Sorted Set
type ZNode struct {
	Tree  AVLNode // for score-based range queries
	HNode HNode   // for name-based exact lookups
	Score float64
	Name  string
}

// ZSet — The Container-> complete set
type ZSet struct {
	// AVL tree indexed by (score, name)
	Root *AVLNode
	// for inside the set (member <-> score)
	HMap HMap
}

// lookup key — never inserted into the tree, only used for hashtable search:
type ZLookupKey struct {
	Node HNode
	Name string
}

func NodeEq(a *HNode, b *HNode) bool {
	za := (*Entry)(unsafe.Pointer(a))
	zb := (*Entry)(unsafe.Pointer(b))

	return za.Key == zb.Key
}

// checks if the names of members are same
func ZNodeEq(a *HNode, b *HNode) bool {
	// a points to ZNode.HNode which is NOT the first field
	// so we need to subtract the offset of HNode within ZNode

	offset := unsafe.Offsetof(ZNode{}.HNode)
	za := (*ZNode)(unsafe.Pointer(uintptr(unsafe.Pointer(a)) - offset))

	// b is always the lookup key — HNode IS its first field so direct cast is fine
	zb := (*ZLookupKey)(unsafe.Pointer(b))

	return za.Name == zb.Name
}

func Zless(a *AVLNode, b *AVLNode) bool {
	za := (*ZNode)(unsafe.Pointer(a))
	zb := (*ZNode)(unsafe.Pointer(b))

	if za.Score != zb.Score {
		return za.Score < zb.Score
	}

	return za.Name < zb.Name
}

// just remember we're inside a container called set
// Point Queries: Lookup, Insert, Delete
func ZSetLookup(zset *ZSet, member string) *ZNode {
	// if hmap is empty we can't look up
	if zset.HMap == (HMap{}) {
		return nil
	}
	key := &ZLookupKey{}
	key.Name = member
	// it would be used in slotting later
	key.Node.Hcode = HashKey(member)

	// in the set we are looking up to all the members comparing our members name
	from := HmapLookup(&zset.HMap, &key.Node, ZNodeEq)
	if from == nil {
		return nil
	}

	offset := unsafe.Offsetof(ZNode{}.HNode)
	za := (*ZNode)(unsafe.Pointer(uintptr(unsafe.Pointer(*from)) - offset))
	return za
}

// for ZADD command
func ZSetInsert(zset *ZSet, member string, score float64) {
	znode := ZSetLookup(zset, member)
	// it is a new member
	if znode == nil {
		// insert into hash table
		node := &ZNode{Name: member, Score: score}
		fmt.Println("inserting new member:", member, "score:", score)
		node.HNode.Hcode = HashKey(member)
		HmapInsert(&zset.HMap, &node.HNode)

		// insert into AVL tree
		node.Tree = AVLNode{}
		zset.Root = insertNode(zset.Root, &node.Tree, Zless)

	} else {
		// we need to update the score
		ZSetUpdate(zset, znode, score)
		return
	}
}

func ZSetUpdate(zset *ZSet, node *ZNode, score float64) {
	// remove and re insert with new score
	zset.Root = deleteNode(zset.Root, &node.Tree, Zless)

	node.Score = score

	node.Tree = AVLNode{}
	zset.Root = insertNode(zset.Root, &node.Tree, Zless)
	// we only update the hashtable
	// but why ?
	// hashtable is only for quick lookups which can be done using name and name doesn't change here
	// where as AVL tree is for score based calculations so it must be updated
}

func ZSetDelete(zset *ZSet, node *ZNode) {
	// remove from hash table
	key := &ZLookupKey{Name: node.Name}
	key.Node = node.HNode

	HmapDetach(&zset.HMap, &key.Node, ZNodeEq)

	zset.Root = deleteNode(zset.Root, &node.Tree, Zless)
}

func zlessScoreName(node *AVLNode, score float64, name string) bool {
	za := (*ZNode)(unsafe.Pointer(node))

	if za.Score != score {
		return za.Score < score
	}

	return za.Name < name
}

// zsetSeekGE finds the first node where (score, name) >= (target_score, target_name).
// GE means "greater than or equal to."
func ZSetSeekGE(zset *ZSet, score float64, name string) *ZNode {
	root := zset.Root
	found := (*AVLNode)(nil)
	for root != nil {
		// true if root's score is less than raw provided
		if zlessScoreName(root, score, name) {
			root = root.Right
		} else {
			found = root
			root = root.Left
		}
	}

	if found == nil {
		return nil
	}

	return (*ZNode)(unsafe.Pointer(found))
}

func AvlOffset(node *AVLNode, offset int64) *AVLNode {
	// how far we have moved from the start so far => pos
	// how far we have to move => offset
	pos := int64(0)

	// until we reach there
	for pos != offset {
		rightCnt := int64(avlCnt(node.Right))
		leftCnt := int64(avlCnt(node.Left))

		// the target is in the right subtree
		if pos < offset && pos+rightCnt >= offset {
			node = node.Right
			// moving right means we passed: all of new node's left subtree + new node itself
			pos += int64(avlCnt(node.Left)) + 1

		} else if pos > offset && pos-leftCnt <= offset {
			// target is somewhere inside our left subtree
			// move into left child
			node = node.Left
			// moving left means we went back past: all of new node's right subtree + new node itself
			pos -= int64(avlCnt(node.Right)) + 1

		} else {
			// target is not in our subtree at all — go up
			parent := node.Parent
			// only true for root -> hence offset is out of range
			if parent == nil {
				return nil
			}
			if parent.Right == node {
				// we came from the right going up moves us left
				pos -= int64(avlCnt(node.Left)) + 1
			} else {
				// we came from the left going up moves us right
				pos += int64(avlCnt(node.Right)) + 1
			}
			node = parent
		}
	}
	return node
}
