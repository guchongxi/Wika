package safefetch

import (
	"context"
	"net"
	"strings"
	"testing"
)

type fakeResolver struct {
	ips map[string][]net.IPAddr
}

func (r fakeResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return r.ips[strings.ToLower(host)], nil
}

func TestValidatorRejectsUnsafeSourceURLs(t *testing.T) {
	validator := NewValidator(fakeResolver{ips: map[string][]net.IPAddr{
		"private.example.com":      {{IP: net.ParseIP("10.0.0.7")}},
		"metadata.google.internal": {{IP: net.ParseIP("169.254.169.254")}},
		"safe.example.com":         {{IP: net.ParseIP("93.184.216.34")}},
		"0x7f000001":               {{IP: net.ParseIP("93.184.216.34")}},
	}})

	tests := []struct {
		name string
		raw  string
	}{
		{name: "ftp scheme", raw: "ftp://safe.example.com/file.txt"},
		{name: "userinfo", raw: "https://user:pass@safe.example.com/page"},
		{name: "loopback ipv4", raw: "http://127.0.0.1/admin"},
		{name: "loopback ipv6", raw: "http://[::1]/admin"},
		{name: "metadata ip", raw: "http://169.254.169.254/latest/meta-data"},
		{name: "decimal ip", raw: "http://2130706433/"},
		{name: "octal ip", raw: "http://0177.0.0.1/"},
		{name: "hex ip", raw: "http://0x7f000001/"},
		{name: "carrier grade nat ip", raw: "http://100.64.0.1/"},
		{name: "benchmark reserved ip", raw: "http://198.18.0.1/"},
		{name: "documentation reserved ip", raw: "http://192.0.2.1/"},
		{name: "metadata hostname", raw: "http://metadata.google.internal/computeMetadata/v1"},
		{name: "aws metadata hostname", raw: "http://metadata.aws.internal/latest/meta-data"},
		{name: "tencent metadata hostname", raw: "http://metadata.tencentyun.com/latest/meta-data"},
		{name: "docker host", raw: "http://host.docker.internal/"},
		{name: "kubernetes service", raw: "http://api.default.svc.cluster.local/"},
		{name: "resolved private ip", raw: "https://private.example.com/a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := validator.ValidateURL(context.Background(), tt.raw); err == nil {
				t.Fatalf("expected %q to be rejected", tt.raw)
			}
		})
	}
}

func TestValidatorNormalizesSafeURL(t *testing.T) {
	validator := NewValidator(fakeResolver{ips: map[string][]net.IPAddr{
		"safe.example.com": {{IP: net.ParseIP("93.184.216.34")}},
	}})

	got, err := validator.ValidateURL(context.Background(), "HTTPS://Safe.Example.Com./docs?q=1")
	if err != nil {
		t.Fatalf("ValidateURL returned error: %v", err)
	}
	if got.String() != "https://safe.example.com/docs?q=1" {
		t.Fatalf("unexpected normalized URL: %s", got.String())
	}
}

func TestValidatorNormalizesIDNAHost(t *testing.T) {
	validator := NewValidator(fakeResolver{ips: map[string][]net.IPAddr{
		"xn--bcher-kva.example": {{IP: net.ParseIP("93.184.216.34")}},
	}})

	got, err := validator.ValidateURL(context.Background(), "https://Bücher.Example/docs")
	if err != nil {
		t.Fatalf("ValidateURL returned error: %v", err)
	}
	if got.String() != "https://xn--bcher-kva.example/docs" {
		t.Fatalf("unexpected normalized URL: %s", got.String())
	}
}

func TestValidatorAllowsPublicIPv4Literal(t *testing.T) {
	validator := NewValidator(fakeResolver{})

	got, err := validator.ValidateURL(context.Background(), "https://93.184.216.34/docs")
	if err != nil {
		t.Fatalf("ValidateURL returned error: %v", err)
	}
	if got.String() != "https://93.184.216.34/docs" {
		t.Fatalf("unexpected normalized URL: %s", got.String())
	}
}
