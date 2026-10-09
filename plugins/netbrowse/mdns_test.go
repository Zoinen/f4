package netbrowse

import (
	"encoding/binary"
	"reflect"
	"strings"
	"testing"
	"time"
)

func dnsName(name string) []byte {
	var b []byte
	for _, l := range strings.Split(name, ".") {
		// #nosec G115 -- test names have short labels.
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	return append(b, 0)
}

func dnsRecord(owner []byte, typ uint16, rdata []byte) []byte {
	b := append([]byte(nil), owner...)
	b = binary.BigEndian.AppendUint16(b, typ)
	b = binary.BigEndian.AppendUint16(b, dnsClassIN)
	b = binary.BigEndian.AppendUint32(b, 120)
	// #nosec G115 -- test records are tiny.
	b = binary.BigEndian.AppendUint16(b, uint16(len(rdata)))
	return append(b, rdata...)
}

func srvData(target []byte) []byte {
	b := []byte{0, 0, 0, 0, 0x01, 0xBD} // priority, weight, port 445
	return append(b, target...)
}

// dnsResponse is a response with the given records in its answer section.
func dnsResponse(records ...[]byte) []byte {
	msg := make([]byte, 12)
	msg[2] = 0x84
	// #nosec G115 -- a handful of test records.
	binary.BigEndian.PutUint16(msg[6:], uint16(len(records)))
	for _, r := range records {
		msg = append(msg, r...)
	}
	return msg
}

func TestBuildPTRQueryAsksForTheSMBService(t *testing.T) {
	q := buildPTRQuery(smbService)
	if binary.BigEndian.Uint16(q[4:]) != 1 || q[2]&0x80 != 0 {
		t.Fatalf("header = % x", q[:12])
	}
	name, off, ok := readDNSName(q, 12)
	if !ok || name != smbService {
		t.Fatalf("question name = %q, %v", name, ok)
	}
	if typ, class := binary.BigEndian.Uint16(q[off:]), binary.BigEndian.Uint16(q[off+2:]); typ != dnsTypePTR || class != 0x8001 {
		t.Errorf("type %d class %#x", typ, class)
	}
}

func TestSMBHostsInAnswerReadsSRVTargets(t *testing.T) {
	inst := dnsName("NAS._smb._tcp.local")
	msg := dnsResponse(
		dnsRecord(dnsName(smbService), dnsTypePTR, inst),
		dnsRecord(inst, dnsTypeSRV, srvData(dnsName("nas.local"))),
		dnsRecord(dnsName("printer._ipp._tcp.local"), dnsTypeSRV, srvData(dnsName("printer.local"))),
		dnsRecord(dnsName("Bad._smb._tcp.local"), dnsTypeSRV, srvData(dnsName("a b\\c"))),
	)
	got := smbHostsInAnswer(msg)
	if !reflect.DeepEqual(got, []string{"nas.local"}) {
		t.Fatalf("hosts = %v", got)
	}
}

func TestSMBHostsInAnswerFollowsNameCompression(t *testing.T) {
	msg := dnsResponse()
	ownerOff := len(msg)
	msg = append(msg, dnsName("NAS._smb._tcp.local")...)
	localOff := ownerOff + strings.Index(string(msg[ownerOff:]), "\x05local")
	// #nosec G115 -- the test message is a few dozen bytes.
	target := append([]byte{3, 'n', 'a', 's'}, 0xC0|byte(localOff>>8), byte(localOff))
	rec := dnsRecord(nil, dnsTypeSRV, srvData(target))
	// The owner is written once above; the record body follows it.
	msg = append(msg, rec...)
	binary.BigEndian.PutUint16(msg[6:], 1)
	if got := smbHostsInAnswer(msg); !reflect.DeepEqual(got, []string{"nas.local"}) {
		t.Fatalf("hosts = %v", got)
	}
}

func TestSMBHostsInAnswerSurvivesGarbage(t *testing.T) {
	inst := dnsName("NAS._smb._tcp.local")
	msg := dnsResponse(dnsRecord(inst, dnsTypeSRV, srvData(dnsName("nas.local"))))
	for n := 0; n < len(msg); n++ {
		_ = smbHostsInAnswer(msg[:n]) // must not panic
	}
	loop := dnsResponse([]byte{0xC0, 12, 0, dnsTypeSRV})
	if got := smbHostsInAnswer(loop); got != nil {
		t.Errorf("a compression loop gave %v", got)
	}
	query := buildPTRQuery(smbService)
	if got := smbHostsInAnswer(query); got != nil {
		t.Errorf("a query gave %v", got)
	}
}

func TestDiscoveredHostsAreCachedAndMergedIntoTheTop(t *testing.T) {
	withFakeSMB(t)
	calls := 0
	old := discoverHosts
	discoverHosts = func() []string { calls++; return []string{"nas.local", "Alpha"} }
	t.Cleanup(func() { discoverHosts = old })
	RememberHost("alpha")
	RememberHost("zed")
	top, err := enumerateSMB(nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range top {
		names = append(names, r.Remote)
	}
	if want := []string{`\\alpha`, `\\nas.local`, `\\zed`}; !reflect.DeepEqual(names, want) {
		t.Fatalf("servers = %v, want %v", names, want)
	}
	if _, err := enumerateSMB(nil); err != nil || calls != 1 {
		t.Errorf("discovery ran %d times within its TTL (err %v)", calls, err)
	}
	discovery.Lock()
	discovery.at = time.Now().Add(-2 * discoveryTTL)
	discovery.Unlock()
	if _, err := enumerateSMB(nil); err != nil || calls != 2 {
		t.Errorf("stale discovery not refreshed: %d calls", calls)
	}
}
