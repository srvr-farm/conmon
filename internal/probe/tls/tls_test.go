package tlsprobe

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/mcallan/conmon/internal/config"
)

func TestProbeRunReportsWholeDaysRemaining(t *testing.T) {
	now := time.Date(2026, time.April, 7, 18, 0, 0, 0, time.UTC)
	notAfter := now.Add(10*24*time.Hour + 6*time.Hour)

	addr, rootCAs, shutdown := startTLSServer(t, notAfter)
	defer shutdown()

	host, port := splitHostPort(t, addr)
	check := config.Check{
		ID:               "cert",
		Name:             "Example Certificate",
		Group:            "external-services",
		Kind:             "tls",
		Scope:            "external",
		Host:             host,
		Port:             port,
		MinDaysRemaining: 7,
		Timeout:          config.Duration{Duration: time.Second},
	}

	probe := New(check, Options{
		RootCAs: rootCAs,
		Now: func() time.Time {
			return now
		},
	})
	result := probe.Run(context.Background())

	if !result.Success {
		t.Fatal("expected success")
	}
	if got, want := result.TLSCertDaysRemaining, 10; got != want {
		t.Fatalf("TLSCertDaysRemaining = %d, want %d", got, want)
	}
}

func TestProbeRunFailsWithoutTrustedRoot(t *testing.T) {
	now := time.Date(2026, time.April, 7, 18, 0, 0, 0, time.UTC)
	notAfter := now.Add(10 * 24 * time.Hour)

	addr, _, shutdown := startTLSServer(t, notAfter)
	defer shutdown()

	host, port := splitHostPort(t, addr)
	check := config.Check{
		ID:               "cert",
		Name:             "Example Certificate",
		Group:            "external-services",
		Kind:             "tls",
		Scope:            "external",
		Host:             host,
		Port:             port,
		MinDaysRemaining: 7,
		Timeout:          config.Duration{Duration: time.Second},
	}

	probe := New(check, Options{
		Now: func() time.Time {
			return now
		},
	})
	result := probe.Run(context.Background())

	if result.Success {
		t.Fatal("expected verification failure without trusted root")
	}
	if got := result.TLSCertDaysRemaining; got != 0 {
		t.Fatalf("TLSCertDaysRemaining = %d, want 0 when handshake fails", got)
	}
}

func TestProbeRunHonorsTimeoutDuringHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen returned error: %v", err)
	}
	defer listener.Close()

	accepted := make(chan struct{})
	release := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			close(accepted)
			<-release
			_ = conn.Close()
		}
	}()
	defer close(release)

	host, port := splitHostPort(t, listener.Addr().String())
	check := config.Check{
		ID:      "cert",
		Name:    "Example Certificate",
		Group:   "external-services",
		Kind:    "tls",
		Scope:   "external",
		Host:    host,
		Port:    port,
		Timeout: config.Duration{Duration: 100 * time.Millisecond},
	}

	start := time.Now()
	result := New(check, Options{}).Run(context.Background())
	elapsed := time.Since(start)

	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("server did not accept connection")
	}

	if result.Success {
		t.Fatal("expected timeout failure")
	}
	if elapsed > time.Second {
		t.Fatalf("probe took %v, want handshake timeout", elapsed)
	}
}

func TestProbeRunFailsWhenDaysRemainingIsBelowThreshold(t *testing.T) {
	now := time.Date(2026, time.April, 7, 18, 0, 0, 0, time.UTC)
	notAfter := now.Add(3*24*time.Hour + time.Hour)

	addr, rootCAs, shutdown := startTLSServer(t, notAfter)
	defer shutdown()

	host, port := splitHostPort(t, addr)
	check := config.Check{
		ID:               "cert",
		Name:             "Example Certificate",
		Group:            "external-services",
		Kind:             "tls",
		Scope:            "external",
		Host:             host,
		Port:             port,
		MinDaysRemaining: 4,
		Timeout:          config.Duration{Duration: time.Second},
	}

	probe := New(check, Options{
		RootCAs: rootCAs,
		Now: func() time.Time {
			return now
		},
	})
	result := probe.Run(context.Background())

	if result.Success {
		t.Fatal("expected failure")
	}
	if got, want := result.TLSCertDaysRemaining, 3; got != want {
		t.Fatalf("TLSCertDaysRemaining = %d, want %d", got, want)
	}
}

func TestProbeRunFailsWhenCertificateIsExpired(t *testing.T) {
	now := time.Date(2026, time.April, 7, 18, 0, 0, 0, time.UTC)
	notAfter := now.Add(-2 * time.Hour)

	addr, rootCAs, shutdown := startTLSServer(t, notAfter)
	defer shutdown()

	host, port := splitHostPort(t, addr)
	check := config.Check{
		ID:      "cert",
		Name:    "Example Certificate",
		Group:   "external-services",
		Kind:    "tls",
		Scope:   "external",
		Host:    host,
		Port:    port,
		Timeout: config.Duration{Duration: time.Second},
	}

	probe := New(check, Options{
		RootCAs: rootCAs,
		Now: func() time.Time {
			return now
		},
	})
	result := probe.Run(context.Background())

	if result.Success {
		t.Fatal("expected failure")
	}
	if got, want := result.TLSCertDaysRemaining, 0; got != want {
		t.Fatalf("TLSCertDaysRemaining = %d, want %d when verification fails", got, want)
	}
}

func startTLSServer(t *testing.T, notAfter time.Time) (string, *x509.CertPool, func()) {
	t.Helper()

	certificate := makeCertificate(t, notAfter)
	tlsCert, err := tls.X509KeyPair(certificate.CertPEM, certificate.KeyPEM)
	if err != nil {
		t.Fatalf("tls.X509KeyPair returned error: %v", err)
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	})
	if err != nil {
		t.Fatalf("tls.Listen returned error: %v", err)
	}

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		conn, err := listener.Accept()
		if err == nil {
			tlsConn := conn.(*tls.Conn)
			_ = tlsConn.Handshake()
			_ = tlsConn.Close()
		}
	}()

	rootCAs := x509.NewCertPool()
	rootCAs.AddCert(certificate.Root)

	shutdown := func() {
		_ = listener.Close()
		<-stopped
	}

	return listener.Addr().String(), rootCAs, shutdown
}

type generatedCertificate struct {
	CertPEM []byte
	KeyPEM  []byte
	Root    *x509.Certificate
}

func makeCertificate(t *testing.T, notAfter time.Time) generatedCertificate {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey returned error: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "conmon test root",
		},
		NotBefore:             notAfter.Add(-365 * 24 * time.Hour),
		NotAfter:              notAfter.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("x509.CreateCertificate returned error: %v", err)
	}

	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("x509.ParseCertificate returned error: %v", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey returned error: %v", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "127.0.0.1",
		},
		NotBefore:             notAfter.Add(-30 * 24 * time.Hour),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("x509.CreateCertificate returned error: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})

	return generatedCertificate{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
		Root:    caCert,
	}
}

func splitHostPort(t *testing.T, address string) (string, int) {
	t.Helper()

	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("net.SplitHostPort returned error: %v", err)
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil {
		t.Fatalf("net.LookupPort returned error: %v", err)
	}
	return host, port
}
