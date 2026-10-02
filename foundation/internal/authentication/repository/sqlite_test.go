package repository_test

import (
	"testing"
	"time"

	"arrowhead/foundation/internal/authentication/model"
	"arrowhead/foundation/internal/authentication/repository"
)

func TestSQLiteAuthTokenPersists(t *testing.T) {
	dbPath := t.TempDir() + "/auth_test.db"
	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	login := time.Now().UTC().Truncate(time.Second)

	repo1, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	repo1.Save(&model.IdentityToken{
		Token:      "tok-abc",
		SystemName: "my-system",
		ExpiresAt:  exp,
		LoginTime:  login,
	})
	repo1.Close()

	repo2, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer repo2.Close()

	tok, ok := repo2.FindByToken("tok-abc")
	if !ok {
		t.Fatal("token not found after reopen")
	}
	if tok.SystemName != "my-system" {
		t.Errorf("systemName mismatch: %q", tok.SystemName)
	}
	if !tok.ExpiresAt.Equal(exp) {
		t.Errorf("expiresAt mismatch: got %v want %v", tok.ExpiresAt, exp)
	}
}

func TestSQLiteAuthTokenDeleteAndExpired(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	defer repo.Close()

	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	repo.Save(&model.IdentityToken{Token: "valid", SystemName: "s", ExpiresAt: future, LoginTime: time.Now()})
	repo.Save(&model.IdentityToken{Token: "expired", SystemName: "s", ExpiresAt: past, LoginTime: time.Now()})

	repo.DeleteExpired()

	if _, ok := repo.FindByToken("valid"); !ok {
		t.Error("valid token should still exist")
	}
	if _, ok := repo.FindByToken("expired"); ok {
		t.Error("expired token should be gone")
	}

	if !repo.Delete("valid") {
		t.Error("Delete returned false for existing token")
	}
	if _, ok := repo.FindByToken("valid"); ok {
		t.Error("deleted token should not exist")
	}
}

// TestSQLiteSave_ClosedDB_ReturnsError: a token that could not be stored must
// not be handed out by login.
func TestSQLiteSave_ClosedDB_ReturnsError(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(t.TempDir() + "/closed.db")
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	repo.Close()
	if err := repo.Save(&model.IdentityToken{Token: "t", SystemName: "s", ExpiresAt: time.Now().Add(time.Hour), LoginTime: time.Now()}); err == nil {
		t.Error("expected an error on a closed database")
	}
}

// TestSQLiteDeleteBySystemName_ClosedDB_ReturnsError: a failed session revoke
// is reported, so the API cannot answer success while the tokens still verify.
func TestSQLiteDeleteBySystemName_ClosedDB_ReturnsError(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(t.TempDir() + "/closed.db")
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	repo.Close()
	if err := repo.DeleteBySystemName("s"); err == nil {
		t.Error("expected an error on a closed database")
	}
}
