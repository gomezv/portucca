//go:build windows

package platform

func execProcess(string, []string, []string) error {
	return ErrUnsupported
}
