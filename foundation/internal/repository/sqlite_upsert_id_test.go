package repository_test

import (
	"testing"

	"arrowhead/foundation/internal/model"
	"arrowhead/foundation/internal/repository"
)

// TestSQLiteSave_UpsertReturnsOwnID: re-saving an existing instance after
// another insert must return that instance's own id. SQLite's LastInsertId is
// per connection and is not reset by the UPDATE branch of an upsert, so it
// cannot be used to tell an insert from an update.
func TestSQLiteSave_UpsertReturnsOwnID(t *testing.T) {
	r, err := repository.NewSQLiteRepository(t.TempDir() + "/legacy.db")
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	defer r.Close()

	a := &model.ServiceInstance{ServiceDefinition: "temp", ProviderSystem: model.System{SystemName: "a", Address: "10.0.0.1", Port: 9000}, Version: 1}
	b := &model.ServiceInstance{ServiceDefinition: "temp", ProviderSystem: model.System{SystemName: "b", Address: "10.0.0.2", Port: 9000}, Version: 1}
	first, err := r.Save(a)
	if err != nil {
		t.Fatalf("save a: %v", err)
	}
	if _, err := r.Save(b); err != nil {
		t.Fatalf("save b: %v", err)
	}
	again, err := r.Save(&model.ServiceInstance{ServiceDefinition: "temp", ProviderSystem: model.System{SystemName: "a", Address: "10.0.0.1", Port: 9000}, Version: 1, ServiceUri: "/new"})
	if err != nil {
		t.Fatalf("re-save a: %v", err)
	}
	if again.ID != first.ID {
		t.Errorf("re-save of a: got id %d, want %d (a's own id)", again.ID, first.ID)
	}
}

// TestSQLiteSave_ClosedDB_ReturnsError: a failed write is reported, not
// silently returned as if stored.
func TestSQLiteSave_ClosedDB_ReturnsError(t *testing.T) {
	r, err := repository.NewSQLiteRepository(t.TempDir() + "/closed.db")
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	r.Close()
	if _, err := r.Save(&model.ServiceInstance{ServiceDefinition: "temp", ProviderSystem: model.System{SystemName: "a", Address: "10.0.0.1", Port: 9000}, Version: 1}); err == nil {
		t.Error("expected an error on a closed database")
	}
}
