package mr

//
// RPC definitions.
//
// remember to capitalize all names.
//

import (
	"os"
	"strconv"
	"time"
)

//
// example to show how to declare the arguments
// and reply for an RPC.
//

type ExampleArgs struct {
	X int
}

type ExampleReply struct {
	Y int
}

// Add your RPC definitions here.

type TaskType bool

const (
	MAP    TaskType = true
	REDUCE TaskType = false
)

type Task struct {
	ID        int
	Type      TaskType //map or reduce
	Filename  string   //for maptasks
	State     int
	Index     int
	NMap      int
	NReduce   int
	Phase     int
	StartTime time.Time
}

type WorkArgs struct {
	WorkerID int
}

type ReplyArgs struct {
	Task  *Task
	Round int
}
type ReportArgs struct {
	Task  Task
	Round int
}
type ReportReply struct {
	Finished bool
}

// Cook up a unique-ish UNIX-domain socket name
// in /var/tmp, for the coordinator.
// Can't use the current directory since
// Athena AFS doesn't support UNIX-domain sockets.
func coordinatorSock() string {
	s := "/var/tmp/5840-mr-"
	s += strconv.Itoa(os.Getuid())
	return s
}
