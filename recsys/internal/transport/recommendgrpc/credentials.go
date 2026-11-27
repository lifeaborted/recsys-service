package recommendgrpc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"

	"recsys/internal/config"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func ClientCredentials(cfg config.GRPCConfig) (credentials.TransportCredentials, error) {
	tlsConfig, err := loadTLS(cfg, cfg.Target)
	if err != nil {
		return nil, err
	}
	if tlsConfig == nil {
		return insecure.NewCredentials(), nil
	}
	return credentials.NewTLS(tlsConfig), nil
}

func ServerCredentials(cfg config.GRPCConfig) (credentials.TransportCredentials, error) {
	tlsConfig, err := loadTLS(cfg, cfg.ListenAddr)
	if err != nil || tlsConfig == nil {
		return nil, err
	}
	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	tlsConfig.ClientCAs = tlsConfig.RootCAs
	tlsConfig.RootCAs = nil
	return credentials.NewTLS(tlsConfig), nil
}

func loadTLS(cfg config.GRPCConfig, address string) (*tls.Config, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("gRPC address must use host:port: %w", err)
	}
	if cfg.CertFile == "" && cfg.KeyFile == "" && cfg.CAFile == "" {
		ip, _ := netip.ParseAddr(host)
		if !ip.IsLoopback() && !strings.EqualFold(host, "localhost") {
			return nil, fmt.Errorf("gRPC address %q requires mutual TLS", address)
		}
		return nil, nil
	}
	if cfg.CertFile == "" || cfg.KeyFile == "" || cfg.CAFile == "" {
		return nil, fmt.Errorf("mutual TLS requires RECSYS_GRPC_CERT_FILE, RECSYS_GRPC_KEY_FILE and RECSYS_GRPC_CA_FILE")
	}
	certificate, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load gRPC certificate and key: %w", err)
	}
	pem, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read gRPC CA: %w", err)
	}
	ca := x509.NewCertPool()
	if !ca.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("gRPC CA file contains no PEM certificates")
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}, RootCAs: ca}, nil
}
