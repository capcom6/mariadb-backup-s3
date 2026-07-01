package scheduler

func LoadState(filePath string) (*State, error) {
	return loadState(filePath)
}
