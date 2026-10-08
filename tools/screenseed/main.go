// screenseed prints a bootstrap admin bearer token for a Capper store (used by docs screenshots).
// Optionally ensures a local screenshot user exists without a forced password change.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"capper/internal/iam"
	"capper/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: screenseed <store-root> [--user name:password[:role]]")
		os.Exit(2)
	}
	paths := store.NewPaths(os.Args[1])
	st, err := store.Open(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer st.Close()
	if err := st.IAM.Bootstrap(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "--user" && i+1 < len(os.Args) {
			if err := ensureUser(st, os.Args[i+1]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			i++
		}
	}

	pt, pid := st.IAM.LocalPrincipal()
	bearer, _, err := st.IAM.Issue("docs-screenshots", pt, pid, 2*time.Hour)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(bearer)
}

func ensureUser(st *store.Store, spec string) error {
	parts := strings.Split(spec, ":")
	if len(parts) < 2 {
		return fmt.Errorf("invalid --user spec %q (want name:password[:role])", spec)
	}
	name := strings.TrimSpace(parts[0])
	pass := parts[1]
	role := iam.RoleAdmin
	if len(parts) > 2 && strings.TrimSpace(parts[2]) != "" {
		role = strings.TrimSpace(parts[2])
	}
	if name == "" || pass == "" {
		return fmt.Errorf("name and password required in --user spec")
	}

	u, err := st.IAM.IAMStore().GetUser(name)
	if err != nil {
		u, err = st.IAM.CreateManagedUser(name, "", "local")
		if err != nil {
			return err
		}
	}
	if err := st.IAM.SetPassword(u.ID, pass); err != nil {
		return err
	}
	if err := st.IAM.IAMStore().SetMustChangePassword(u.ID, false); err != nil {
		return err
	}
	if err := st.IAM.AssignRole(u.ID, role); err != nil {
		return err
	}
	return nil
}
