package httpx

import (
	"net/netip"
	"testing"
)

func TestBlockedIPTable(t *testing.T) {
	defaultPolicy := Default()
	defaultPolicy.AllowLoopback = false
	defaultPolicy.AllowPrivate = false

	privateAllowedPolicy := Policy{
		AllowPrivate:  true,
		AllowLoopback: false,
	}

	loopbackAllowedPolicy := Policy{
		AllowPrivate:  false,
		AllowLoopback: true,
	}

	tests := []struct {
		addr        string
		policy      Policy
		wantBlocked bool
		wantReason  string
	}{
		// Unspecified
		{"0.0.0.0", defaultPolicy, true, "unspecified address"},
		{"::", defaultPolicy, true, "unspecified address"},

		// Loopback
		{"127.0.0.1", defaultPolicy, true, "loopback address"},
		{"127.0.0.2", defaultPolicy, true, "loopback address"},
		{"127.255.255.255", defaultPolicy, true, "loopback address"},
		{"::1", defaultPolicy, true, "loopback address"},
		{"127.0.0.1", loopbackAllowedPolicy, false, ""},
		{"::1", loopbackAllowedPolicy, false, ""},

		// Link-local / metadata (never allowed even with AllowPrivate or AllowLoopback)
		{"169.254.0.1", defaultPolicy, true, "link-local address"},
		{"169.254.169.254", defaultPolicy, true, "link-local address"},
		{"169.254.169.254", privateAllowedPolicy, true, "link-local address"},
		{"169.254.169.254", loopbackAllowedPolicy, true, "link-local address"},
		{"169.254.255.255", defaultPolicy, true, "link-local address"},
		{"fe80::1", defaultPolicy, true, "link-local address"},
		{"fe80::ffff", defaultPolicy, true, "link-local address"},
		{"fe80::1", privateAllowedPolicy, true, "link-local address"},

		// Private RFC1918 / ULA
		{"10.0.0.1", defaultPolicy, true, "private network address"},
		{"10.255.255.254", defaultPolicy, true, "private network address"},
		{"172.16.0.1", defaultPolicy, true, "private network address"},
		{"172.31.255.254", defaultPolicy, true, "private network address"},
		{"192.168.0.1", defaultPolicy, true, "private network address"},
		{"192.168.255.254", defaultPolicy, true, "private network address"},
		{"fc00::1", defaultPolicy, true, "private network address"},
		{"fd12:3456:789a::1", defaultPolicy, true, "private network address"},
		{"10.0.0.1", privateAllowedPolicy, false, ""},
		{"192.168.1.1", privateAllowedPolicy, false, ""},
		{"fd12:3456:789a::1", privateAllowedPolicy, false, ""},

		// CGNAT 100.64.0.0/10
		{"100.64.0.1", defaultPolicy, true, "CGNAT address"},
		{"100.127.255.254", defaultPolicy, true, "CGNAT address"},
		{"100.64.0.1", privateAllowedPolicy, false, ""},

		// Benchmarking 198.18.0.0/15
		{"198.18.0.1", defaultPolicy, true, "benchmarking address"},
		{"198.19.255.254", defaultPolicy, true, "benchmarking address"},
		{"198.18.0.1", privateAllowedPolicy, false, ""},

		// Documentation ranges
		{"192.0.2.1", defaultPolicy, true, "documentation address"},
		{"198.51.100.1", defaultPolicy, true, "documentation address"},
		{"203.0.113.1", defaultPolicy, true, "documentation address"},
		{"2001:db8::1", defaultPolicy, true, "documentation address"},
		{"2001:db8:ffff::1", defaultPolicy, true, "documentation address"},
		{"192.0.2.1", privateAllowedPolicy, false, ""},

		// Multicast
		{"224.0.0.1", defaultPolicy, true, "multicast address"},
		{"239.255.255.250", defaultPolicy, true, "multicast address"},
		{"ff02::1", defaultPolicy, true, "multicast address"},
		{"ff05::2", defaultPolicy, true, "multicast address"},

		// IPv4-mapped IPv6
		{"::ffff:127.0.0.1", defaultPolicy, true, "loopback address"},
		{"::ffff:10.0.0.1", defaultPolicy, true, "private network address"},
		{"::ffff:169.254.169.254", defaultPolicy, true, "link-local address"},

		// NAT64 64:ff9b::/96
		{"64:ff9b::127.0.0.1", defaultPolicy, true, "loopback address"},
		{"64:ff9b::169.254.169.254", defaultPolicy, true, "link-local address"},
		{"64:ff9b::10.0.0.1", defaultPolicy, true, "private network address"},

		// 6to4 2002::/16
		{"2002:7f00:0001::", defaultPolicy, true, "loopback address"},
		{"2002:a9fe:a9fe::", defaultPolicy, true, "link-local address"},
		{"2002:0a00:0001::", defaultPolicy, true, "private network address"},

		// Public IP addresses (allowed)
		{"8.8.8.8", defaultPolicy, false, ""},
		{"1.1.1.1", defaultPolicy, false, ""},
		{"93.184.216.34", defaultPolicy, false, ""},
		{"2606:4700:4700::1111", defaultPolicy, false, ""},
	}

	if len(tests) < 40 {
		t.Fatalf("expected >= 40 tests, got %d", len(tests))
	}

	for _, tc := range tests {
		t.Run(tc.addr, func(t *testing.T) {
			addr, err := netip.ParseAddr(tc.addr)
			if err != nil {
				t.Fatalf("invalid test IP: %s: %v", tc.addr, err)
			}
			reason := BlockedIP(addr, tc.policy)
			if tc.wantBlocked {
				if reason == "" {
					t.Errorf("expected %s to be blocked, got allowed", tc.addr)
				} else if tc.wantReason != "" && reason != tc.wantReason {
					t.Errorf("expected reason %q, got %q", tc.wantReason, reason)
				}
			} else {
				if reason != "" {
					t.Errorf("expected %s to be allowed, got blocked (%s)", tc.addr, reason)
				}
			}
		})
	}
}
