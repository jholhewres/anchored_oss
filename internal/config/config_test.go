package config

import "testing"

func TestTrustedNets(t *testing.T) {
	cfg := DefaultConfig().RateLimit
	if nets, err := cfg.TrustedNets(); err != nil || len(nets) != 0 {
		t.Fatalf("default trusted proxies = %v, %v; want none", nets, err)
	}
	cfg.TrustedProxies = []string{"10.1.2.3", "fd00::/8", " ", "::1"}
	nets, err := cfg.TrustedNets()
	if err != nil || len(nets) != 3 {
		t.Fatalf("valid list = %v, %v; want 3 networks", nets, err)
	}
	if ones, bits := nets[0].Mask.Size(); ones != 32 || bits != 32 {
		t.Fatalf("single IPv4 = /%d of %d bits, want /32", ones, bits)
	}
	for _, bad := range []string{"10.0.0.0/33", "caddy", "10.0.0"} {
		cfg.TrustedProxies = []string{bad}
		if _, err := cfg.TrustedNets(); err == nil {
			t.Errorf("TrustedNets(%q) = nil error, want one", bad)
		}
	}
}
