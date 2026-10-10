package panel

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestPumpPTYOutputKeepsOrderAndReportsTheEnd(t *testing.T) {
	var chunks [][]byte
	for i := byte(0); i < 200; i++ {
		chunks = append(chunks, bytes.Repeat([]byte{i}, 1+int(i)%7))
	}
	i := 0
	errEnd := errors.New("terminal gone")
	read := func(b []byte) (int, error) {
		if i == len(chunks) {
			return 0, errEnd
		}
		n := copy(b, chunks[i])
		i++
		return n, nil
	}
	var got []byte
	err := pumpPTYOutput(read, func(c []byte) { got = append(got, c...) })
	if err != errEnd {
		t.Fatalf("error = %v, want the reader's", err)
	}
	if !bytes.Equal(got, bytes.Join(chunks, nil)) {
		t.Fatal("the parser did not see every byte in order, or saw the end before the data")
	}
}

// A slow parser must not stop the read: the reader gets through many chunks
// while the first one is still being consumed.
func TestPumpPTYOutputReadsWhileTheParserIsBusy(t *testing.T) {
	release := make(chan struct{})
	readCount := byte(0)
	readAhead := make(chan struct{})
	read := func(b []byte) (int, error) {
		if readCount == 20 {
			close(readAhead)
			<-release // hold the end until the test has looked
			return 0, errors.New("done")
		}
		readCount++
		b[0] = readCount
		return 1, nil
	}
	consumed := 0
	done := make(chan error, 1)
	go func() {
		done <- pumpPTYOutput(read, func([]byte) {
			consumed++
			if consumed == 1 {
				<-release // the parser is stuck on the first chunk
			}
		})
	}()
	select {
	case <-readAhead:
	case <-time.After(5 * time.Second):
		t.Fatal("the reader was held up by the parser")
	}
	close(release)
	if err := <-done; err == nil || consumed != 20 {
		t.Fatalf("err=%v consumed=%d, want the 20 chunks consumed", err, consumed)
	}
}
