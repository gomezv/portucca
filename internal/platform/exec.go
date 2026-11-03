package platform

type Executor struct{}

func (Executor) Exec(path string, argv, env []string) error {
	return execProcess(path, argv, env)
}
