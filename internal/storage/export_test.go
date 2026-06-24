package storage

type FTPStorage struct {
	BasePath string
}

func NewTestFTPStorage(basePath string) *FTPStorage {
	return &FTPStorage{BasePath: basePath}
}

func (f *FTPStorage) ValidateFilename(filename string) (string, error) {
	s := &ftpStorage{
		host:     "",
		username: "",
		password: "",
		basePath: f.BasePath,
		client:   nil,
	}
	return s.validateFilename(filename)
}

func IsFTPNotFoundError(err error) bool {
	return isFTPNotFoundError(err)
}
