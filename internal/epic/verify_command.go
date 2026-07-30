package epic

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultVerifyCommand is what a repository set up by the project-bootstrap
// contract offers: one executable that runs everything that has to pass.
const DefaultVerifyCommand = "bin/ci"

// ResolveVerifyCommand answers how a repository says "this works".
//
// The stage prompt used to answer it for every repository at once, with
// `cd orchestrator && go vet ./... && go test ./...` - a Go toolchain, a
// module path that does not exist, and an instruction that means nothing in a
// Rust repository. The question belongs to the repository being worked on, so
// it is asked of the repository.
//
// A configured command is taken as given: it can be anything the repository
// runs, and there is no way to check `make check` short of running it. The
// default is checked, because the alternative to finding out now is an agent
// declaring itself done having verified nothing.
func ResolveVerifyCommand(repoPath, configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}

	path := filepath.Join(repoPath, DefaultVerifyCommand)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: %s has no %s, so nothing tells an agent working there how to verify its own work - Fix: add %s to the repository, or set registry.repos[].verifyCommand in kernl.yaml to the command that runs its checks", repoPath, DefaultVerifyCommand, DefaultVerifyCommand)
	}
	if info.IsDir() || info.Mode()&0111 == 0 {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: %s exists but is not an executable file - Fix: `chmod +x %s`, or set registry.repos[].verifyCommand in kernl.yaml", path, path)
	}
	return DefaultVerifyCommand, nil
}
