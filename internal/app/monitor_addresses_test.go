package app

import (
	"net"
	"reflect"
	"strings"
	"testing"
)

func TestUsableServerAddressClassification(t *testing.T) {
	for _, value := range []string{"8.8.8.8", "1.1.1.1", "::ffff:8.8.8.8", "2606:4700:4700::1111", "2001:4860:4860::8888", "10.1.2.3", "172.16.2.3", "192.168.1.1", "192.168.8.52", "100.64.0.1", "fd00::1", "2001:db8::1"} {
		if _, ok := usableServerAddress(value); !ok {
			t.Errorf("NIC address rejected: %s", value)
		}
	}
	for _, value := range []string{"", "host.example", "127.0.0.1", "0.0.0.0", "169.254.1.2", "224.0.0.1", "255.255.255.255", "fe80::1", "fe80::1%eth0", "2001:4860::1%eth0", "::", "::1", "ff02::1", "198.18.0.1", "198.19.255.254"} {
		if ip, ok := usableServerAddress(value); ok {
			t.Errorf("non-NIC address accepted: %s => %s", value, ip)
		}
	}
}

func TestVirtualInterfaceNames(t *testing.T) {
	for _, name := range []string{"docker0", "docker_gwbridge", "br-9f8e7d6c5b4a", "br-ex", "veth1a2b3c4", "vnet0", "virbr0", "lxcbr0", "lxdbr0", "flannel.1", "cni0", "cni-podman0", "kube-ipvs0", "ovs-system", "tun0", "tap0", "tunl0", "wg0", "tailscale0", "zt3f2a1b", "dummy0", "ifb0", "sit0", "gre0", "gretap0", "ip6tnl0", "ipip0", "erspan0"} {
		if !virtualInterfaceName(name) {
			t.Errorf("virtual interface not recognised: %s", name)
		}
	}
	for _, name := range []string{"eth0", "eno1", "enp3s0", "wlan0", "br0", "vmbr0", "bond0", "team0", "eth0.100", "ppp0", "usb0", "wwan0"} {
		if virtualInterfaceName(name) {
			t.Errorf("real NIC misclassified as virtual: %s", name)
		}
	}
}

func TestDirectSSHPeerNeverIncludesExplicitProxy(t *testing.T) {
	peer := &net.TCPAddr{IP: net.ParseIP("8.8.4.4"), Port: 1080}
	for _, proxy := range []string{"socks5", "http"} {
		if got := directSSHServerPeer(peer, proxy); got != "" {
			t.Errorf("%s proxy leaked as server: %s", proxy, got)
		}
	}
	if got := directSSHServerPeer(peer, "direct"); got != "8.8.4.4" {
		t.Fatal(got)
	}
	if got := directSSHServerPeer(&net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}, "direct"); got != "192.168.1.1" {
		t.Errorf("LAN peer rejected: %q", got)
	}
	for _, value := range []string{"127.0.0.1", "198.18.0.1", "fe80::1"} {
		if got := directSSHServerPeer(&net.TCPAddr{IP: net.ParseIP(value), Port: 22}, "direct"); got != "" {
			t.Errorf("local/TUN peer accepted: %s", got)
		}
	}
}

func publicNetworkFixture() map[string][]string {
	return map[string][]string{
		"__CS_NET__":  {"lo: 0 0 0 0 0 0 0 0 0", "wan: 1 0 0 0 0 0 0 0 1", "other: 2 0 0 0 0 0 0 0 2", "down: 3 0 0 0 0 0 0 0 3"},
		"__CS_LINK__": {"1: lo: <LOOPBACK,UP> mtu 65536 state UNKNOWN", "2: wan: <UP,LOWER_UP> mtu 1500 state UP", "3: other: <UP,LOWER_UP> mtu 1500 state UP", "4: down: <BROADCAST> mtu 1500 state DOWN"},
		"__CS_ADDR__": {
			"2: wan inet 8.8.8.8/24 scope global wan", "2: wan inet 1.1.1.1/24 scope global secondary wan", "2: wan inet 192.168.1.2/24 scope global wan", "2: wan inet6 fe80::1/64 scope link", "2: wan inet6 2606:4700:4700::1111/64 scope global", "2: wan inet6 2001:4860:4860::8888/64 scope global", "2: wan inet6 2606:4700::1001/64 scope global tentative", "2: wan inet6 2606:4700::1002/64 scope global dadfailed", "2: wan inet6 2606:4700::1003/64 scope global deprecated", "2: wan inet 8.8.8.7/24 scope link", "3: other inet 9.9.9.9/24 scope global other", "4: down inet 4.2.2.1/24 scope global down", "1: lo inet 127.0.0.1/8 scope host lo"},
		"__CS_ROUTES__":     {"default via 1.1.1.254 dev wan"},
		"__CS_CONNECTION__": {"192.168.10.1 45000 10.0.0.2 22"},
	}
}

