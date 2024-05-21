package raft

import proto "github.com/lquyet/raft/pb"

func EntriesPointerToValue(ep []*proto.LogEntry) []proto.LogEntry {
	var entries []proto.LogEntry
	for _, e := range ep {
		entries = append(entries, *e)
	}
	return entries
}

func EntriesValueToPointer(ev []proto.LogEntry) []*proto.LogEntry {
	var entries []*proto.LogEntry
	for _, e := range ev {
		entries = append(entries, &e)
	}
	return entries
}

func ToInt32(v int) int32 {
	return int32(v)
}
