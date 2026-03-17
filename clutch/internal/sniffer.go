//go:build linux

package clutch

import (
	"encoding/binary"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// StartSniffer launches the sniffer goroutines (AF_PACKET + heartbeat).
// Non-blocking: returns nil immediately (or logs warning if caps missing).
func StartSniffer() error {
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(htons(syscall.ETH_P_ALL)))
	if err != nil {
		log.Printf("sniffer: socket: %v (need CAP_NET_RAW) — sniffer disabled", err)
		return nil // non-fatal
	}

	log.Printf("sniffer: started (listening on all interfaces)")
	go monitorHeartbeatLoop()
	go runCapture(fd)
	return nil
}

func runCapture(fd int) {
	defer syscall.Close(fd)

	buf := make([]byte, 65536)
	for {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			log.Printf("sniffer: recvfrom: %v", err)
			continue
		}
		pkt := buf[:n]
		dstIP, _, dstPort, payload := extractPacketInfo(pkt)
		if dstIP == "" {
			continue
		}
		if isPrivateIP(dstIP) {
			continue
		}

		var domain, proto string
		if len(payload) > 0 {
			if dstPort == 443 || dstPort == 8443 {
				domain = parseTLSClientHelloSNI(payload)
				proto = "TLS"
			} else if dstPort == 80 {
				domain = parseHTTPHost(payload)
				proto = "HTTP"
			}
		}
		al, _ := Allowlist.Load().([]string)

		if domain == "" {
			// For web ports (80/443/8443), an empty domain means this is a
			// SYN/ACK or keepalive packet — no payload to extract a hostname from.
			// Skip it: the TLS ClientHello or HTTP request will arrive in a
			// subsequent packet and will be checked then.
			if dstPort == 80 || dstPort == 443 || dstPort == 8443 {
				continue
			}
			// For all other ports: enforce based on IP only.
			// These connections have no hostname and can only be matched by IP/CIDR.
			if len(al) == 0 {
				continue
			}
			if !matchesAllowlistIP(dstIP, al) {
				rejectConnection(dstIP, dstPort)
				Stats.Blocked.Add(1)
				log.Printf("sniffer: blocked direct IP %s (port %d)", dstIP, dstPort)
				bufferLog(map[string]any{
					"domain":    dstIP,
					"method":    proto,
					"path":      "",
					"action":    "blocked",
					"status":    0,
					"latencyMs": 0,
					"ts":        time.Now().UTC().Format(time.RFC3339),
				})
			}
			continue
		}

		action := "allowed"
		if len(al) > 0 {
			if !matchesAllowlist(domain, al) {
				action = "blocked"
				rejectConnection(dstIP, dstPort)
				Stats.Blocked.Add(1)
			} else {
				Stats.Allowed.Add(1)
				Stats.ReqCount.Add(1)
			}
		}

		log.Printf("sniffer: %s %s (port %d)", action, domain, dstPort)
		bufferLog(map[string]any{
			"domain":    domain,
			"method":    proto,
			"path":      "",
			"action":    action,
			"status":    0,
			"latencyMs": 0,
			"ts":        time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func monitorHeartbeatLoop() {
	tick := time.NewTicker(30 * time.Second)
	sendMonitorHeartbeat()
	for range tick.C {
		sendMonitorHeartbeat()
	}
}

func sendMonitorHeartbeat() {
	resp, err := CpDo("POST", "/monitor/heartbeat", map[string]string{})
	if err != nil {
		log.Printf("sniffer: heartbeat: %v", err)
		return
	}
	resp.Body.Close()
}

func rejectConnection(dstIP string, dstPort int) {
	dport := strconv.Itoa(dstPort)
	check := []string{"-C", "OUTPUT", "-d", dstIP, "-p", "tcp", "--dport", dport, "-j", "REJECT"}
	if exec.Command("iptables", check...).Run() == nil {
		return
	}
	out := []string{"-A", "OUTPUT", "-d", dstIP, "-p", "tcp", "--dport", dport, "-j", "REJECT"}
	in := []string{"-A", "INPUT", "-s", dstIP, "-p", "tcp", "--sport", dport, "-j", "REJECT"}
	if err := exec.Command("iptables", out...).Run(); err != nil {
		log.Printf("sniffer: iptables OUTPUT rule for %s:%s: %v", dstIP, dport, err)
	}
	if err := exec.Command("iptables", in...).Run(); err != nil {
		log.Printf("sniffer: iptables INPUT rule for %s:%s: %v", dstIP, dport, err)
	}
	log.Printf("sniffer: blocked %s port %s via iptables", dstIP, dport)
}

var privateRanges []*net.IPNet

func init() {
	for _, cidr := range []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"127.0.0.0/8", "169.254.0.0/16",
		"::1/128", "fc00::/7", "fe80::/10",
	} {
		_, n, _ := net.ParseCIDR(cidr)
		if n != nil {
			privateRanges = append(privateRanges, n)
		}
	}
}

func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, r := range privateRanges {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}

func htons(v uint16) uint16 {
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], v)
	return *(*uint16)(unsafe.Pointer(&buf[0]))
}

