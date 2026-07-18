package utils

import (
	"math"
)

type HeapItem struct {
	Val uint64 // expiration timestamp in milliseconds
	Ref *int   // points to Entry.HeapIdx
	// ↑ NOT a pointer to Entry, a pointer to the INDEX field
}

// pos -> inserted at the last position
func heapUp(heap []HeapItem, pos int) int {
	save := heap[pos]

	for pos > 0 {
		parentPos := int(math.Floor(float64(pos-1) / 2))

		if heap[parentPos].Val <= save.Val {
			break
		}

		heap[pos] = heap[parentPos]
		*heap[pos].Ref = pos
		pos = parentPos
	}

	heap[pos] = save
	*heap[pos].Ref = pos
	return pos
}

func heapDown(heap []HeapItem, pos int) {
	// save the current value to restore later
	save := heap[pos]

	for {
		// positions of left and right child
		leftChildPos := 2*pos + 1
		rightChildPos := 2*pos + 2
		// track min value from both the child using minValue for value and minIndex for index
		minIndex := pos
		minValue := save.Val

		// pick minimum of both right and left child
		if rightChildPos < len(heap) && heap[rightChildPos].Val < minValue {
			minIndex = rightChildPos
			minValue = heap[rightChildPos].Val
		}

		if leftChildPos < len(heap) && heap[leftChildPos].Val < minValue {
			minIndex = leftChildPos
			minValue = heap[leftChildPos].Val
		}

		// if minIndex == pos, we've found the correct position already no need to swap
		if minIndex == pos {
			break
		}
		// move into pos, therefore save ref to pos
		// ignore the moved from part, because that doesn't really matter
		heap[pos] = heap[minIndex]
		*heap[pos].Ref = pos
		pos = minIndex
	}

	heap[pos] = save
	*heap[pos].Ref = pos
}

func InsertHeap(heap *[]HeapItem, item HeapItem) int {
	// insert at the end
	pos := len(*heap)
	*heap = append(*heap, item)

	// slice is being passed by reference, so heapUp will modify it directly
	return heapUp(*heap, pos)
}

func DeleteHeap(heap *[]HeapItem, pos int) {
	last := len(*heap) - 1
	if last != pos {
		(*heap)[pos] = (*heap)[last]
		*(*heap)[pos].Ref = pos
	}

	// from 0 to last, excluding the last element
	*heap = (*heap)[:last]
	if pos < len(*heap) {
		heapDown(*heap, pos)
	}
}

func UpdateHeap(heap []HeapItem, pos int) {
	parentPos := int(math.Floor(float64(pos-1) / 2))

	if pos > 0 && heap[parentPos].Val > heap[pos].Val {
		heapUp(heap, pos)
	} else {
		heapDown(heap, pos)
	}
}

// heapUpsert handles both "insert new" and "update existing".
// -1 is the sentinel for "not yet in the heap"; any other value is a valid index.
func HeapUpsert(heap *[]HeapItem, pos int, item HeapItem) {
	// pos != -1 means the entry already has a slot in the heap, so update it.
	if pos != -1 && pos < len(*heap) {
		(*heap)[pos] = item
		*(*heap)[pos].Ref = pos
		UpdateHeap(*heap, pos)
	} else {
		// pos is zero or out of bounds, so insert at the end
		InsertHeap(heap, item)
	}
}

// function for debugging
func IsValidHeap(heap []HeapItem) bool {
	for i, v := range heap {
		rightChild := 2*i + 2
		leftChild := 2*i + 1
		maxValue := v.Val

		if rightChild < len(heap) && heap[rightChild].Val > maxValue {
			maxValue = heap[rightChild].Val
		}
		if leftChild < len(heap) && heap[leftChild].Val > maxValue {
			maxValue = heap[leftChild].Val
		}

		if maxValue != v.Val {
			continue
		}

		if v.Val > maxValue {
			return false
		}
	}

	return true
}
