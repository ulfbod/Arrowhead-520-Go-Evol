// ca.go — Arrowhead 5.2 profile-based Local Cloud CA.
//
// Certificate profiles encoded in Subject OrganizationalUnit (OU):
//
//	lo — Local Cloud CA (root)
//	on — Onboarding  (may request Device certs)
//	de — Device      (may request System certs)
//	sy — System      (used for service-to-service mTLS)
//
// Issuance rules strictly enforced:
//
//	HTTP bootstrap → on
//	on client cert → de
//	de client cert → sy
//
// Features:
//   - CertRecord registry (protected by mu; persisted to records.json, see store.go)
//   - Revoke(cn) — marks revoked
//   - Reissue(cn) — un-revokes
//   - GetAll() — returns all non-revoked records (snapshot)
//   - GetRecord(cn) — returns record including revoked (for CA-as-PIP)
//   - GetAllRecords() — returns ALL records including revoked (for CA-as-PIP)
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"
)

// CertProfile is an Arrowhead 5.2 certificate profile tier.
type CertProfile string

const (
	ProfileOnboarding CertProfile = "on"
	ProfileDevice     CertProfile = "de"
	ProfileSystem     CertProfile = "sy"
)

// CertRecord holds registry metadata for an issued certificate.
// Records are immutable once stored: a change replaces the pointer in the map, so
// a record returned by GetRecord never changes under the caller.
type CertRecord struct {
	CN        string
	OU        string
	Serial    int64
	IssuedAt  time.Time
	ExpiresAt time.Time
	Revoked   bool
}

// ProfileCA is the Local Cloud Certificate Authority with profile enforcement.
type ProfileCA struct {
	caKey     *ecdsa.PrivateKey
	caCert    *x509.Certificate
	caCertPEM []byte
	certDur   time.Duration
	paths     statePaths

	// mu protects records and nextSerial, and serialises state writes.
	mu         sync.Mutex
	records    map[string]*CertRecord
	nextSerial int64
}

// NewProfileCA creates a Local Cloud CA with the given leaf cert lifetime.
// keyFile is the CA key path; ca.crt and records.json are kept next to it.
// An empty keyFile gives an ephemeral CA that persists nothing; only unit tests
// use that (main always passes a path, see CA_KEY_FILE in SPEC.md).
func NewProfileCA(certDuration time.Duration, keyFile string) (*ProfileCA, error) {
	return newProfileCA(certDuration, resolvePaths(keyFile, "", ""))
}

// NewProfileCAFromFiles is NewProfileCA with explicit certificate and state
// paths (empty = default next to keyFile).
func NewProfileCAFromFiles(certDuration time.Duration, keyFile, certFile, stateFile string) (*ProfileCA, error) {
	return newProfileCA(certDuration, resolvePaths(keyFile, certFile, stateFile))
}

// newProfileCA loads the persisted CA (SPEC.md "Persistent State", start-up) or,
// on a first start, generates and persists a new one. Any existing file that
// cannot be read or parsed is an error: the CA never silently replaces state.
func newProfileCA(certDuration time.Duration, p statePaths) (*ProfileCA, error) {
	ca := &ProfileCA{
		certDur:    certDuration,
		paths:      p,
		records:    make(map[string]*CertRecord),
		nextSerial: firstSerial,
	}
	if !p.persistent() {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate CA key: %w", err)
		}
		if err := ca.setNewCACert(key); err != nil {
			return nil, err
		}
		return ca, nil
	}

	removed, err := removeLeftoverTemps(p)
	for _, f := range removed {
		log.Printf("[profile-ca] removed leftover temp file %s from an interrupted write", f)
	}
	if err != nil {
		return nil, err
	}

	keyData, keyExists, err := readIfExists(p.key)
	if err != nil {
		return nil, fmt.Errorf("read CA key %s: %w", p.key, err)
	}
	certData, certExists, err := readIfExists(p.cert)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate %s: %w", p.cert, err)
	}
	stateData, stateExists, err := readIfExists(p.state)
	if err != nil {
		return nil, fmt.Errorf("read CA state %s: %w", p.state, err)
	}
	if !keyExists && (certExists || stateExists) {
		return nil, fmt.Errorf("CA key %s is missing but %s or %s exists; refusing to start with a new key", p.key, p.cert, p.state)
	}
	if stateExists && !certExists {
		// Only key-only (v0.1.1) may get a new CA certificate; with records present
		// a missing certificate means state was lost, not an upgrade.
		return nil, fmt.Errorf("CA certificate %s is missing but %s exists; refusing to create a new CA certificate", p.cert, p.state)
	}

	// Key: load, or generate and persist on a first start.
	var key *ecdsa.PrivateKey
	if keyExists {
		key, err = parseECKey(keyData)
		if err != nil {
			return nil, fmt.Errorf("CA key %s: %w", p.key, err)
		}
		log.Printf("[profile-ca] loaded CA key from %s", p.key)
	} else {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate CA key: %w", err)
		}
		der, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("marshal new CA key: %w", err)
		}
		if err := writeFileAtomic(p.key, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
			return nil, fmt.Errorf("write CA key %s: %w", p.key, err)
		}
		log.Printf("[profile-ca] generated and saved new CA key to %s", p.key)
	}

	// Certificate: reuse byte for byte, or create once from the key (first start,
	// or the v0.1.1 upgrade with key only).
	if certExists {
		if err := ca.loadCACert(certData, key); err != nil {
			return nil, fmt.Errorf("CA certificate %s: %w", p.cert, err)
		}
		log.Printf("[profile-ca] loaded CA certificate from %s", p.cert)
	} else {
		if err := ca.setNewCACert(key); err != nil {
			return nil, err
		}
		if err := writeFileAtomic(p.cert, ca.caCertPEM, 0o644); err != nil {
			return nil, fmt.Errorf("write CA certificate %s: %w", p.cert, err)
		}
		log.Printf("[profile-ca] created and saved CA certificate to %s", p.cert)
	}

	// Records and serial counter.
	if stateExists {
		records, next, err := decodeState(stateData)
		if err != nil {
			return nil, fmt.Errorf("CA state %s: %w", p.state, err)
		}
		ca.records, ca.nextSerial = records, next
		log.Printf("[profile-ca] loaded %d certificate records from %s (next serial %d)", len(records), p.state, next)
	}
	return ca, nil
}

