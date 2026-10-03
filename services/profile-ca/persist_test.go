package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// newPersistentCA starts a CA whose key, certificate and records live in dir.
func newPersistentCA(t *testing.T, dir string) *ProfileCA {
	t.Helper()
	ca, err := NewProfileCA(24*time.Hour, filepath.Join(dir, "ca.key"))
	if err != nil {
		t.Fatalf("start CA in %s: %v", dir, err)
	}
	return ca
}

func parseLeaf(t *testing.T, certPEM string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		t.Fatal("no PEM block in issued certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse issued certificate: %v", err)
	}
	return cert
}

func issue(t *testing.T, ca *ProfileCA, cn string) *x509.Certificate {
	t.Helper()
	certPEM, _, err := ca.IssueInfraCert(cn)
	if err != nil {
		t.Fatalf("issue %s: %v", cn, err)
	}
	return parseLeaf(t, certPEM)
}

// TestPersist_FirstStart: no files -> files created with the SPEC modes,
// first serial 3, as before persistence existed.
func TestPersist_FirstStart(t *testing.T) {
	dir := t.TempDir()
	ca := newPersistentCA(t, dir)
	leaf := issue(t, ca, "first")
	if leaf.SerialNumber.Int64() != firstSerial {
		t.Errorf("first serial: got %d want %d", leaf.SerialNumber.Int64(), firstSerial)
	}
	for name, want := range map[string]os.FileMode{"ca.key": 0o600, "ca.crt": 0o644, "records.json": 0o600} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s mode: got %o want %o", name, got, want)
		}
	}
}

// TestPersist_RestartRoundTrip: valid and revoked records, CA certificate bytes,
// old leaves and serials survive a restart; reissue works after it.
func TestPersist_RestartRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ca1 := newPersistentCA(t, dir)
	leafA := issue(t, ca1, "a")
	leafB := issue(t, ca1, "b")
	if err := ca1.Revoke("b"); err != nil {
		t.Fatalf("revoke b: %v", err)
	}
	caPEM1 := ca1.CACertPEM()

	ca2 := newPersistentCA(t, dir) // "restart"
	if ca2.CACertPEM() != caPEM1 {
		t.Error("CA certificate bytes changed across restart")
	}
	recA, okA := ca2.GetRecord("a")
	recB, okB := ca2.GetRecord("b")
	if !okA || !okB {
		t.Fatalf("records lost: a=%v b=%v", okA, okB)
	}
	if !certIsValid(recA) {
		t.Error("a should be valid after restart")
	}
	if !recB.Revoked || certIsValid(recB) {
		t.Error("b should still be revoked after restart")
	}
	if recA.Serial != leafA.SerialNumber.Int64() || recB.Serial != leafB.SerialNumber.Int64() {
		t.Errorf("serials not persisted: a %d/%d b %d/%d", recA.Serial, leafA.SerialNumber.Int64(), recB.Serial, leafB.SerialNumber.Int64())
	}

	// Old leaves still verify against the reloaded CA certificate.
	pool := x509.NewCertPool()
	pool.AddCert(ca2.CACert())
	for _, leaf := range []*x509.Certificate{leafA, leafB} {
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
			t.Errorf("leaf %s no longer verifies: %v", leaf.Subject.CommonName, err)
		}
	}

	// Serials keep increasing and never repeat.
	leafC := issue(t, ca2, "c")
	seen := map[int64]string{}
	for _, l := range []*x509.Certificate{leafA, leafB, leafC} {
		s := l.SerialNumber.Int64()
		if prev, dup := seen[s]; dup {
			t.Errorf("serial %d issued twice (%s and %s)", s, prev, l.Subject.CommonName)
		}
		seen[s] = l.Subject.CommonName
	}
	if leafC.SerialNumber.Int64() <= leafB.SerialNumber.Int64() {
		t.Errorf("serial after restart %d not above %d", leafC.SerialNumber.Int64(), leafB.SerialNumber.Int64())
	}

	// Reissue after the restart un-revokes b, and that also survives a restart.
	if err := ca2.Reissue("b"); err != nil {
		t.Fatalf("reissue b after restart: %v", err)
	}
	ca3 := newPersistentCA(t, dir)
	if rec, _ := ca3.GetRecord("b"); rec == nil || rec.Revoked {
		t.Error("reissue of b did not survive a restart")
	}
}

