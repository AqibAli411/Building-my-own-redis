package main

import (
	"log"
	"math"
	"unsafe"

	"aqib.builds/utils"
	"golang.org/x/sys/unix"
)

func getMonotonicMs() uint64 {
	var ts unix.Timespec
	err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts)
	if err != nil {
		return 0
	}
	return uint64(ts.Sec*1000) + uint64(ts.Nsec/1_000_000)
}

func entryFromHeapRef(ref *int) *utils.Entry {
	offset := unsafe.Offsetof(utils.Entry{}.HeapIndex)
	ptr := uintptr(unsafe.Pointer(ref)) - offset
	return (*utils.Entry)(unsafe.Pointer(ptr))
}

func nextTimerMs() int32 {
	now := getMonotonicMs()
	nextMs := uint64(math.MaxUint64)

	if !utils.DlistEmpty(&Gdata.IdleList) {
		conn := DlistToConn(Gdata.IdleList.Next)
		expiresAt := conn.LastActiveMs + utils.K_IDLE_TIMEOUT_MS
		nextMs = min(expiresAt, nextMs)
	}

	if len(Gdata.heap) > 0 {
		root := Gdata.heap[0]
		nextMs = min(root.Val, nextMs)
	}

	if nextMs == math.MaxUint64 {
		return -1
	}
	if nextMs <= now {
		return 0
	}
	return int32(nextMs - now)
}

func processTimers(fd2Conn map[int32]*Conn) {
	now := getMonotonicMs()

	for !utils.DlistEmpty(&Gdata.IdleList) {
		conn := DlistToConn(Gdata.IdleList.Next)
		expiresAt := conn.LastActiveMs + utils.K_IDLE_TIMEOUT_MS

		if expiresAt >= now {
			break
		}
		log.Println("closing idle connection")
		destroyConn(conn, fd2Conn)
	}

	workDone := 0
	for len(Gdata.heap) > 0 && Gdata.heap[0].Val <= now && workDone < 2000 {
		entry := entryFromHeapRef(Gdata.heap[0].Ref)
		utils.DeleteHeap(&Gdata.heap, 0)
		entry.HeapIndex = -1
		utils.HmapDetach(&Gdata.DB, &entry.Node, utils.NodeEq)
		entryDel(entry)
		workDone++
	}
}

func connUpdateTimer(conn *Conn) {
	conn.LastActiveMs = getMonotonicMs()
	utils.DlistDetach(&conn.IdleNode)
	utils.DlistInsertBefore(&Gdata.IdleList, &conn.IdleNode)
}

func entrySetTTL(entry *utils.Entry, ttlMs int64) {
	if ttlMs < 0 {
		if entry.HeapIndex == -1 {
			return
		}
		utils.DeleteHeap(&Gdata.heap, entry.HeapIndex)
		entry.HeapIndex = -1
	} else {
		expiresAt := getMonotonicMs() + uint64(ttlMs)
		item := utils.HeapItem{Val: expiresAt, Ref: &entry.HeapIndex}
		utils.HeapUpsert(&Gdata.heap, entry.HeapIndex, item)
	}
}