func parseECKey(data []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

// setNewCACert creates the self-signed CA certificate for key.
func (ca *ProfileCA) setNewCACert(key *ecdsa.PrivateKey) error {
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:         "Arrowhead Local Cloud CA",
			Organization:       []string{"Arrowhead"},
			OrganizationalUnit: []string{"lo"},
		},
		// SANs needed so Go TLS accepts the CA cert as a server cert on the mTLS port.
		DNSNames:              []string{"profile-ca", "localhost"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create CA cert: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return fmt.Errorf("parse CA cert: %w", err)
	}
	ca.caKey, ca.caCert = key, cert
	ca.caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return nil
}

// loadCACert parses a persisted CA certificate and checks it belongs to key.
func (ca *ProfileCA) loadCACert(data []byte, key *ecdsa.PrivateKey) error {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return errors.New("no PEM CERTIFICATE block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok || !pub.Equal(&key.PublicKey) {
		return errors.New("certificate does not match the CA key")
	}
	ca.caKey, ca.caCert, ca.caCertPEM = key, cert, data
	return nil
}

// commitLocked makes records/nextSerial the new state: it persists them first
// (if the CA is persistent) and swaps them in only after the write succeeded.
// The caller holds mu.
func (ca *ProfileCA) commitLocked(records map[string]*CertRecord, nextSerial int64) error {
	if ca.paths.persistent() {
		data, err := encodeState(records, nextSerial)
		if err != nil {
			return fmt.Errorf("%w: encode: %v", errPersist, err)
		}
		if err := writeFileAtomic(ca.paths.state, data, 0o600); err != nil {
			return fmt.Errorf("%w: %s: %v", errPersist, ca.paths.state, err)
		}
	}
	ca.records, ca.nextSerial = records, nextSerial
	return nil
}

// withRecord returns a copy of the record map with cn set to rec.
func (ca *ProfileCA) withRecord(cn string, rec *CertRecord) map[string]*CertRecord {
	out := make(map[string]*CertRecord, len(ca.records)+1)
	for k, v := range ca.records {
		out[k] = v
	}
	out[cn] = rec
	return out
}

// issueCert is the internal cert issuance. Profile is set in OU.
// The certificate and key are returned only after the record and the advanced
// serial counter are durable (SPEC.md "Persistent State", writes).
func (ca *ProfileCA) issueCert(systemName string, profile CertProfile) (certPEM, keyPEM string, err error) {
	if systemName == "" {
		return "", "", errors.New("systemName is required")
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate key: %w", err)
	}

	ca.mu.Lock()
	defer ca.mu.Unlock()

	serial := ca.nextSerial
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject: pkix.Name{
			CommonName:         systemName,
			Organization:       []string{"Arrowhead"},
			OrganizationalUnit: []string{string(profile)},
		},
		DNSNames:    []string{systemName},
		NotBefore:   now.Add(-time.Minute),
		NotAfter:    now.Add(ca.certDur),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.caCert, &leafKey.PublicKey, ca.caKey)
	if err != nil {
		return "", "", fmt.Errorf("create certificate: %w", err)
	}
	rec := &CertRecord{
		CN:        systemName,
		OU:        string(profile),
		Serial:    serial,
		IssuedAt:  now,
		ExpiresAt: now.Add(ca.certDur),
	}
	// On a failed write nothing changes: the certificate is discarded unreturned
	// and its serial is used again by the next successful issue.
	if err := ca.commitLocked(ca.withRecord(systemName, rec), serial+1); err != nil {
		return "", "", err
	}

	keyBytes, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return "", "", fmt.Errorf("marshal key: %w", err)
	}
	certPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return string(certPEMBytes), string(keyPEMBytes), nil
}

