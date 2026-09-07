package main

// printableStream removes terminal control sequences for marker observation.
// It is not a screen or row reconstruction: bytes are retained in order and
// no boundary is inferred; logical line boundaries remain explicit LF bytes.
func printableStream(data []byte) []byte {
	result := make([]byte, 0, len(data))
	state := byte(0)
	for _, b := range data {
		switch state {
		case 1: // ESC
			if b == '[' {
				state = 2
			} else if b == ']' {
				state = 3
			} else {
				state = 0
			}
		case 2: // CSI
			if b >= 0x40 && b <= 0x7e {
				state = 0
			}
		case 3: // OSC
			if b == 0x07 {
				state = 0
			} else if b == 0x1b {
				state = 4
			}
		case 4: // OSC ST candidate
			if b == '\\' {
				state = 0
			} else {
				state = 3
			}
		default:
			if b == 0x1b {
				state = 1
			} else if b >= 0x20 || b == '\t' || b == '\r' || b == '\n' {
				result = append(result, b)
			}
		}
	}
	return result
}
