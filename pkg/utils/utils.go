/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/pool"
	"github.com/miekg/dns"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/exp/constraints"
	"math/big"
	"net"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"time"
	"unsafe"
)

// GetIPFromAddr returns a net.IP from the given net.Addr.
// addr can be *net.TCPAddr, *net.UDPAddr, *net.IPNet, *net.IPAddr
// Will return nil otherwise.
func GetIPFromAddr(addr net.Addr) (ip net.IP) {
	switch v := addr.(type) {
	case *net.TCPAddr:
		return v.IP
	case *net.UDPAddr:
		return v.IP
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	}
	return nil
}

// GetAddrFromAddr returns netip.Addr from net.Addr.
// See also: GetIPFromAddr.
func GetAddrFromAddr(addr net.Addr) netip.Addr {
	a, _ := netip.AddrFromSlice(GetIPFromAddr(addr))
	return a
}

// SplitSchemeAndHost splits addr to protocol and host.
func SplitSchemeAndHost(addr string) (protocol, host string) {
	if protocol, host, ok := SplitString2(addr, "://"); ok {
		return protocol, host
	} else {
		return "", addr
	}
}

// GetMsgKey unpacks m and set its id to salt.
func GetMsgKey(m *dns.Msg, salt uint16) (string, error) {
	wireMsg, err := m.Pack()
	if err != nil {
		return "", err
	}
	wireMsg[0] = byte(salt >> 8)
	wireMsg[1] = byte(salt)
	return BytesToStringUnsafe(wireMsg), nil
}

// GetMsgKeyWithBytesSalt unpacks m and appends salt to the string.
func GetMsgKeyWithBytesSalt(m *dns.Msg, salt []byte) (string, error) {
	wireMsg, buf, err := pool.PackBuffer(m)
	if err != nil {
		return "", err
	}
	defer buf.Release()

	wireMsg[0] = 0
	wireMsg[1] = 0

	sb := new(strings.Builder)
	sb.Grow(len(wireMsg) + len(salt))
	sb.Write(wireMsg)
	sb.Write(salt)

	return sb.String(), nil
}

// GetMsgKeyWithInt64Salt unpacks m and appends salt to the string.
func GetMsgKeyWithInt64Salt(m *dns.Msg, salt int64) (string, error) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(salt))
	return GetMsgKeyWithBytesSalt(m, b)
}

// BytesToStringUnsafe converts bytes to string.
func BytesToStringUnsafe(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// LoadCertPool reads and loads certificates in certs.
func LoadCertPool(certs []string) (*x509.CertPool, error) {
	rootCAs := x509.NewCertPool()
	for _, cert := range certs {
		b, err := os.ReadFile(cert)
		if err != nil {
			return nil, err
		}

		if ok := rootCAs.AppendCertsFromPEM(b); !ok {
			return nil, fmt.Errorf("no certificate was successfully parsed in %s", cert)
		}
	}
	return rootCAs, nil
}

// GenerateCertificate generates an ecdsa certificate with given dnsName.
// This should only use in test.
func GenerateCertificate(dnsName string) (cert tls.Certificate, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return
	}

	//serial number
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		err = fmt.Errorf("generate serial number: %w", err)
		return
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},

		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(10, 0, 0),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return
	}
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return tls.X509KeyPair(certPEM, keyPEM)
}

var charBlockExpr = regexp.MustCompile("\\S+")

// SplitLineReg extracts words from s by using regexp "\S+".
func SplitLineReg(s string) []string {
	return charBlockExpr.FindAllString(s, -1)
}

// RemoveComment removes comment after "symbol".
func RemoveComment(s, symbol string) string {
	if i := strings.Index(s, symbol); i >= 0 {
		return s[:i]
	}
	return s
}

// SplitString2 split s to two parts by given symbol
func SplitString2(s, symbol string) (s1 string, s2 string, ok bool) {
	if len(symbol) == 0 {
		return "", s, true
	}
	if i := strings.Index(s, symbol); i >= 0 {
		return s[:i], s[i+len(symbol):], true
	}
	return "", "", false
}

// ClosedChan returns true if c is closed.
// c must not use for sending data and must be used in close() only.
// If ClosedChan receives something from c, it panics.
func ClosedChan(c chan struct{}) bool {
	select {
	case _, ok := <-c:
		if !ok {
			return true
		}
		panic("received from the chan")
	default:
		return false
	}
}

// WeakDecode decodes args from config to output.
func WeakDecode(in map[string]interface{}, output interface{}) error {
	config := &mapstructure.DecoderConfig{
		ErrorUnused:      true,
		Result:           output,
		WeaklyTypedInput: true,
		TagName:          "yaml",
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(in)
}

func SetDefaultNum[K constraints.Integer | constraints.Float](p *K, d K) {
	if *p == 0 {
		*p = d
	}
}
