package mr

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/rpc"
	"os"
	"sort"
	"time"
)

type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

// Map functions return a slice of KeyValue.
type KeyValue struct {
	Key   string
	Value string
}

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

// main/mrworker.go calls this function.
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	for {
		args := WorkArgs{}
		reply := ReplyArgs{}
		ok := call("Coordinator.GetTask", &args, &reply)

		if !ok {
			fmt.Printf("call faile9d!\n")
		}
		if reply.Task == nil {
			time.Sleep(3 * time.Second)
			continue
		}

		if reply.Task.Type == MAP {
			callMap(*reply.Task, mapf, reply.Round)
		} else {
			callReduce(*reply.Task, reducef, reply.Round)
		}
	}

}
func callReduce(task Task, reducef func(string, []string) string, round int) {
	fileIndex := task.Index
	intermediate := []KeyValue{}

	for i := 0; i < task.NMap; i++ {
		filename := fmt.Sprintf("mr-%d-%d", i, fileIndex)
		file, err := os.Open(filename)

		if err != nil {
			log.Fatalf("cannot open %v", filename)
		}

		dec := json.NewDecoder(file)

		for {
			var kv KeyValue
			if err := dec.Decode(&kv); err != nil {
				break
			}
			intermediate = append(intermediate, kv)
		}
		file.Close()
	}

	sort.Sort(ByKey(intermediate))

	oname := fmt.Sprintf("mr-out-%d", fileIndex)
	ofile, _ := ioutil.TempFile(".", oname)

	//
	// call Reduce on each distinct key in intermediate[],
	// and print the result to mr-out-0.
	//
	i := 0
	for i < len(intermediate) {
		j := i + 1
		for j < len(intermediate) && intermediate[j].Key == intermediate[i].Key {
			j++
		}
		values := []string{}
		for k := i; k < j; k++ {
			values = append(values, intermediate[k].Value)
		}
		output := reducef(intermediate[i].Key, values)

		// this is the correct format for each line of Reduce output.
		fmt.Fprintf(ofile, "%v %v\n", intermediate[i].Key, output)

		i = j
	}

	os.Rename(ofile.Name(), oname)

	CallReport(task, round)

}

func callMap(task Task, mapf func(string, string) []KeyValue, Round int) {

	file, err := os.Open(task.Filename)
	if err != nil {
		log.Fatalf("cannot open %v", task.Filename)
	}

	content, err := ioutil.ReadAll(file)

	if err != nil {
		log.Fatalf("cannot read %v", task.Filename)
	}

	file.Close()

	kva := mapf(task.Filename, string(content))

	for i := 0; i < task.NReduce; i++ {
		imtFilename := fmt.Sprintf("mr-%d-%d", task.ID, i)

		imtFile, err := ioutil.TempFile(".", imtFilename)

		enc := json.NewEncoder(imtFile)

		if err != nil {
			log.Fatalf("cannot create %v", imtFilename)
		}

		for _, kv := range kva {
			hash := ihash(kv.Key) % task.NReduce
			if hash == i {
				err := enc.Encode(&kv)
				if err != nil {
					log.Fatalf("cannot encode %v", kv)
				}
			}
		}

		imtFile.Close()

		os.Rename(imtFile.Name(), imtFilename)
	}

	CallReport(task, task.Phase)
}

func CallReport(task Task, round int) {
	args := ReportArgs{
		Task:  task,
		Round: round,
	}
	reply := ReportReply{}

	ok := call("Coordinator.ReportTask", &args, &reply)
	if !ok {
		fmt.Printf("ReportTask RPC failed for task %v\n", task)
	} else if reply.Finished {
		fmt.Printf("Task %v successfully reported\n", task)
	}
}

// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply)
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
}

// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
