package panel

import "sync"

// ptyReadChunk is how much one read of the terminal may take. It was 32 KiB
// read and parsed in one loop; the ConPTY host writes synchronously and stalls
// whenever its pipe is full, so the time the parser spends must not be time the
// pipe goes unread (unxed/f4#1681, idea 1.4).
const ptyReadChunk = 64 << 10

// ptyReadQueue is how many chunks may wait for the parser before the reader
// blocks; 64 chunks of 64 KiB is 4 MiB of slack, enough to ride out a slow
// frame without letting a runaway program grow the process without bound.
const ptyReadQueue = 64

// pumpPTYOutput reads with read on its own goroutine and hands every chunk to
// consume, in order, on another, so that parsing never holds up the read. It
// returns when read fails, after consume has seen everything that was read, and
// reports that error; the caller decides what a finished terminal means.
func pumpPTYOutput(read func([]byte) (int, error), consume func([]byte)) error {
	queue := make(chan []byte, ptyReadQueue)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for chunk := range queue {
			consume(chunk)
		}
	}()

	var err error
	buf := make([]byte, ptyReadChunk)
	for {
		var n int
		n, err = read(buf)
		if n > 0 {
			// The reader reuses buf, the parser keeps the chunk until it is done.
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			queue <- chunk
		}
		if err != nil {
			break
		}
	}
	close(queue)
	wg.Wait()
	return err
}