// IssueOnboardingCert issues an Onboarding certificate (OU=on).
// Available over plain HTTP — no authentication required.
func (ca *ProfileCA) IssueOnboardingCert(systemName string) (certPEM, keyPEM string, err error) {
	return ca.issueCert(systemName, ProfileOnboarding)
}

// IssueDeviceCert issues a Device certificate (OU=de).
// Requires the requester to present a valid Onboarding certificate (OU=on).
func (ca *ProfileCA) IssueDeviceCert(systemName string, requesterCert *x509.Certificate) (certPEM, keyPEM string, err error) {
	if err := ca.verifyProfile(requesterCert, ProfileOnboarding); err != nil {
		return "", "", fmt.Errorf("requester profile: %w", err)
	}
	return ca.issueCert(systemName, ProfileDevice)
}

// IssueSystemCert issues a System certificate (OU=sy).
// Requires the requester to present a valid Device certificate (OU=de).
func (ca *ProfileCA) IssueSystemCert(systemName string, requesterCert *x509.Certificate) (certPEM, keyPEM string, err error) {
	if err := ca.verifyProfile(requesterCert, ProfileDevice); err != nil {
		return "", "", fmt.Errorf("requester profile: %w", err)
	}
	return ca.issueCert(systemName, ProfileSystem)
}

// IssueInfraCert issues a System certificate without profile chain enforcement.
// This backward-compatible endpoint is used by cert-provisioner for Kafka,
// RabbitMQ, and core system certificate files.
func (ca *ProfileCA) IssueInfraCert(systemName string) (certPEM, keyPEM string, err error) {
	return ca.issueCert(systemName, ProfileSystem)
}

// Reissue un-revokes a previously revoked certificate.
// Returns an error if the CN is not found or the certificate is not currently
// revoked, or an errPersist-wrapped error if the new state cannot be written.
func (ca *ProfileCA) Reissue(cn string) error {
	return ca.setRevoked(cn, false)
}

// Revoke marks the certificate with the given CN as revoked.
// Returns an error if CN is not found or is already revoked, or an
// errPersist-wrapped error if the new state cannot be written.
func (ca *ProfileCA) Revoke(cn string) error {
	return ca.setRevoked(cn, true)
}

func (ca *ProfileCA) setRevoked(cn string, revoked bool) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	rec, ok := ca.records[cn]
	if !ok {
		return fmt.Errorf("certificate not found: %s", cn)
	}
	if rec.Revoked == revoked {
		if revoked {
			return fmt.Errorf("certificate already revoked: %s", cn)
		}
		return fmt.Errorf("certificate not revoked: %s", cn)
	}
	updated := *rec
	updated.Revoked = revoked
	return ca.commitLocked(ca.withRecord(cn, &updated), ca.nextSerial)
}

// GetAll returns all non-revoked certificate records (snapshot).
func (ca *ProfileCA) GetAll() []*CertRecord {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	out := make([]*CertRecord, 0, len(ca.records))
	for _, rec := range ca.records {
		if !rec.Revoked {
			out = append(out, rec)
		}
	}
	return out
}

// GetRecord returns the cert record for cn regardless of revocation status.
// Returns (record, true) if found, (nil, false) if not found.
// Used by CA-as-PIP handlers to serve /pip/attributes/{cn}.
func (ca *ProfileCA) GetRecord(cn string) (*CertRecord, bool) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	rec, ok := ca.records[cn]
	return rec, ok
}

// GetAllRecords returns ALL cert records including revoked (snapshot).
// Used by CA-as-PIP handlers to serve /pip/subjects.
func (ca *ProfileCA) GetAllRecords() []*CertRecord {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	out := make([]*CertRecord, 0, len(ca.records))
	for _, rec := range ca.records {
		out = append(out, rec)
	}
	return out
}

// verifyProfile checks that cert was issued by this CA and has the expected OU profile.
func (ca *ProfileCA) verifyProfile(cert *x509.Certificate, expected CertProfile) error {
	pool := x509.NewCertPool()
	pool.AddCert(ca.caCert)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		return fmt.Errorf("chain invalid: %w", err)
	}
	for _, ou := range cert.Subject.OrganizationalUnit {
		if CertProfile(ou) == expected {
			return nil
		}
	}
	return fmt.Errorf("expected profile %q, got %v", expected, cert.Subject.OrganizationalUnit)
}

// CACertPEM returns the CA certificate in PEM format.
func (ca *ProfileCA) CACertPEM() string { return string(ca.caCertPEM) }

// CACert returns the parsed CA certificate.
func (ca *ProfileCA) CACert() *x509.Certificate { return ca.caCert }

// TLSCert returns a tls.Certificate using the CA's own cert+key for serving mTLS.
func (ca *ProfileCA) TLSCert() (tls.Certificate, error) {
	keyBytes, err := x509.MarshalECPrivateKey(ca.caKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshal CA key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return tls.X509KeyPair(ca.caCertPEM, keyPEM)
}
