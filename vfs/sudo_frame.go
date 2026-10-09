package vfs

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

// sudoFrameLimit bounds one message of the elevated channel. Requests and
// answers are paths and a few directory entries; the length prefix is read
// from a peer that has not been authenticated yet, so it must not decide how
// much the elevated side allocates.
const sudoFrameLimit = 64 << 20

// writeSudoFrame sends msg as a 4-byte little-endian length and its JSON, the
// framing sendMsg uses on the Unix socket, without a descriptor to pass along.
func writeSudoFrame(w io.Writer, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if uint64(len(data)) > sudoFrameLimit || uint64(len(data)) > math.MaxUint32 {
		return fmt.Errorf("sudo IPC message is too large: %d bytes", len(data))
	}
	frame := make([]byte, 4+len(data))
	// #nosec G115 -- the length was checked against sudoFrameLimit above.
	binary.LittleEndian.PutUint32(frame, uint32(len(data)))
	copy(frame[4:], data)
	_, err = w.Write(frame)
	return err
}

// readSudoFrame reads one message written by writeSudoFrame.
func readSudoFrame(r io.Reader, msg any) error {
	var prefix [4]byte
	if _, err := io.ReadFull(r, prefix[:]); err != nil {
		return err
	}
	length := binary.LittleEndian.Uint32(prefix[:])
	if length > sudoFrameLimit {
		return fmt.Errorf("sudo IPC message is too large: %d bytes", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, msg)
}
