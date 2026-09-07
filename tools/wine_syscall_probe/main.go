package main

import "fmt"

func rawGetpid() uint64

func main() {
	fmt.Println("issuing raw Linux SYSCALL instruction (getpid, number 39) from a windows/amd64 binary...")
	pid := rawGetpid()
	fmt.Printf("raw syscall returned: %d\n", pid)
	if pid > 0 && pid < 4194304 {
		fmt.Println("RESULT: PASS -- looks like a plausible Linux PID. Wine let the raw syscall through.")
	} else {
		fmt.Println("RESULT: UNCERTAIN -- ran without crashing, but the value doesn't look like a PID.")
	}
}
