//go:build unix

package platform

import "syscall"

func execProcess(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}
