package manager

import (
	"fmt"
	"time"

	"capper/internal/database"
	"capper/internal/metadata"
	"capper/internal/store"
)

// CreateManagedDatabase registers a managed DB, stores its password secret, and
// launches the hidden alpine instance that runs the engine. subnetID is the VPC
// subnet the engine instance is placed in; netOpts carries the resolved
// dataplane attachment (bridge, CIDR, gateway, preferred IP) and is required.
func CreateManagedDatabase(st *store.Store, im InstanceManager, meta *metadata.Manager, name, project, engine, version, subnetID string, port int, netOpts *NetworkRunOpts) (database.ManagedDB, error) {
	if subnetID == "" {
		return database.ManagedDB{}, fmt.Errorf("database: subnetId is required")
	}
	if netOpts == nil {
		return database.ManagedDB{}, fmt.Errorf("database: network placement is required")
	}
	if err := st.CheckHostDeployLimit(); err != nil {
		return database.ManagedDB{}, err
	}
	db, password, err := st.Databases.Create(name, project, engine, version, subnetID, port)
	if err != nil {
		return database.ManagedDB{}, err
	}
	if _, err := st.Secrets.Create(db.SecretName, project, "managed database password", password); err != nil {
		_ = st.Databases.Delete(db.Name, project)
		return database.ManagedDB{}, fmt.Errorf("database: store password secret: %w", err)
	}
	instanceID, err := im.ProvisionDatabase(meta, db, project, password, "alpine", netOpts)
	if err != nil {
		_ = st.Secrets.Delete(db.SecretName, project)
		_ = st.Databases.Delete(db.Name, project)
		return database.ManagedDB{}, err
	}
	if err := st.Databases.UpdateInstanceID(db.ID, instanceID, database.DBStatusRunning); err != nil {
		return database.ManagedDB{}, err
	}
	db.InstanceID = instanceID
	db.Status = database.DBStatusRunning
	return db, nil
}

// DeleteManagedDatabase removes the backing instance, secret, and DB record.
func DeleteManagedDatabase(st *store.Store, im InstanceManager, nameOrID, project string) (database.ManagedDB, error) {
	db, err := st.Databases.Get(nameOrID, project)
	if err != nil {
		return database.ManagedDB{}, err
	}
	if db.InstanceID != "" {
		if _, _, stopErr := im.Stop(db.InstanceID, 5*time.Second, true); stopErr != nil {
			return database.ManagedDB{}, fmt.Errorf("cannot stop backing instance %s: %w", db.InstanceID, stopErr)
		}
		if err := st.Databases.UpdateStatus(db.ID, project, database.DBStatusStopped); err != nil {
			return database.ManagedDB{}, fmt.Errorf("cannot record stopped status for backing instance %s: %w", db.InstanceID, err)
		}
		if removeErr := im.Remove(db.InstanceID); removeErr != nil {
			return database.ManagedDB{}, fmt.Errorf("cannot remove backing instance %s: %w", db.InstanceID, removeErr)
		}
	}
	if db.SecretName != "" {
		_ = st.Secrets.Delete(db.SecretName, project)
	}
	if err := st.Databases.Delete(nameOrID, project); err != nil {
		return database.ManagedDB{}, err
	}
	return db, nil
}
