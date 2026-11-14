package tor

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// Policy controls how outbound peer connections select between direct TCP and Tor SOCKS5.
type Policy string

const (
	PolicyDirectFirst  Policy = "direct-first"
	PolicyTorPreferred Policy = "tor-preferred"
	PolicyTorOnly      Policy = "tor-only"
)

// Config holds Tor transport settings.
type Config struct {
	Policy       Policy `json:"policy"`
	SocksAddr    string `json:"socks_addr"`    // e.g. "127.0.0.1:9050"
	OnionService string `json:"onion_service"` // configured local .onion:port if published
}

// DialWithPolicy dials targetAddr (and optional onionAddr) strictly obeying cfg.Policy.
// In PolicyTorOnly mode, direct TCP connections are NEVER attempted.
func DialWithPolicy(cfg Config, directAddr, onionAddr string, timeout time.Duration) (net.Conn, string, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	policy := cfg.Policy
	if policy == "" {
		policy = PolicyDirectFirst
	}

	torTarget := onionAddr
	if torTarget == "" {
		torTarget = directAddr
	}

	switch policy {
	case PolicyTorOnly:
		if cfg.SocksAddr == "" {
			return nil, "tor", errors.New("tor-only policy active, but no Tor SOCKS5 proxy is configured (direct fallback blocked)")
		}
		if torTarget == "" {
			return nil, "tor", errors.New("tor-only policy active, but target has no reachable address")
		}
		conn, err := DialSOCKS5(cfg.SocksAddr, torTarget, timeout)
		if err != nil {
			return nil, "tor", fmt.Errorf("tor-only dial via %s failed (direct fallback blocked): %w", cfg.SocksAddr, err)
		}
		return conn, "tor", nil

	case PolicyTorPreferred:
		if cfg.SocksAddr != "" && torTarget != "" {
			if conn, err := DialSOCKS5(cfg.SocksAddr, torTarget, timeout); err == nil {
				return conn, "tor", nil
			}
		}
		if directAddr == "" {
			return nil, "direct", errors.New("tor-preferred dial failed and no direct address available")
		}
		conn, err := net.DialTimeout("tcp", directAddr, timeout)
		return conn, "direct", err

	case PolicyDirectFirst:
		if directAddr != "" {
			if conn, err := net.DialTimeout("tcp", directAddr, timeout); err == nil {
				return conn, "direct", nil
			}
		}
		if cfg.SocksAddr != "" && torTarget != "" {
			conn, err := DialSOCKS5(cfg.SocksAddr, torTarget, timeout)
			return conn, "tor", err
		}
		return nil, "direct", errors.New("direct dial failed and no Tor SOCKS5 proxy configured")

	default:
		return nil, "", fmt.Errorf("unknown tor policy: %s", policy)
	}
}

// DialSOCKS5 connects to targetAddr ("host:port" or "xyz.onion:port") through a SOCKS5 proxy.
func DialSOCKS5(proxyAddr, targetAddr string, timeout time.Duration) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid target address %q: %w", targetAddr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid target port %q", portStr)
	}

	conn, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return nil, fmt.Errorf("connect socks5 proxy %s: %w", proxyAddr, err)
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	// Greeting: version 5, 1 auth method (0x00 = no auth)
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		conn.Close()
		return nil, err
	}
	var authResp [2]byte
	if _, err := io.ReadFull(conn, authResp[:]); err != nil {
		conn.Close()
		return nil, err
	}
	if authResp[0] != 0x05 || authResp[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5 auth rejected: %v", authResp)
	}

	// CONNECT request
	req := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			req = append(req, 0x01)
			req = append(req, ip4...)
		} else {
			req = append(req, 0x04)
			req = append(req, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			conn.Close()
			return nil, errors.New("hostname too long for socks5")
		}
		req = append(req, 0x03, byte(len(host)))
		req = append(req, []byte(host)...)
	}
	var portBytes [2]byte
	binary.BigEndian.PutUint16(portBytes[:], uint16(port))
	req = append(req, portBytes[:]...)

	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, err
	}

	var hdr [4]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		conn.Close()
		return nil, err
	}
	if hdr[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5 connect failed with code 0x%02x", hdr[1])
	}
	// Drain bound address
	switch hdr[3] {
	case 0x01:
		var b [6]byte
		_, _ = io.ReadFull(conn, b[:])
	case 0x04:
		var b [18]byte
		_, _ = io.ReadFull(conn, b[:])
	case 0x03:
		var l [1]byte
		if _, err := io.ReadFull(conn, l[:]); err == nil {
			b := make([]byte, int(l[0])+2)
			_, _ = io.ReadFull(conn, b)
		}
	}
	return conn, nil
}
