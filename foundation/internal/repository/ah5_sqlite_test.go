package repository_test

import (
	"testing"

	"arrowhead/foundation/internal/model"
	"arrowhead/foundation/internal/repository"
)

func TestSQLiteAH5ServiceRegistryPersists(t *testing.T) {
	dbPath := t.TempDir() + "/ah5_test.db"

	store1, err := repository.NewAH5SQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewAH5SQLiteStore: %v", err)
	}
	store1.SaveDevice(&model.DeviceRegistrationRequest{Name: "gateway-1"})
	store1.SaveSystem(&model.SystemRegistrationRequest{Name: "sensor-1", Version: "1.0.0"})
	store1.SaveServiceDefinitions([]string{"temperature"})
	store1.SaveServiceInstance(&model.ServiceRegistrationRequest{
		SystemName: "sensor-1", ServiceDefinitionName: "temperature", Version: "1.0.0",
	})
	store1.Close()

	store2, err := repository.NewAH5SQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer store2.Close()

	devs := store2.AllDevices()
	if len(devs) != 1 || devs[0].Name != "gateway-1" {
		t.Errorf("devices not persisted: %v", devs)
	}
	syss := store2.AllSystems()
	if len(syss) != 1 || syss[0].Name != "sensor-1" {
		t.Errorf("systems not persisted: %v", syss)
	}
	defs := store2.AllServiceDefinitions()
	if len(defs) != 1 || defs[0].Name != "temperature" {
		t.Errorf("service definitions not persisted: %v", defs)
	}
	insts := store2.AllServiceInstances()
	if len(insts) != 1 {
		t.Fatalf("service instances not persisted: %v", insts)
	}
	if insts[0].ServiceDefinitionName != "temperature" {
		t.Errorf("wrong service def name: %q", insts[0].ServiceDefinitionName)
	}
}

func TestSQLiteAH5HasDependentSystems(t *testing.T) {
	store, err := repository.NewAH5SQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewAH5SQLiteStore: %v", err)
	}
	defer store.Close()

	store.SaveDevice(&model.DeviceRegistrationRequest{Name: "gw"})
	store.SaveSystem(&model.SystemRegistrationRequest{Name: "sys-1", DeviceName: "gw", Version: "1.0"})

	if !store.HasDependentSystems("gw") {
		t.Error("expected dependent systems for gw")
	}
	if store.HasDependentSystems("other") {
		t.Error("expected no dependent systems for other")
	}
}

// ── created flag and write errors (SPEC.md: register → 201 new, 200 update) ──

// stores returns both AH5StoreInterface implementations, so the created-flag
// contract is checked against the same expectations for each.
func stores(t *testing.T) map[string]repository.AH5StoreInterface {
	t.Helper()
	sq, err := repository.NewAH5SQLiteStore(t.TempDir() + "/created.db")
	if err != nil {
		t.Fatalf("NewAH5SQLiteStore: %v", err)
	}
	t.Cleanup(func() { sq.Close() })
	return map[string]repository.AH5StoreInterface{
		"sqlite": sq,
		"memory": repository.NewAH5Store(),
	}
}

func TestSaveDevice_CreatedThenUpdated(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			req := &model.DeviceRegistrationRequest{Name: "GW01", Metadata: map[string]string{"v": "1"}}
			d1, created, err := s.SaveDevice(req)
			if err != nil || !created || d1 == nil {
				t.Fatalf("first save: device=%v created=%v err=%v, want created=true", d1, created, err)
			}
			req.Metadata = map[string]string{"v": "2"}
			d2, created, err := s.SaveDevice(req)
			if err != nil || created || d2 == nil {
				t.Fatalf("second save: device=%v created=%v err=%v, want created=false", d2, created, err)
			}
			if d2.Metadata["v"] != "2" {
				t.Errorf("update not applied: %v", d2.Metadata)
			}
		})
	}
}

func TestSaveSystem_CreatedThenUpdated(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			req := &model.SystemRegistrationRequest{Name: "Sensor1", Version: "1.0.0"}
			if _, created, err := s.SaveSystem(req); err != nil || !created {
				t.Fatalf("first save: created=%v err=%v, want created=true", created, err)
			}
			req.Version = "1.1.0"
			sys, created, err := s.SaveSystem(req)
			if err != nil || created {
				t.Fatalf("second save: created=%v err=%v, want created=false", created, err)
			}
			if sys == nil || sys.Version != "1.1.0" {
				t.Errorf("update not applied: %+v", sys)
			}
		})
	}
}

func TestSaveServiceInstance_CreatedThenUpdated(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			req := &model.ServiceRegistrationRequest{
				SystemName: "Sensor1", ServiceDefinitionName: "temperature", Version: "1.0.0",
				Metadata: map[string]string{"zone": "A"},
			}
			i1, created, err := s.SaveServiceInstance(req)
			if err != nil || !created || i1 == nil {
				t.Fatalf("first save: inst=%v created=%v err=%v, want created=true", i1, created, err)
			}
			req.Metadata = map[string]string{"zone": "B"}
			i2, created, err := s.SaveServiceInstance(req)
			if err != nil || created || i2 == nil {
				t.Fatalf("second save: inst=%v created=%v err=%v, want created=false", i2, created, err)
			}
			if i2.InstanceID != i1.InstanceID || i2.Metadata["zone"] != "B" {
				t.Errorf("second save: id %q -> %q, metadata %v", i1.InstanceID, i2.InstanceID, i2.Metadata)
			}
		})
	}
}

// TestSQLiteWritesOnClosedDB_ReturnError: every write path returns the write
// error instead of panicking on a nil sql.Result (the old `res, _ := Exec`).
func TestSQLiteWritesOnClosedDB_ReturnError(t *testing.T) {
	s, err := repository.NewAH5SQLiteStore(t.TempDir() + "/closed.db")
	if err != nil {
		t.Fatalf("NewAH5SQLiteStore: %v", err)
	}
	s.Close()

	checks := map[string]func() error{
		"SaveDevice": func() error { _, _, err := s.SaveDevice(&model.DeviceRegistrationRequest{Name: "GW01"}); return err },
		"SaveSystem": func() error { _, _, err := s.SaveSystem(&model.SystemRegistrationRequest{Name: "Sys1"}); return err },
		"SaveServiceInstance": func() error {
			_, _, err := s.SaveServiceInstance(&model.ServiceRegistrationRequest{SystemName: "Sys1", ServiceDefinitionName: "svc"})
			return err
		},
		"UpdateDevice": func() error { _, _, err := s.UpdateDevice(&model.DeviceRegistrationRequest{Name: "GW01"}); return err },
		"UpdateSystem": func() error { _, _, err := s.UpdateSystem(&model.SystemRegistrationRequest{Name: "Sys1"}); return err },
		"UpdateServiceInstance": func() error {
			_, _, err := s.UpdateServiceInstance(&model.ServiceUpdateRequest{InstanceID: "x"})
			return err
		},
		"DeleteDevice":          func() error { _, err := s.DeleteDevice("GW01"); return err },
		"DeleteSystem":          func() error { _, err := s.DeleteSystem("Sys1"); return err },
		"DeleteServiceInstance": func() error { _, err := s.DeleteServiceInstance("x"); return err },
	}
	for name, call := range checks {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked: %v", r)
				}
			}()
			if err := call(); err == nil {
				t.Error("expected an error on a closed database")
			}
		})
	}
}
