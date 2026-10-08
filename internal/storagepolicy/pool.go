package storagepolicy

import (
	"fmt"

	"capper/internal/adminconfig"
	"capper/internal/hoststorage"
)

// RequireDefaultPool ensures an admin storage pool is configured and healthy.
func RequireDefaultPool(admin *adminconfig.Store, hs *hoststorage.Store) (string, error) {
	if admin == nil || hs == nil {
		return "", fmt.Errorf("storage pools are not configured: register a pool under Admin → Storage")
	}
	poolID, ok, _ := admin.Get(adminconfig.KeyDefaultInstancePool)
	if !ok || poolID == "" {
		return "", fmt.Errorf("no default storage pool configured: set one under Admin → Storage")
	}
	pool, err := hoststorage.NewManager(hs).GetPool(poolID)
	if err != nil {
		return "", fmt.Errorf("default storage pool unavailable")
	}
	if pool.Health == hoststorage.PoolDegraded {
		return "", fmt.Errorf("storage pool %q is degraded", pool.Name)
	}
	return poolID, nil
}

// ValidatePoolCapacity checks requested bytes fit the default pool.
func ValidatePoolCapacity(admin *adminconfig.Store, hs *hoststorage.Store, bytes int64) error {
	poolID, err := RequireDefaultPool(admin, hs)
	if err != nil {
		return err
	}
	if bytes <= 0 {
		return nil
	}
	pool, err := hoststorage.NewManager(hs).GetPool(poolID)
	if err != nil {
		return err
	}
	if bytes > pool.AvailableBytes {
		return fmt.Errorf("requested size (%d bytes) exceeds available pool capacity (%d bytes)", bytes, pool.AvailableBytes)
	}
	return nil
}
