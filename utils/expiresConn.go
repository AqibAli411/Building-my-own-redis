package utils

type Dlist struct {
	Next *Dlist
	Prev *Dlist
}

// creates and returns the dummmy node for the doubly linked list
func DlistInit(dummy *Dlist) *Dlist {
	dummy.Next = dummy
	dummy.Prev = dummy
	return dummy
}

// returns true if the list is empty, false otherwise
func DlistEmpty(d *Dlist) bool {
	return d.Next == d
}

// removes the node from the list, leaving the list intact
func DlistDetach(node *Dlist) {
	node.Prev.Next = node.Next
	node.Next.Prev = node.Prev
	// GC would handle the free pointer
}

// inserts the node before the target node in the list
func DlistInsertBefore(target *Dlist, node *Dlist) {
	target.Prev.Next = node
	node.Next = target
	node.Prev = target.Prev
	target.Prev = node
}
