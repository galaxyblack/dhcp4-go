package dhcp4

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/packethost/pkg/log"
)

var dlog log.Logger

func Init(l log.Logger) {
	dlog = l.Package("dhcp")
}

var optionFormats = map[Option]func([]byte) (string, string){
	OptionDHCPMsgType:    nil,
	OptionDHCPMaxMsgSize: nil, // func(b []byte) string { return fmt.Sprintf("max_msg_size=%d", binary.BigEndian.Uint16(b)) },
	OptionParameterList:  nil, // func(b []byte) string { return "param_list=..." }
	OptionClientID:       func(b []byte) (string, string) { return "client_id", formatHex(b) },
	OptionClientNDI:      func(b []byte) (string, string) { return "client_ndi", formatNDI(b) },
	OptionDHCPServerID:   func(b []byte) (string, string) { return "dhcp_server", net.IP(b).String() },
	OptionDomainServer:   func(b []byte) (string, string) { return "dns", formatIP(b) },
	OptionHostname:       func(b []byte) (string, string) { return "hostname", string(b) },
	OptionAddressRequest: func(b []byte) (string, string) { return "requested_ip", net.IP(b).String() },
	OptionAddressTime:    func(b []byte) (string, string) { return "lease_time", formatSeconds(b) },
	OptionSubnetMask:     func(b []byte) (string, string) { return "netmask", net.IP(b).String() },
	OptionRouter:         func(b []byte) (string, string) { return "routers", formatIP(b) },
	OptionLogServer:      func(b []byte) (string, string) { return "syslog", formatIP(b) },
	OptionUUIDGUID:       func(b []byte) (string, string) { return "uuid", formatUUID(b[1:]) },
	OptionVendorSpecific: func(b []byte) (string, string) { return "vendor_specific", formatHex(b) },
	OptionClassID:        func(b []byte) (string, string) { return "class_id", fmt.Sprintf("%q", b) },
	OptionClientSystem:   func(b []byte) (string, string) { return "client_arch", fmt.Sprintf("%d", binary.BigEndian.Uint16(b)) },
	OptionDHCPMessage:    func(b []byte) (string, string) { return "msg", fmt.Sprintf("%q", b) },
	OptionUserClass:      func(b []byte) (string, string) { return "user_class", fmt.Sprintf("%q", b) },
}

func SetOptionFormatter(o Option, fn func([]byte) (string, string)) {
	optionFormats[o] = fn
}

func formatHex(b []byte) string {
	const hex = "0123456789abcdef"

	buf := append(make([]byte, 0, len(b)*2+len(b)+2), '"')
	for i, c := range b {
		if i > 0 {
			buf = append(buf, ':')
		}
		buf = append(buf, hex[c>>4], hex[c&0xF])
	}
	buf = append(buf, '"')
	return string(buf)
}

func formatIP(b []byte) string {
	if len(b)%4 != 0 {
		return fmt.Sprintf("%q", b)
	}
	ips := make([]string, 0, len(b)/4)
	for i := 0; i < len(b); i += 4 {
		ips = append(ips, net.IP(b[i:i+4]).String())
	}
	return strings.Join(ips, ",")
}

func formatNDI(b []byte) string {
	if len(b) != 3 {
		return formatHex(b)
	}
	if t := b[0]; t == 1 {
		return fmt.Sprintf("UNDI-%d.%d", b[1], b[2])
	}
	return fmt.Sprintf("%d-%d.%d", b[0], b[1], b[2])
}

func formatSeconds(b []byte) string {
	var (
		secs = binary.BigEndian.Uint32(b)
		dur  = time.Duration(secs) * time.Second
	)
	return dur.String()
}

func formatUUID(b []byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func getPacketFields(p *Packet) []interface{} {
	fields := []interface{}{
		"xid", formatHex(p.XID()),
		"type", p.GetMessageType(),
	}

	if addr := p.GetYIAddr(); !net.IPv4zero.Equal(addr) {
		fields = append(fields, "address", addr)
	}

	if secs := binary.BigEndian.Uint16(p.Secs()); secs > 0 {
		fields = append(fields, "secs", secs)
	}

	if addr := p.GetSIAddr(); !net.IPv4zero.Equal(addr) {
		fields = append(fields, "next_server", addr)
	}

	if filename := p.File(); len(filename) > 0 {
		fields = append(fields, "filename", string(filename))
	}

	return append(fields, optionFields(p.OptionMap)...)
}

func optionFields(om OptionMap) []interface{} {
	fields := []interface{}{}
	for _, o := range om.GetSortedOptions() {
		fn, ok := optionFormats[o]
		if !ok {
			fields = append(fields, fmt.Sprintf("option(%d)", o), om[o])
			continue
		}
		if fn == nil {
			continue
		}
		if k, v := fn(om[o]); k != "" {
			fields = append(fields, k, v)
		}
	}
	return fields
}

func toFields(event string, ifindex int, ip net.IP, req, resp *Packet) []interface{} {
	var gi string
	if req.GetGIAddr().Equal(ip) {
		gi = "via"
	} else {
		gi = "src"
		if resp != nil {
			gi = "dst"
		}
	}
	fields := []interface{}{
		"event", event,
		"mac", req.GetCHAddr(),
		gi, ip,
	}

	if iface, err := net.InterfaceByIndex(ifindex); err == nil {
		fields = append(fields, "iface", iface.Name)
	}

	if resp == nil {
		return append(fields, getPacketFields(req)...)
	} else {
		return append(fields, getPacketFields(resp)...)
	}
}
