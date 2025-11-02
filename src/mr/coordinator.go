package mr

import (
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sync"
	"time"
)

const (
	StateIdle    = 0
	StateRunning = 1
	StateDone    = 2
)

type Coordinator struct {
	mu       sync.Mutex
	Tasks    []Task
	Filename []string
	NReduce  int
	NMap     int
	Phase    int // 0 = map, 1 = reduce, 2 = done
	done     bool
}

func (c *Coordinator) GetTask(args *WorkArgs, reply *ReplyArgs) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// if map phase
	if c.Phase == 0 {
		for i := range c.Tasks {
			if c.Tasks[i].Type == MAP && c.Tasks[i].State == StateIdle {
				c.Tasks[i].State = StateRunning
				c.Tasks[i].StartTime = time.Now()
				reply.Task = &c.Tasks[i]
				reply.Round = 0
				return nil
			}
		}
		// no available map task
		reply.Task = nil
		return nil
	}

	// if reduce phase
	if c.Phase == 1 {
		for i := range c.Tasks {
			if c.Tasks[i].Type == REDUCE && c.Tasks[i].State == StateIdle {
				c.Tasks[i].State = StateRunning
				reply.Task = &c.Tasks[i]
				reply.Round = 1
				return nil
			}
		}
		// no available reduce task
		reply.Task = nil
		return nil
	}

	// done phase
	reply.Task = nil
	return nil
}

// RPC handler: workers call this after finishing a task.
func (c *Coordinator) ReportTask(args *ReportArgs, reply *ReportReply) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// find the task
	for i := range c.Tasks {
		if c.Tasks[i].ID == args.Task.ID && c.Tasks[i].Type == args.Task.Type {
			c.Tasks[i].State = StateDone
		}
	}

	// check if we finished map phase
	if c.Phase == 0 {
		allDone := true
		for _, t := range c.Tasks {
			if t.Type == MAP && t.State != StateDone {
				allDone = false
				break
			}
		}
		if allDone {
			// switch to reduce phase
			c.Phase = 1
			// create reduce tasks
			c.Tasks = []Task{}
			for i := 0; i < c.NReduce; i++ {
				c.Tasks = append(c.Tasks, Task{
					ID:      i,
					Type:    REDUCE,
					Index:   i,
					NMap:    c.NMap,
					NReduce: c.NReduce,
					State:   StateIdle,
					Phase:   1,
				})
			}
		}
	} else if c.Phase == 1 {
		allDone := true
		for _, t := range c.Tasks {
			if t.Type == REDUCE && t.State != StateDone {
				allDone = false
				break
			}
		}
		if allDone {
			c.Phase = 2
			c.done = true
		}
	}

	reply.Finished = true
	return nil
}

// start a thread that listens for RPCs from worker.go
func (c *Coordinator) server() {
	rpc.Register(c)
	rpc.HandleHTTP()
	sockname := coordinatorSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

// main/mrcoordinator.go calls Done() periodically
func (c *Coordinator) Done() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.done
}

// create a Coordinator.
// nReduce is the number of reduce tasks to use.
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	c := Coordinator{
		NReduce: nReduce,
		NMap:    len(files),
		Phase:   0,
		done:    false,
	}

	// create initial map tasks
	for i, f := range files {
		c.Tasks = append(c.Tasks, Task{
			ID:       i,
			Type:     MAP,
			Filename: f,
			Index:    i,
			NReduce:  nReduce,
			State:    StateIdle,
			Phase:    0,
		})
	}

	c.server()
	go func() {
		for {
			time.Sleep(2 * time.Second)
			c.mu.Lock()
			if c.done {
				c.mu.Unlock()
				return
			}
			for i := range c.Tasks {
				if c.Tasks[i].State == StateRunning &&
					time.Since(c.Tasks[i].StartTime) > 10*time.Second {
					// worker probably crashed, reset task
					c.Tasks[i].State = StateIdle
				}
			}
			c.mu.Unlock()
		}
	}()
	return &c
}
