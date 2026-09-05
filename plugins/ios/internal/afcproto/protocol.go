package afcproto

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	magic            uint64 = 0x4141504c36414643 // "CFA6LPAA" on the wire.
	headerSize       uint64 = 40
	maxPacketSize    uint64 = 64 << 20
	maxHeaderPayload uint64 = 1 << 20
	maxIOChunk              = 256 << 10
	maxPathBytes            = 16 << 10
)

type opcode uint64

const (
	opStatus                opcode = 0x01
	opData                  opcode = 0x02
	opReadDir               opcode = 0x03
	opRemovePath            opcode = 0x08
	opMakeDir               opcode = 0x09
	opGetFileInfo           opcode = 0x0a
	opGetDeviceInfo         opcode = 0x0b
	opFileOpen              opcode = 0x0d
	opFileOpenResult        opcode = 0x0e
	opFileRead              opcode = 0x0f
	opFileWrite             opcode = 0x10
	opFileSeek              opcode = 0x11
	opFileClose             opcode = 0x14
	opFileSetSize           opcode = 0x15
	opRenamePath            opcode = 0x18
	opSetFileModTime        opcode = 0x1e
	opRemovePathAndContents opcode = 0x22
)

type packetHeader struct {
	Magic     uint64
	EntireLen uint64
	ThisLen   uint64
	PacketNum uint64
	Operation opcode
}

type packet struct {
	header        packetHeader
	headerPayload []byte
	payload       []byte
}

func writePacket(w io.Writer, number uint64, operation opcode, headerPayload, payload []byte) error {
	thisLen := headerSize + uint64(len(headerPayload))
	entireLen := thisLen + uint64(len(payload))
	if uint64(len(headerPayload)) > maxHeaderPayload || entireLen > maxPacketSize {
		return fmt.Errorf("%w: outgoing AFC packet is too large", ErrProtocol)
	}

	var rawHeader [headerSize]byte
	binary.LittleEndian.PutUint64(rawHeader[0:8], magic)
	binary.LittleEndian.PutUint64(rawHeader[8:16], entireLen)
	binary.LittleEndian.PutUint64(rawHeader[16:24], thisLen)
	binary.LittleEndian.PutUint64(rawHeader[24:32], number)
	binary.LittleEndian.PutUint64(rawHeader[32:40], uint64(operation))
	if err := writeFull(w, rawHeader[:]); err != nil {
		return err
	}
	if err := writeFull(w, headerPayload); err != nil {
		return err
	}
	return writeFull(w, payload)
}

func readPacket(r io.Reader, expectedNumber uint64) (packet, error) {
	var h packetHeader
	if err := binary.Read(r, binary.LittleEndian, &h); err != nil {
		return packet{}, err
	}
	if h.Magic != magic {
		return packet{}, fmt.Errorf("%w: invalid AFC magic %#x", ErrProtocol, h.Magic)
	}
	if h.PacketNum != expectedNumber {
		return packet{}, fmt.Errorf("%w: response packet number %d, want %d", ErrProtocol, h.PacketNum, expectedNumber)
	}
	if h.ThisLen < headerSize || h.EntireLen < h.ThisLen {
		return packet{}, fmt.Errorf("%w: invalid AFC lengths entire=%d this=%d", ErrProtocol, h.EntireLen, h.ThisLen)
	}
	if h.EntireLen > maxPacketSize || h.ThisLen-headerSize > maxHeaderPayload {
		return packet{}, fmt.Errorf("%w: AFC packet exceeds configured bounds", ErrProtocol)
	}

	// #nosec G115 -- maxPacketSize caps both validated wire lengths at 64 MiB.
	headerLen := int(h.ThisLen - headerSize)
	payloadLen := int(h.EntireLen - h.ThisLen)
	p := packet{
		header:        h,
		headerPayload: make([]byte, headerLen),
		payload:       make([]byte, payloadLen),
	}
	if _, err := io.ReadFull(r, p.headerPayload); err != nil {
		return packet{}, err
	}
	if _, err := io.ReadFull(r, p.payload); err != nil {
		return packet{}, err
	}
	return p, nil
}

func writeFull(w io.Writer, p []byte) error {
	for len(p) != 0 {
		n, err := w.Write(p)
		if n > 0 {
			p = p[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func putUint64s(values ...uint64) []byte {
	b := make([]byte, 8*len(values))
	for i, value := range values {
		binary.LittleEndian.PutUint64(b[i*8:], value)
	}
	return b
}
