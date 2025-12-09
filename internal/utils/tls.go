package utils

import (
    "crypto/rand"
    "crypto/rsa"
    "crypto/x509"
    "crypto/x509/pkix"
    "encoding/pem"
    "math/big"
    "net"
    "os"
    "path/filepath"
    "time"
)

func EnsureSelfSignedCert(certPath, keyPath, cn string) error {
    if _, err := os.Stat(certPath); err == nil {
        if _, err2 := os.Stat(keyPath); err2 == nil { return nil }
    }
    _ = os.MkdirAll(filepath.Dir(certPath), 0o755)
    _ = os.MkdirAll(filepath.Dir(keyPath), 0o755)
    priv, err := rsa.GenerateKey(rand.Reader, 2048)
    if err != nil { return err }
    serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
    serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
    if err != nil { return err }
    tmpl := x509.Certificate{
        SerialNumber: serialNumber,
        Subject: pkix.Name{CommonName: cn},
        NotBefore: time.Now().Add(-time.Hour),
        NotAfter:  time.Now().Add(365 * 24 * time.Hour),
        KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
        ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
        BasicConstraintsValid: true,
    }
    tmpl.DNSNames = []string{"localhost", cn}
    tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
    derBytes, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
    if err != nil { return err }
    certOut, err := os.Create(certPath)
    if err != nil { return err }
    defer certOut.Close()
    if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil { return err }
    keyOut, err := os.Create(keyPath)
    if err != nil { return err }
    defer keyOut.Close()
    if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil { return err }
    return nil
}

