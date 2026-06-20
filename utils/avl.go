package utils

import (
	"math"
)

type AVLNode struct {
	Left   *AVLNode
	Right  *AVLNode
	Parent *AVLNode
	Height uint32
	Cnt    uint32
}

func avlHeight(Node *AVLNode) int {
	// height of leaf nodes is zero
	if Node == nil {
		return -1
	}
	if Node.Left == nil && Node.Right == nil {
		return 0
	}
	return int(Node.Height)
}

func avlCnt(root *AVLNode) uint32 {
	if root == nil {
		return 0
	}

	return root.Cnt
}

func avlUpdateHeight(Node *AVLNode) {
	Node.Height = uint32(1 + max(avlHeight(Node.Left), avlHeight(Node.Right)))
	Node.Cnt = 1 + avlCnt(Node.Left) + avlCnt(Node.Right)
}

func avlBalanceFactor(Node *AVLNode) int {
	if Node == nil {
		return 0
	}
	return int(avlHeight(Node.Left)) - int(avlHeight(Node.Right))
}

func avlIsBalanced(Node *AVLNode) bool {
	return math.Abs(float64(avlBalanceFactor(Node))) <= 1.00
}

func RRrotation(Root *AVLNode) *AVLNode {
	if Root == nil {
		return nil
	}

	// Save references
	t1 := Root.Right
	t2 := t1.Left

	// Perform rotation
	t1.Left = Root
	Root.Right = t2

	// Update parent pointers
	if t2 != nil {
		t2.Parent = Root
	}
	Root.Parent = t1
	t1.Parent = nil // t1 is the new root

	// Update heights (order matters: children first)
	avlUpdateHeight(Root)
	avlUpdateHeight(t1)

	return t1
}

func LLrotation(Root *AVLNode) *AVLNode {
	if Root == nil {
		return nil
	}

	// Save references
	t1 := Root.Left
	t2 := t1.Right

	// Perform rotation
	t1.Right = Root
	Root.Left = t2

	// Update parent pointers
	if t2 != nil {
		t2.Parent = Root
	}
	Root.Parent = t1
	t1.Parent = nil // t1 is the new root

	// Update heights (order matters: children first)
	avlUpdateHeight(Root)
	avlUpdateHeight(t1)

	return t1
}

func avlRebalance(Root *AVLNode) *AVLNode {
	if avlBalanceFactor(Root) > 1 && avlBalanceFactor(Root.Left) >= 0 {
		return LLrotation(Root)
	} else if avlBalanceFactor(Root) < -1 && avlBalanceFactor(Root.Right) <= 0 {
		return RRrotation(Root)
	} else if avlBalanceFactor(Root) > 1 && avlBalanceFactor(Root.Left) < 0 {
		Root.Left = RRrotation(Root.Left)
		return LLrotation(Root)
	} else if avlBalanceFactor(Root) < -1 && avlBalanceFactor(Root.Right) > 0 {
		Root.Right = LLrotation(Root.Right)
		return RRrotation(Root)
	}

	return Root
}

func insertNode(Root *AVLNode, Node *AVLNode, less func(*AVLNode, *AVLNode) bool) *AVLNode {
	if Root == nil {
		Node.Left = nil
		Node.Right = nil
		Node.Parent = nil
		Node.Height = 0
		Node.Cnt = 1
		return Node
	}

	if less(Node, Root) {
		child := insertNode(Root.Left, Node, less)
		Root.Left = child
		child.Parent = Root
	} else {
		child := insertNode(Root.Right, Node, less)
		Root.Right = child
		child.Parent = Root
	}

	avlUpdateHeight(Root)
	if avlIsBalanced(Root) {
		return Root
	}

	return avlRebalance(Root)
}

func minNode(node *AVLNode) *AVLNode {
	if node == nil {
		return nil
	}

	for node.Left != nil {
		node = node.Left
	}
	return node
}

func deleteNode(Root *AVLNode, Node *AVLNode, less func(*AVLNode, *AVLNode) bool) *AVLNode {
	if Root == nil {
		return Root
	}
	// check a with b
	if less(Node, Root) {
		Root.Left = deleteNode(Root.Left, Node, less)

		// check b with a
	} else if less(Root, Node) {
		Root.Right = deleteNode(Root.Right, Node, less)
	} else {
		if Root.Left == nil {
			return Root.Right
		} else if Root.Right == nil {
			return Root.Left
		}

		successor := minNode(Root.Right)

		Root.Right = deleteNode(Root.Right, successor, less)

		successor.Left = Root.Left
		successor.Right = Root.Right
		successor.Parent = Root.Parent

		if successor.Left != nil {
			successor.Left.Parent = successor
		}
		if successor.Right != nil {
			successor.Right.Parent = successor
		}

		avlUpdateHeight(successor)
		return avlRebalance(successor)
	}

	avlUpdateHeight(Root)
	if avlIsBalanced(Root) {
		return Root
	}

	return avlRebalance(Root)
}

func avlSearch(root *AVLNode, target *AVLNode, less func(*AVLNode, *AVLNode) bool) *AVLNode {
	node := root
	for node != nil {
		if less(target, node) {
			// target is strictly less than node
			node = node.Left
		} else if less(node, target) {
			// node is strictly less than target
			node = node.Right
		} else {
			// neither is less than the other → equal → found
			return node
		}
	}
	// not found
	return nil
}
