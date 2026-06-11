package fileutils

import (
	"errors"
	"io/fs"
	"log"
	"os"

	"github.com/spf13/afero"
)

// ReadDirSafe reads directory entries, gracefully skipping files that
// cannot be stat-ed (e.g. Windows system files on WSL 9p mounts).
// On such filesystems, Readdir(-1) fails for the entire directory when
// even one file's lstat returns EACCES. ReadDir returns DirEntry values
// lazily, letting us skip problematic entries individually.
//
// It resolves the real path and uses os.ReadDir directly — afero wraps
// *os.File in BasePathFile, so a plain type assertion misses it.
func ReadDirSafe(afs afero.Fs, path string) ([]os.FileInfo, error) {
	// Try to get the real path for direct os.ReadDir usage.
	if rp, ok := afs.(interface{ RealPath(name string) (string, error) }); ok {
		realPath, err := rp.RealPath(path)
		if err == nil {
			entries, err := os.ReadDir(realPath)
			if err != nil {
				return nil, err
			}
			var result []os.FileInfo
			for _, e := range entries {
				info, err := e.Info()
				if err != nil {
					log.Printf("fileutils: skipping %s: %v", e.Name(), err)
					continue
				}
				result = append(result, info)
			}
			return result, nil
		}
	}

	f, err := afs.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if osFile, ok := f.(*os.File); ok {
		entries, err := osFile.ReadDir(-1)
		if err != nil {
			return nil, err
		}
		var result []os.FileInfo
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				log.Printf("fileutils: skipping %s: %v", e.Name(), err)
				continue
			}
			result = append(result, info)
		}
		return result, nil
	}

	if bpf, ok := f.(*afero.BasePathFile); ok {
		if osFile, ok := bpf.File.(*os.File); ok {
			entries, err := osFile.ReadDir(-1)
			if err != nil {
				return nil, err
			}
			var result []os.FileInfo
			for _, e := range entries {
				info, err := e.Info()
				if err != nil {
					log.Printf("fileutils: skipping %s: %v", e.Name(), err)
					continue
				}
				result = append(result, info)
			}
			return result, nil
		}
	}

	return f.Readdir(-1)
}

// CopyDir copies a directory from source to dest and all
// of its sub-directories. It doesn't stop if it finds an error
// during the copy. Returns an error if any.
func CopyDir(afs afero.Fs, source, dest string, fileMode, dirMode fs.FileMode) error {
	// Get properties of source.
	srcinfo, err := afs.Stat(source)
	if err != nil {
		return err
	}

	// Create the destination directory.
	err = afs.MkdirAll(dest, srcinfo.Mode())
	if err != nil {
		return err
	}

	obs, err := ReadDirSafe(afs, source)
	if err != nil {
		return err
	}

	var errs []error

	for _, obj := range obs {
		fsource := source + "/" + obj.Name()
		fdest := dest + "/" + obj.Name()

		if obj.IsDir() {
			// Create sub-directories, recursively.
			err = CopyDir(afs, fsource, fdest, fileMode, dirMode)
			if err != nil {
				errs = append(errs, err)
			}
		} else {
			// Perform the file copy.
			err = CopyFile(afs, fsource, fdest, fileMode, dirMode)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	var errString string
	for _, err := range errs {
		errString += err.Error() + "\n"
	}

	if errString != "" {
		return errors.New(errString)
	}

	return nil
}
