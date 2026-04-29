package jshttp

import "testing"

func TestStartAuthProxyAllowsNoAuthHTTPProxy(t *testing.T) {
	proxy, err := StartAuthProxy("http://127.0.0.1:18080", "", "")
	if err != nil {
		t.Fatalf("StartAuthProxy() error = %v", err)
	}
	defer proxy.Close()

	if proxy.Port() == 0 {
		t.Fatal("expected local proxy port to be assigned")
	}

	if proxy.auth != "" {
		t.Fatal("expected no upstream auth header for no-auth proxy")
	}
}

func TestStartAuthProxyAllowsNoAuthSOCKS5Proxy(t *testing.T) {
	proxy, err := StartAuthProxy("socks5://127.0.0.1:1080", "", "")
	if err != nil {
		t.Fatalf("StartAuthProxy() error = %v", err)
	}
	defer proxy.Close()

	if proxy.socks5Auth != nil {
		t.Fatal("expected nil SOCKS5 auth for no-auth proxy")
	}
}

func TestStartAuthProxyAddsDefaultProxyPort(t *testing.T) {
	proxy, err := StartAuthProxy("https://proxy.example.com", "", "")
	if err != nil {
		t.Fatalf("StartAuthProxy() error = %v", err)
	}
	defer proxy.Close()

	if got, want := proxy.upstream.Host, "proxy.example.com:443"; got != want {
		t.Fatalf("expected upstream host %q, got %q", want, got)
	}
}

func TestStartAuthProxyKeepsAuthWhenProvided(t *testing.T) {
	proxy, err := StartAuthProxy("http://127.0.0.1:18080", "user", "pass")
	if err != nil {
		t.Fatalf("StartAuthProxy() error = %v", err)
	}
	defer proxy.Close()

	if proxy.auth == "" {
		t.Fatal("expected upstream auth to be configured")
	}
}