func TestServerAddressSelectionSourcesAndStableInterfaceOrder(t *testing.T) {
	sections := publicNetworkFixture()
	var first rawStats
	parseNetworkStats(&first, sections)
	if !reflect.DeepEqual(first.ServerIPv4, []string{"10.0.0.2", "1.1.1.1", "8.8.8.8", "192.168.1.2", "9.9.9.9"}) {
		t.Fatalf("wrong NIC IPv4/order: %v", first.ServerIPv4)
	}
	if !reflect.DeepEqual(first.ServerIPv6, []string{"2001:4860:4860::8888", "2606:4700:4700::1111"}) {
		t.Fatalf("wrong public IPv6/order: %v", first.ServerIPv6)
	}
	if got := strings.Join(first.Interfaces[0].Addresses, ","); !strings.Contains(got, "192.168.1.2/24") || !strings.Contains(got, "fe80::1/64") {
		t.Fatalf("NIC tooltip lost non-public addresses: %s", got)
	}
	if first.ServerAddressInfo[0].Source != "ssh-server" || first.ServerAddressInfo[0].Address != "10.0.0.2" {
		t.Fatalf("SSH server socket address should lead the provenance list: %+v", first.ServerAddressInfo)
	}
	if first.ServerAddressInfo[1].Source != "interface" || first.ServerAddressInfo[1].Interface != "wan" {
		t.Fatalf("incorrect provenance: %+v", first.ServerAddressInfo)
	}
	// ip's enumeration order can change as DHCP/IPv6 addresses are renewed.
	for i, j := 0, len(sections["__CS_ADDR__"])-1; i < j; i, j = i+1, j-1 {
		sections["__CS_ADDR__"][i], sections["__CS_ADDR__"][j] = sections["__CS_ADDR__"][j], sections["__CS_ADDR__"][i]
	}
	var second rawStats
	parseNetworkStats(&second, sections)
	if !reflect.DeepEqual(first.ServerAddressInfo, second.ServerAddressInfo) {
		t.Fatalf("address order depends on collection order: %v != %v", first.ServerAddressInfo, second.ServerAddressInfo)
	}
}

func TestServerAddressProvenanceAndOrder(t *testing.T) {
	cases := []struct {
		name, host, peer, remote string
		want                     []string
		source                   string
	}{
		{"NAT numeric target", "8.8.4.4", "", "10.0.0.2", []string{"10.0.0.2", "8.8.4.4"}, "ssh-server"},
		{"NAT direct hostname actual peer", "server.example", "1.0.0.1", "10.0.0.2", []string{"10.0.0.2", "1.0.0.1"}, "ssh-server"},
		{"proxy hostname cannot infer IP", "server.example", "", "10.0.0.2", []string{"10.0.0.2"}, "ssh-server"},
		{"private LAN target", "192.168.1.2", "", "10.0.0.2", []string{"10.0.0.2", "192.168.1.2"}, "ssh-server"},
		{"TUN fake target excluded", "198.18.0.1", "", "10.0.0.2", []string{"10.0.0.2"}, "ssh-server"},
		{"remote socket wins", "8.8.4.4", "8.8.4.4", "1.1.1.1", []string{"1.1.1.1", "8.8.4.4"}, "ssh-server"},
		{"numeric target before actual peer", "8.8.4.4", "1.0.0.1", "10.0.0.2", []string{"10.0.0.2", "8.8.4.4", "1.0.0.1"}, "ssh-server"},
		{"mapped IPv4 duplicate", "8.8.4.4", "8.8.4.4", "::ffff:8.8.4.4", []string{"8.8.4.4"}, "ssh-server"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := Session{connectionHost: tc.host, connectionPeer: tc.peer}
			stats := Stats{SSHConnection: SSHConnectionInfo{ServerAddress: tc.remote}}
			s.applyServerAddresses(&stats)
			if !reflect.DeepEqual(stats.ServerIPv4, tc.want) {
				t.Fatalf("got %v want %v", stats.ServerIPv4, tc.want)
			}
			if len(tc.want) > 0 && stats.ServerAddressInfo[0].Source != tc.source {
				t.Fatalf("wrong source: %+v", stats.ServerAddressInfo)
			}
		})
	}
}