// TestPersist_StartFailures: unreadable or inconsistent files fail start and
// never get replaced.
func TestPersist_StartFailures(t *testing.T) {
	setup := func(t *testing.T) string {
		dir := t.TempDir()
		ca := newPersistentCA(t, dir)
		issue(t, ca, "a")
		return dir
	}
	cases := []struct {
		name   string
		break_ func(t *testing.T, dir string)
		want   string
	}{
		{"corrupt key", func(t *testing.T, dir string) {
			os.WriteFile(filepath.Join(dir, "ca.key"), []byte("not a key"), 0o600) //nolint:errcheck
		}, "ca.key"},
		{"truncated records", func(t *testing.T, dir string) {
			p := filepath.Join(dir, "records.json")
			data, _ := os.ReadFile(p)
			os.WriteFile(p, data[:len(data)/2], 0o600) //nolint:errcheck
		}, "records.json"},
		{"corrupt certificate", func(t *testing.T, dir string) {
			os.WriteFile(filepath.Join(dir, "ca.crt"), []byte("garbage"), 0o644) //nolint:errcheck
		}, "ca.crt"},
		{"certificate of another key", func(t *testing.T, dir string) {
			other, _ := NewProfileCA(time.Hour, "")
			os.WriteFile(filepath.Join(dir, "ca.crt"), []byte(other.CACertPEM()), 0o644) //nolint:errcheck
		}, "does not match"},
		{"state without key", func(t *testing.T, dir string) {
			os.Remove(filepath.Join(dir, "ca.key")) //nolint:errcheck
		}, "missing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setup(t)
			before := snapshotDir(t, dir)
			tc.break_(t, dir)
			broken := snapshotDir(t, dir)
			_, err := NewProfileCA(24*time.Hour, filepath.Join(dir, "ca.key"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("start: got err %v, want one mentioning %q", err, tc.want)
			}
			if after := snapshotDir(t, dir); !equalSnapshots(broken, after) {
				t.Errorf("failed start changed files: before break %v, broken %v, after %v", keys(before), keys(broken), keys(after))
			}
		})
	}

	t.Run("unreadable records (mode 000)", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores file modes")
		}
		dir := setup(t)
		p := filepath.Join(dir, "records.json")
		if err := os.Chmod(p, 0); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(p, 0o600) //nolint:errcheck
		_, err := NewProfileCA(24*time.Hour, filepath.Join(dir, "ca.key"))
		if err == nil || !strings.Contains(err.Error(), "records.json") {
			t.Fatalf("start: got err %v, want one naming records.json", err)
		}
	})
}

