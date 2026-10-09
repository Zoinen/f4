package netbrowse

import (
	"encoding/binary"
	"net"
	"strings"
	"sync"
	"time"
)

// Finding SMB servers on the local network (f4#1702, part 3): a one-shot
// multicast DNS query for _smb._tcp.local, the service Samba (through Avahi),
// macOS and most NAS boxes announce. It needs no dependency: the query is one
// packet and the answers are read with the few record types that matter (PTR,
// SRV). Windows machines do not answer this, so on a Windows-only LAN the list
// stays what the session has reached; a server can always be named by hand.

const (
	dnsTypePTR  = 12
	dnsTypeSRV  = 33
	dnsClassIN  = 1
	smbService  = "_smb._tcp.local"
	mdnsAddress = "224.0.0.251:5353"
)

// buildPTRQuery is a DNS query for the PTR records of name. The QU bit is set,
// so responders may reply straight to us rather than to the multicast group.
func buildPTRQuery(name string) []byte {
	msg := make([]byte, 12, 64)
	binary.BigEndian.PutUint16(msg[4:], 1) // one question
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		// #nosec G115 -- the fixed service name has short labels.
		msg = append(msg, byte(len(label)))
		msg = append(msg, label...)
	}
	msg = append(msg, 0)
	msg = binary.BigEndian.AppendUint16(msg, dnsTypePTR)
	msg = binary.BigEndian.AppendUint16(msg, 0x8000|dnsClassIN)
	return msg
}

// readDNSName reads a possibly compressed name at off, and returns it with
// the offset just after it in the original position.
func readDNSName(msg []byte, off int) (string, int, bool) {
	var labels []string
	next := -1
	for hops := 0; hops < 32; hops++ {
		if off >= len(msg) {
			return "", 0, false
		}
		n := int(msg[off])
		switch {
		case n == 0:
			if next < 0 {
				next = off + 1
			}
			return strings.Join(labels, "."), next, true
		case n&0xC0 == 0xC0:
			if off+1 >= len(msg) {
				return "", 0, false
			}
			if next < 0 {
				next = off + 2
			}
			off = (n&0x3F)<<8 | int(msg[off+1])
		case n&0xC0 != 0:
			return "", 0, false
		default:
			if off+1+n > len(msg) {
				return "", 0, false
			}
			labels = append(labels, string(msg[off+1:off+1+n]))
			off += 1 + n
		}
	}
	return "", 0, false
}

// smbHostsInAnswer returns the host names an mDNS response gives for
// _smb._tcp: the targets of its SRV records, and the hosts named by its A
// records' owners when a PTR points at an instance without an SRV.
func smbHostsInAnswer(msg []byte) []string {
	if len(msg) < 12 || msg[2]&0x80 == 0 { // not a response
		return nil
	}
	questions := int(binary.BigEndian.Uint16(msg[4:]))
	records := int(binary.BigEndian.Uint16(msg[6:])) + int(binary.BigEndian.Uint16(msg[8:])) + int(binary.BigEndian.Uint16(msg[10:]))
	off := 12
	for i := 0; i < questions; i++ {
		_, n, ok := readDNSName(msg, off)
		if !ok || n+4 > len(msg) {
			return nil
		}
		off = n + 4
	}
	var hosts []string
	for i := 0; i < records; i++ {
		owner, n, ok := readDNSName(msg, off)
		if !ok || n+10 > len(msg) {
			break
		}
		typ := binary.BigEndian.Uint16(msg[n:])
		rdlen := int(binary.BigEndian.Uint16(msg[n+8:]))
		data := n + 10
		if data+rdlen > len(msg) {
			break
		}
		off = data + rdlen
		if typ == dnsTypeSRV && rdlen >= 7 && strings.HasSuffix(strings.ToLower(owner), "."+smbService) {
			if target, _, ok := readDNSName(msg, data+6); ok && validHostName(target) {
				hosts = append(hosts, target)
			}
		}
	}
	return hosts
}

// validHostName admits what a machine can be called on a network: letters,
// digits, dots, dashes and underscores. A name from the network goes into a
// smb:// address, so nothing else is let through.
func validHostName(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// mdnsDiscover asks the local network for SMB servers and collects answers for
// wait. Failure of any kind (no multicast, no network) is an empty result: a
// missing list must not stop the network browser.
func mdnsDiscover(wait time.Duration) []string {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil
	}
	defer func() { _ = conn.Close() }()
	dst, err := net.ResolveUDPAddr("udp4", mdnsAddress)
	if err != nil {
		return nil
	}
	if _, err := conn.WriteTo(buildPTRQuery(smbService), dst); err != nil {
		return nil
	}
	deadline := time.Now().Add(wait)
	_ = conn.SetReadDeadline(deadline)
	seen := map[string]bool{}
	var found []string
	buf := make([]byte, 9000)
	for time.Now().Before(deadline) {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			break
		}
		for _, h := range smbHostsInAnswer(buf[:n]) {
			h = strings.TrimSuffix(h, ".")
			if k := strings.ToLower(h); !seen[k] {
				seen[k] = true
				found = append(found, h)
			}
		}
	}
	return found
}

// discoverHosts is what the Network view calls; a variable so tests do not go
// to the network.
var discoverHosts = func() []string { return mdnsDiscover(1200 * time.Millisecond) }

const discoveryTTL = 2 * time.Minute

var discovery = struct {
	sync.Mutex
	at    time.Time
	hosts []string
}{}

// discoveredHosts is the last discovery result, refreshed at most every
// discoveryTTL so re-reading the top of the Network view stays quick.
func discoveredHosts() []string {
	discovery.Lock()
	defer discovery.Unlock()
	if discovery.at.IsZero() || time.Since(discovery.at) > discoveryTTL {
		discovery.hosts = discoverHosts()
		discovery.at = time.Now()
	}
	return append([]string(nil), discovery.hosts...)
}