func TestServerAddressSkipsVirtualInterfaces(t *testing.T) {
	var parsed rawStats
	parseNetworkStats(&parsed, map[string][]string{
		"__CS_NET__": {"eth0: 1 0 0 0 0 0 0 0 1", "docker0: 2 0 0 0 0 0 0 0 2", "br-9f8e7d: 3 0 0 0 0 0 0 0 3", "veth1a2b: 4 0 0 0 0 0 0 0 4", "virbr0: 5 0 0 0 0 0 0 0 5", "flannel.1: 6 0 0 0 0 0 0 0 6", "cni0: 7 0 0 0 0 0 0 0 7", "wg0: 8 0 0 0 0 0 0 0 8", "kube-ipvs0: 9 0 0 0 0 0 0 0 9", "tun0: 10 0 0 0 0 0 0 0 10", "bond0: 11 0 0 0 0 0 0 0 11", "eth0.100: 12 0 0 0 0 0 0 0 12", "vmbr0: 13 0 0 0 0 0 0 0 13"},
		"__CS_LINK__": {"1: eth0: <BROADCAST,UP,LOWER_UP> mtu 1500 state UP", "2: docker0: <BROADCAST,UP,LOWER_UP> mtu 1500 state UP", "3: br-9f8e7d: <BROADCAST,UP,LOWER_UP> mtu 1500 state UP", "4: veth1a2b: <BROADCAST,UP,LOWER_UP> mtu 1500 state UP", "5: virbr0: <BROADCAST,UP,LOWER_UP> mtu 1500 state UP", "6: flannel.1: <BROADCAST,UP,LOWER_UP> mtu 1450 state UNKNOWN", "7: cni0: <BROADCAST,UP,LOWER_UP> mtu 1500 state UP", "8: wg0: <POINTOPOINT,NOARP,UP> mtu 1420 state UNKNOWN", "9: kube-ipvs0: <BROADCAST,UP,LOWER_UP> mtu 1500 state DOWN", "10: tun0: <POINTOPOINT,MULTICAST,NOARP,UP> mtu 1500 state UNKNOWN", "11: bond0: <BROADCAST,UP,LOWER_UP> mtu 1500 state UP", "12: eth0.100: <BROADCAST,UP,LOWER_UP> mtu 1500 state UP", "13: vmbr0: <BROADCAST,UP,LOWER_UP> mtu 1500 state UP"},
		"__CS_ADDR__": {"1: eth0 inet 192.168.8.52/24 scope global eth0", "2: docker0 inet 172.17.0.1/16 scope global docker0", "3: br-9f8e7d inet 172.18.0.1/16 scope global br-9f8e7d", "4: veth1a2b inet 169.254.1.1/16 scope global veth1a2b", "5: virbr0 inet 192.168.122.1/24 scope global virbr0", "6: flannel.1 inet 10.244.0.0/32 scope global flannel.1", "7: cni0 inet 10.85.0.1/16 scope global cni0", "8: wg0 inet 10.8.0.5/24 scope global wg0", "9: kube-ipvs0 inet 10.96.0.1/12 scope global kube-ipvs0", "10: tun0 inet 10.9.0.1/24 scope global tun0", "11: bond0 inet 10.0.0.1/24 scope global bond0", "12: eth0.100 inet 192.168.9.5/24 scope global eth0.100", "13: vmbr0 inet 10.10.10.10/24 scope global vmbr0"},
		"__CS_ROUTES__": {"default via 192.168.8.1 dev eth0"},
	})
	if !reflect.DeepEqual(parsed.ServerIPv4, []string{"192.168.8.52", "10.0.0.1", "192.168.9.5", "10.10.10.10"}) {
		t.Fatalf("virtual interface addresses leaked: %v", parsed.ServerIPv4)
	}
	for _, info := range parsed.ServerAddressInfo {
		if virtualInterfaceName(info.Interface) {
			t.Fatalf("virtual interface provenance leaked: %+v", info)
		}
	}
}

func TestServerAddressMetadataCopiesDoNotAlias(t *testing.T) {
	var parsed rawStats
	parseNetworkStats(&parsed, publicNetworkFixture())
	original := parsed.ServerAddressInfo[0]
	shown := displayStats(parsed.Stats, false, false, LatencySample{})
	shown.ServerAddressInfo[0].Address = "mutated"
	if parsed.ServerAddressInfo[0] != original {
		t.Fatal("display aliases cached address provenance")
	}
	var current rawStats
	applyStaticMetadata(&current, &parsed.Stats)
	current.ServerAddressInfo[0].Source = "mutated"
	if parsed.ServerAddressInfo[0] != original {
		t.Fatal("static metadata aliases address provenance")
	}
}

func TestServerAddressPointToPointLocalAddress(t *testing.T) {
	var parsed rawStats
	parseNetworkStats(&parsed, map[string][]string{
		"__CS_NET__":  {"ppp0: 1 0 0 0 0 0 0 0 1"},
		"__CS_LINK__": {"5: ppp0: <POINTOPOINT,UP,LOWER_UP> mtu 1492 state UNKNOWN"},
		"__CS_ADDR__": {"5: ppp0 inet 8.8.4.4 peer 1.0.0.1/32 scope global ppp0"},
	})
	if !reflect.DeepEqual(parsed.ServerIPv4, []string{"8.8.4.4"}) || !reflect.DeepEqual(parsed.Interfaces[0].Addresses, []string{"8.8.4.4/32"}) {
		t.Fatalf("point-to-point local address was lost or confused with peer: %+v", parsed.Stats)
	}
}