func extractPacketInfo(pkt []byte) (dstIP string, srcPort, dstPort int, payload []byte) {
	if len(pkt) < 14 {
		return
	}

	etherType := binary.BigEndian.Uint16(pkt[12:14])
	const (
		etherTypeIPv4  = 0x0800
		etherTypeIPv6  = 0x86DD
		etherType8021Q = 0x8100
	)

	offset := 14
	switch etherType {
	case etherType8021Q:
		if len(pkt) < 18 {
			return
		}
		etherType = binary.BigEndian.Uint16(pkt[16:18])
		offset = 18
		fallthrough
	case etherTypeIPv4, etherTypeIPv6:
		// handled below
	default:
		return
	}

	ip := pkt[offset:]
	var tcp []byte
	switch etherType {
	case etherTypeIPv4:
		if len(ip) < 20 {
			return
		}
		if ip[9] != 6 {
			return
		}
		dstIP = net.IP(ip[16:20]).String()
		ihl := int(ip[0]&0x0f) * 4
		if len(ip) < ihl {
			dstIP = ""
			return
		}
		tcp = ip[ihl:]
	case etherTypeIPv6:
		if len(ip) < 40 {
			return
		}
		if ip[6] != 6 {
			return
		}
		dstIP = net.IP(ip[24:40]).String()
		tcp = ip[40:]
	default:
		return
	}

	if len(tcp) < 20 {
		dstIP = ""
		return
	}
	srcPort = int(binary.BigEndian.Uint16(tcp[0:2]))
	dstPort = int(binary.BigEndian.Uint16(tcp[2:4]))

	tcpHdrLen := int((tcp[12] >> 4) * 4)
	if len(tcp) < tcpHdrLen {
		dstIP = ""
		return
	}
	payload = tcp[tcpHdrLen:]
	return
}

func parseTLSClientHelloSNI(data []byte) string {
	if len(data) < 5 {
		return ""
	}
	if data[0] != 0x16 {
		return ""
	}
	recLen := int(binary.BigEndian.Uint16(data[3:5]))
	if len(data) < 5+recLen {
		return ""
	}
	hs := data[5 : 5+recLen]

	if len(hs) < 4 {
		return ""
	}
	if hs[0] != 0x01 {
		return ""
	}
	hsLen := int(hs[1])<<16 | int(hs[2])<<8 | int(hs[3])
	if len(hs) < 4+hsLen {
		return ""
	}
	hello := hs[4 : 4+hsLen]

	pos := 0
	if len(hello) < 35 {
		return ""
	}
	pos += 2 + 32

	sessionIDLen := int(hello[pos])
	pos++
	pos += sessionIDLen

	if len(hello) < pos+2 {
		return ""
	}
	cipherSuitesLen := int(binary.BigEndian.Uint16(hello[pos : pos+2]))
	pos += 2 + cipherSuitesLen

	if len(hello) < pos+1 {
		return ""
	}
	compressionLen := int(hello[pos])
	pos++
	pos += compressionLen

	if len(hello) < pos+2 {
		return ""
	}
	extLen := int(binary.BigEndian.Uint16(hello[pos : pos+2]))
	pos += 2
	extEnd := pos + extLen

	for pos+4 <= extEnd && pos+4 <= len(hello) {
		extType := binary.BigEndian.Uint16(hello[pos : pos+2])
		extDataLen := int(binary.BigEndian.Uint16(hello[pos+2 : pos+4]))
		pos += 4
		if pos+extDataLen > len(hello) {
			break
		}
		extData := hello[pos : pos+extDataLen]
		pos += extDataLen

		if extType != 0x0000 {
			continue
		}
		if len(extData) < 5 {
			break
		}
		if extData[2] != 0x00 {
			break
		}
		nameLen := int(binary.BigEndian.Uint16(extData[3:5]))
		if len(extData) < 5+nameLen {
			break
		}
		return string(extData[5 : 5+nameLen])
	}
	return ""
}

func parseHTTPHost(payload []byte) string {
	s := string(payload)
	for _, line := range strings.SplitN(s, "\r\n", 50) {
		if len(line) > 5 && strings.EqualFold(line[:5], "Host:") {
			host := strings.TrimSpace(line[5:])
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			return host
		}
	}
	return ""
}

func matchesAllowlist(domain string, allowlist []string) bool {
	for _, rule := range allowlist {
		if rule == domain {
			return true
		}
		if len(rule) > 2 && rule[:2] == "*." {
			suffix := rule[1:]
			if domain == rule[2:] || hasSuffix(domain, suffix) {
				return true
			}
		}
	}
	// If the domain is actually an IP (e.g. Host: 1.2.3.4), check IP rules too.
	if net.ParseIP(domain) != nil {
		return matchesAllowlistIP(domain, allowlist)
	}
	return false
}

// matchesAllowlistIP checks an IP string against exact IP and CIDR rules in the allowlist.
func matchesAllowlistIP(ip string, allowlist []string) bool {
	parsed := net.ParseIP(ip)
	for _, rule := range allowlist {
		if rule == ip {
			return true
		}
		if strings.Contains(rule, "/") {
			_, cidr, err := net.ParseCIDR(rule)
			if err == nil && parsed != nil && cidr.Contains(parsed) {
				return true
			}
		}
	}
	return false
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
