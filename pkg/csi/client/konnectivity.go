/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	log "k8s.io/klog/v2"
	"sigs.k8s.io/apiserver-network-proxy/konnectivity-client/pkg/client"
)

type GrpcProxyClientOptions struct {
	Mode         string
	ClientCert   string
	ClientKey    string
	CACert       string
	ProxyUDSName string
	ProxyHost    string
	ProxyPort    int
}

type proxyFunc func(ctx context.Context, addr string) (net.Conn, error)

func getKonnectivityUDSDialer(ctx context.Context, address string, timeout time.Duration, o GrpcProxyClientOptions) (func(ctx context.Context, addr string) (net.Conn, error), error) {
	log.Infof("using konnectivity UDS dialer")

	var proxyConn net.Conn
	var userAgent = "csi-open-local"
	var err error

	switch o.Mode {
	case "grpc":
		dialOption := grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			c, err := net.DialTimeout("unix", o.ProxyUDSName, timeout)
			if err != nil {
				log.ErrorS(err, "failed to create connection to uds", "name", o.ProxyUDSName)
			}
			return c, err
		})
		tunnel, err := client.CreateSingleUseGrpcTunnelWithContext(
			context.TODO(),
			ctx,
			o.ProxyUDSName,
			dialOption,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithUserAgent(userAgent),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create tunnel %s, got %v", o.ProxyUDSName, err)
		}

		proxyConn, err = tunnel.DialContext(ctx, "tcp", address)
		if err != nil {
			return nil, fmt.Errorf("failed to dial request %s, got %v", address, err)
		}
	case "http-connect":
		proxyConn, err = net.Dial("unix", o.ProxyUDSName)
		if err != nil {
			return nil, fmt.Errorf("dialing proxy %q failed: %v", o.ProxyUDSName, err)
		}
		fmt.Fprintf(proxyConn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\n\r\n", address, "127.0.0.1", userAgent)
		br := bufio.NewReader(proxyConn)
		res, err := http.ReadResponse(br, nil)
		if err != nil {
			return nil, fmt.Errorf("reading HTTP response from CONNECT to %s via uds proxy %s failed: %v",
				address, o.ProxyUDSName, err)
		}
		if res.StatusCode != 200 {
			return nil, fmt.Errorf("proxy error from %s while dialing %s: %v", o.ProxyUDSName, address, res.Status)
		}

		// It's safe to discard the bufio.Reader here and return the
		// original TCP conn directly because we only use this for
		// TLS, and in TLS the client speaks first, so we know there's
		// no unbuffered data. But we can double-check.
		if br.Buffered() > 0 {
			return nil, fmt.Errorf("unexpected %d bytes of buffered data from CONNECT uds proxy %q",
				br.Buffered(), o.ProxyUDSName)
		}
	default:
		return nil, fmt.Errorf("failed to process mode %s", o.Mode)
	}

	return func(ctx context.Context, addr string) (net.Conn, error) {
		return proxyConn, nil
	}, nil
}

func getKonnectivityMTLSDialer(ctx context.Context, address string, _ time.Duration, o GrpcProxyClientOptions) (func(ctx context.Context, addr string) (net.Conn, error), error) {
	log.Infof("using konnectivity mTLS dialer")

	tlsConfig, err := getClientTLSConfig(o.CACert, o.ClientCert, o.ClientKey, o.ProxyHost, nil)
	if err != nil {
		return nil, err
	}

	var proxyConn net.Conn
	switch o.Mode {
	case "grpc":
		transportCreds := credentials.NewTLS(tlsConfig)
		dialOption := grpc.WithTransportCredentials(transportCreds)
		serverAddress := fmt.Sprintf("%s:%d", o.ProxyHost, o.ProxyPort)
		tunnel, err := client.CreateSingleUseGrpcTunnelWithContext(context.TODO(), ctx, serverAddress, dialOption)
		if err != nil {
			return nil, fmt.Errorf("failed to create tunnel %s, got %v", serverAddress, err)
		}

		proxyConn, err = tunnel.DialContext(ctx, "tcp", address)
		if err != nil {
			return nil, fmt.Errorf("failed to dial request %s, got %v", address, err)
		}
	case "http-connect":
		proxyAddress := fmt.Sprintf("%s:%d", o.ProxyHost, o.ProxyPort)
		proxyConn, err = tls.Dial("tcp", proxyAddress, tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("dialing proxy %q failed: %v", proxyAddress, err)
		}
		fmt.Fprintf(proxyConn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", address, "127.0.0.1")
		br := bufio.NewReader(proxyConn)
		res, err := http.ReadResponse(br, nil)
		if err != nil {
			return nil, fmt.Errorf("reading HTTP response from CONNECT to %s via proxy %s failed: %v",
				address, proxyAddress, err)
		}
		if res.StatusCode != 200 {
			return nil, fmt.Errorf("proxy error from %s while dialing %s: %v", proxyAddress, address, res.Status)
		}

		// It's safe to discard the bufio.Reader here and return the
		// original TCP conn directly because we only use this for
		// TLS, and in TLS the client speaks first, so we know there's
		// no unbuffered data. But we can double-check.
		if br.Buffered() > 0 {
			return nil, fmt.Errorf("unexpected %d bytes of buffered data from CONNECT proxy %q",
				br.Buffered(), proxyAddress)
		}
	default:
		return nil, fmt.Errorf("failed to process mode %s", o.Mode)
	}

	return func(ctx context.Context, addr string) (net.Conn, error) {
		return proxyConn, nil
	}, nil
}

// getCACertPool loads CA certificates to pool
func getCACertPool(caFile string) (*x509.CertPool, error) {
	certPool := x509.NewCertPool()
	caCert, err := os.ReadFile(filepath.Clean(caFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert %s: %v", caFile, err)
	}
	ok := certPool.AppendCertsFromPEM(caCert)
	if !ok {
		return nil, fmt.Errorf("failed to append CA cert to the cert pool")
	}
	return certPool, nil
}

// getClientTLSConfig returns tlsConfig based on x509 certs
func getClientTLSConfig(caFile, certFile, keyFile, serverName string, protos []string) (*tls.Config, error) {
	certPool, err := getCACertPool(caFile)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		RootCAs:    certPool,
		MinVersion: tls.VersionTLS12,
	}
	if len(protos) != 0 {
		tlsConfig.NextProtos = protos
	}
	if certFile == "" && keyFile == "" {
		// return TLS config based on CA only
		return tlsConfig, nil
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load X509 key pair %s and %s: %v", certFile, keyFile, err)
	}

	tlsConfig.ServerName = serverName
	tlsConfig.Certificates = []tls.Certificate{cert}
	return tlsConfig, nil
}
