package disk

import (
	"io/fs"
	"os"
)

// credentialOperations keeps failed filesystem calls reproducible without
// changing the production staging location or granting tests a container engine.
type credentialOperations struct {
	openRoot func(string) (*os.Root, error)
	openFile func(*os.Root, string, int, fs.FileMode) (*os.File, error)
	stat     func(*os.File) (fs.FileInfo, error)
	names    func(*os.File, int) ([]string, error)
}

func (s Credentials) operations() credentialOperations {
	if s.ops != nil {
		return *s.ops
	}
	return credentialOperations{os.OpenRoot, (*os.Root).OpenFile, (*os.File).Stat, (*os.File).Readdirnames}
}
