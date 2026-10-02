package repository_test

import (
	"os"
	"testing"

	"arrowhead/foundation/internal/authentication/repository"
)

func TestSQLiteIdentitySaveAndGet(t *testing.T) {
	f, _ := os.CreateTemp("", "auth-identity-*.db")
	f.Close()
	defer os.Remove(f.Name())

	repo, err := repository.NewSQLiteIdentityRepository(f.Name())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repo.Save(repository.Identity{SystemName: "sql-sys", PasswordHash: "h", Sysop: true})
	got, ok := repo.Get("sql-sys")
	if !ok {
		t.Fatal("not found after save")
	}
	if !got.Sysop {
		t.Error("Sysop = false, want true")
	}
}

// TestSQLiteIdentitySave_ClosedDB_ReturnsError: a failed identity write is
// reported, not answered as created.
func TestSQLiteIdentitySave_ClosedDB_ReturnsError(t *testing.T) {
	repo, err := repository.NewSQLiteIdentityRepository(t.TempDir() + "/closed.db")
	if err != nil {
		t.Fatalf("NewSQLiteIdentityRepository: %v", err)
	}
	repo.Close()
	if err := repo.Save(repository.Identity{SystemName: "Sys1", PasswordHash: "h"}); err == nil {
		t.Error("expected an error on a closed database")
	}
}

// TestSQLiteIdentityDelete_ClosedDB_ReturnsError: a failed delete is reported,
// so the API cannot answer success while the identity can still log in.
func TestSQLiteIdentityDelete_ClosedDB_ReturnsError(t *testing.T) {
	repo, err := repository.NewSQLiteIdentityRepository(t.TempDir() + "/closed.db")
	if err != nil {
		t.Fatalf("NewSQLiteIdentityRepository: %v", err)
	}
	repo.Close()
	if err := repo.Delete("Sys1"); err == nil {
		t.Error("expected an error on a closed database")
	}
}
