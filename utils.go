package distributed_lock

import proto "github.com/lquyet/distributed-lock/pb"

func EntriesPointerToValue(ep []*proto.LogEntry) []proto.LogEntry {
	var entries []proto.LogEntry
	for _, e := range ep {
		entries = append(entries, *e)
	}
	return entries
}