// TestPersist_LeftoverTempFile: a temp file left by a crash mid-write is
// ignored, and the last complete state is used.
func TestPersist_LeftoverTempFile(t *testing.T) {
	dir := t.TempDir()
	ca1 := newPersistentCA(t, dir)
	issue(t, ca1, "a")
	if err := os.WriteFile(filepath.Join(dir, ".records.json.tmp-123"), []byte(`{"version":1,"nextSer`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".ca.key.tmp-9", ".ca.crt.tmp-9"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ca2 := newPersistentCA(t, dir)
	if _, ok := ca2.GetRecord("a"); !ok {
		t.Error("record a lost after a leftover temp file")
	}
	left, _ := filepath.Glob(filepath.Join(dir, ".*.tmp-*"))
	if len(left) != 0 {
		t.Errorf("leftover temp files not removed at start: %v", left)
	}
	issue(t, ca2, "b") // a later write still works
}

// TestPersist_StateWithoutCert_FailsStart: records.json present and ca.crt
// missing is lost state, not the v0.1.1 upgrade; start must refuse and must
// not write a new certificate.
func TestPersist_StateWithoutCert_FailsStart(t *testing.T) {
	dir := t.TempDir()
	ca := newPersistentCA(t, dir)
	issue(t, ca, "a")
	if err := os.Remove(filepath.Join(dir, "ca.crt")); err != nil {
		t.Fatal(err)
	}
	_, err := NewProfileCA(24*time.Hour, filepath.Join(dir, "ca.key"))
	if err == nil || !strings.Contains(err.Error(), "ca.crt") {
		t.Fatalf("start: got err %v, want one naming ca.crt", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "ca.crt")); statErr == nil {
		t.Error("failed start wrote a new ca.crt")
	}
}

// TestPersist_FailedWrite: if the state cannot be written, the call fails with
// errPersist, no certificate is returned and memory is unchanged.
func TestPersist_FailedWrite(t *testing.T) {
	dir := t.TempDir()
	ca := newPersistentCA(t, dir)
	issue(t, ca, "a")
	if err := os.RemoveAll(dir); err != nil { // writes can no longer create a temp file
		t.Fatal(err)
	}

	certPEM, keyPEM, err := ca.IssueInfraCert("b")
	if !errors.Is(err, errPersist) || certPEM != "" || keyPEM != "" {
		t.Errorf("issue: err=%v cert=%d bytes key=%d bytes, want errPersist and nothing returned", err, len(certPEM), len(keyPEM))
	}
	if _, ok := ca.GetRecord("b"); ok {
		t.Error("failed issue left a record in memory")
	}
	if err := ca.Revoke("a"); !errors.Is(err, errPersist) {
		t.Errorf("revoke: got %v, want errPersist", err)
	}
	if rec, _ := ca.GetRecord("a"); rec == nil || rec.Revoked {
		t.Error("failed revoke changed the record in memory")
	}
}

// TestPersist_UpgradeFromKeyOnly: v0.1.1 left only ca.key; the certificate is
// created once from that key and leaves signed by the key still verify.
func TestPersist_UpgradeFromKeyOnly(t *testing.T) {
	dir := t.TempDir()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalECPrivateKey(key)
	if err := os.WriteFile(filepath.Join(dir, "ca.key"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	ca := newPersistentCA(t, dir)
	pub, ok := ca.CACert().PublicKey.(*ecdsa.PublicKey)
	if !ok || !pub.Equal(&key.PublicKey) {
		t.Error("CA certificate not created from the existing key")
	}
	if _, err := os.Stat(filepath.Join(dir, "ca.crt")); err != nil {
		t.Errorf("ca.crt not written: %v", err)
	}
	if ca2 := newPersistentCA(t, dir); ca2.CACertPEM() != ca.CACertPEM() {
		t.Error("CA certificate changed on the next start")
	}
}

// TestPersist_ConcurrentIssueRevoke: parallel issue and revoke under the lock,
// then a reload sees exactly the in-memory state; run with -race.
func TestPersist_ConcurrentIssueRevoke(t *testing.T) {
	dir := t.TempDir()
	ca := newPersistentCA(t, dir)
	const n = 24
	var wg sync.WaitGroup
	serials := make([]int64, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cn := fmt.Sprintf("sys-%02d", i)
			certPEM, _, err := ca.IssueInfraCert(cn)
			if err != nil {
				t.Errorf("issue %s: %v", cn, err)
				return
			}
			serials[i] = parseLeaf(t, certPEM).SerialNumber.Int64()
			if i%3 == 0 {
				if err := ca.Revoke(cn); err != nil {
					t.Errorf("revoke %s: %v", cn, err)
				}
			}
		}(i)
	}
	wg.Wait()

	seen := map[int64]bool{}
	for _, s := range serials {
		if seen[s] {
			t.Errorf("serial %d issued twice", s)
		}
		seen[s] = true
	}
	reloaded := newPersistentCA(t, dir)
	mem, disk := ca.GetAllRecords(), reloaded.GetAllRecords()
	if len(mem) != n || len(disk) != n {
		t.Fatalf("records: memory %d, reloaded %d, want %d", len(mem), len(disk), n)
	}
	for _, m := range mem {
		d, ok := reloaded.GetRecord(m.CN)
		if !ok || d.Serial != m.Serial || d.Revoked != m.Revoked || d.OU != m.OU || !d.ExpiresAt.Equal(m.ExpiresAt) {
			t.Errorf("record %s differs after reload: memory %+v, reloaded %+v", m.CN, m, d)
		}
	}
	if next := issue(t, reloaded, "after").SerialNumber.Int64(); next != firstSerial+n {
		t.Errorf("next serial after reload: got %d want %d", next, firstSerial+n)
	}
}

// TestHandlers_PersistFailure_500: a failed state write answers 500 on issue
// and revoke, and the issue response carries no certificate.
func TestHandlers_PersistFailure_500(t *testing.T) {
	dir := t.TempDir()
	ca := newPersistentCA(t, dir)
	issue(t, ca, "a")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	handleIssueInfra(ca)(w, httptest.NewRequest(http.MethodPost, "/ca/certificate/issue", strings.NewReader(`{"systemName":"b"}`)))
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "BEGIN") {
		t.Errorf("issue: got %d %s, want 500 without a certificate", w.Code, w.Body.String())
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ca/certificates/{cn}", handleRevoke(ca))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/ca/certificates/a", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("revoke: got %d, want 500", w.Code)
	}
}

func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		out[e.Name()] = string(data)
	}
	return out
}

func equalSnapshots(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
