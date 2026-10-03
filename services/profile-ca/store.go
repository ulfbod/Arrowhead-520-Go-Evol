// store.go — persistent CA state (SPEC.md "Persistent State").
//
// Three files, by default next to CA_KEY_FILE in the /data volume:
//
//	ca.key        CA private key (0600)
//	ca.crt        CA certificate (0644), reused byte for byte across restarts
//	records.json  certificate records and the next serial number (0600)
//
// Every write goes to a temp file in the same directory, is fsynced, renamed over
// the target and the directory is fsynced, so a crash leaves either the old or the
// new file, never a partial one. Leftover temp files are ignored at start-up.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// errPersist marks a failed state write; handlers answer 500 for it.
var errPersist = errors.New("persist CA state")

const (
	stateVersion = 1
	// firstSerial is the first serial issued to a leaf; the CA certificate is serial 1.
	firstSerial int64 = 3
)

// statePaths names the persisted files. All empty means an ephemeral CA.
type statePaths struct {
	key, cert, state string
}

func (p statePaths) persistent() bool { return p.key != "" }

// resolvePaths applies the defaults: ca.crt and records.json next to keyFile.
func resolvePaths(keyFile, certFile, stateFile string) statePaths {
	if keyFile == "" {
		return statePaths{}
	}
	dir := filepath.Dir(keyFile)
	if certFile == "" {
		certFile = filepath.Join(dir, "ca.crt")
	}
	if stateFile == "" {
		stateFile = filepath.Join(dir, "records.json")
	}
	return statePaths{key: keyFile, cert: certFile, state: stateFile}
}

type stateRecord struct {
	CN        string    `json:"cn"`
	OU        string    `json:"ou"`
	Serial    int64     `json:"serial"`
	IssuedAt  time.Time `json:"issuedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Revoked   bool      `json:"revoked"`
}

type stateDoc struct {
	Version    int           `json:"version"`
	NextSerial int64         `json:"nextSerial"`
	Records    []stateRecord `json:"records"`
}

// removeLeftoverTemps deletes temp files left by an interrupted writeFileAtomic
// (".<name>.tmp-*" next to each state file). They are never complete state.
func removeLeftoverTemps(p statePaths) ([]string, error) {
	var removed []string
	for _, f := range []string{p.key, p.cert, p.state} {
		matches, err := filepath.Glob(filepath.Join(filepath.Dir(f), "."+filepath.Base(f)+".tmp-*"))
		if err != nil {
			return removed, err
		}
		for _, m := range matches {
			if err := os.Remove(m); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return removed, fmt.Errorf("remove leftover temp file %s: %w", m, err)
			}
			removed = append(removed, m)
		}
	}
	return removed, nil
}

// readIfExists returns (data, true, nil) for a readable file, (nil, false, nil)
// for a missing one, and an error for anything else (e.g. permission denied).
func readIfExists(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	return data, true, nil
}

// writeFileAtomic writes data to path via a temp file in the same directory:
// write, fsync, rename, fsync the directory.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) //nolint:errcheck // already renamed on success
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// encodeState serialises the records (sorted by CN, for stable files).
func encodeState(records map[string]*CertRecord, nextSerial int64) ([]byte, error) {
	doc := stateDoc{Version: stateVersion, NextSerial: nextSerial, Records: make([]stateRecord, 0, len(records))}
	for _, r := range records {
		doc.Records = append(doc.Records, stateRecord{
			CN: r.CN, OU: r.OU, Serial: r.Serial,
			IssuedAt: r.IssuedAt, ExpiresAt: r.ExpiresAt, Revoked: r.Revoked,
		})
	}
	sort.Slice(doc.Records, func(i, j int) bool { return doc.Records[i].CN < doc.Records[j].CN })
	return json.MarshalIndent(doc, "", "  ")
}

// decodeState parses and validates a records.json document.
func decodeState(data []byte) (map[string]*CertRecord, int64, error) {
	var doc stateDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, 0, err
	}
	if doc.Version != stateVersion {
		return nil, 0, fmt.Errorf("unsupported version %d", doc.Version)
	}
	if doc.NextSerial < firstSerial {
		return nil, 0, fmt.Errorf("nextSerial %d below %d", doc.NextSerial, firstSerial)
	}
	records := make(map[string]*CertRecord, len(doc.Records))
	for _, r := range doc.Records {
		if r.CN == "" {
			return nil, 0, errors.New("record with empty cn")
		}
		if r.Serial >= doc.NextSerial {
			return nil, 0, fmt.Errorf("record %q has serial %d >= nextSerial %d", r.CN, r.Serial, doc.NextSerial)
		}
		if _, dup := records[r.CN]; dup {
			return nil, 0, fmt.Errorf("duplicate record %q", r.CN)
		}
		records[r.CN] = &CertRecord{
			CN: r.CN, OU: r.OU, Serial: r.Serial,
			IssuedAt: r.IssuedAt, ExpiresAt: r.ExpiresAt, Revoked: r.Revoked,
		}
	}
	return records, doc.NextSerial, nil
}
