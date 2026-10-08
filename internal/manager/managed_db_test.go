package manager

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"capper/internal/database"
	"capper/internal/store"
	"capper/internal/types"
)

func TestDeleteManagedDatabaseRetainsStoppedStatusWhenRemovalFails(t *testing.T) {
	st, err := store.Open(store.NewPaths(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	db, _, err := st.Databases.Create("protected-db", "project", "postgres", "16", "subnet", 5432)
	if err != nil {
		t.Fatal(err)
	}
	const instanceID = "protected-instance"
	instanceDir := filepath.Join(st.Paths.Instances, instanceID)
	if err := os.MkdirAll(filepath.Join(instanceDir, "rootfs"), 0o700); err != nil {
		t.Fatal(err)
	}
	inst := types.Instance{
		ID:                    instanceID,
		Name:                  instanceID,
		Status:                types.StatusStopped,
		CreatedAt:             time.Now().UTC().Format(time.RFC3339),
		RootFSPath:            filepath.Join(instanceDir, "rootfs"),
		TerminationProtection: true,
	}
	if err := st.InsertInstance(inst); err != nil {
		t.Fatal(err)
	}
	if err := st.WriteInstanceJSON(inst); err != nil {
		t.Fatal(err)
	}
	if err := st.Databases.UpdateInstanceID(db.ID, instanceID, database.DBStatusRunning); err != nil {
		t.Fatal(err)
	}

	_, err = DeleteManagedDatabase(st, InstanceManager{Store: st}, db.ID, "project")
	if err == nil {
		t.Fatal("expected termination protection to prevent instance removal")
	}
	got, err := st.Databases.Get(db.ID, "project")
	if err != nil {
		t.Fatalf("managed database record should remain for retry: %v", err)
	}
	if got.Status != database.DBStatusStopped {
		t.Fatalf("managed database status = %q, want stopped", got.Status)
	}
}
