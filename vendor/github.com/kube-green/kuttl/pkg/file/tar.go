package file

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
)

// UntarInPlace untars a .tar file using the same name as the file as the sub-folder name in the folder it was referenced in.
// i.e., /opt/foo.tar will land in /opt/foo
// supports tar and tgz file formats
func UntarInPlace(path string) error {
	folder := TrimExt(path)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	compressed := filepath.Ext(path) == ".tgz"
	return UnTar(folder, file, compressed)
}

// UnTar untars a tar or tgz file into the dest folder.
// dest is the folder location
// io.Reader is a reader that is tar (or compressed tar) format
// compressed is true if reader is a compressed format
func UnTar(dest string, r io.Reader, compressed bool) (err error) {
	if compressed {
		gzr, err := gzip.NewReader(r)
		if err != nil {
			return err
		}
		defer gzr.Close()
		r = gzr
	}
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()

		switch {
		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		target := filepath.Join(dest, header.Name)

		// check the file type
		switch header.Typeflag {
		case tar.TypeDir:
			// we don't need to handle folders, files have folder name in their names and that should be enough
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			if _, err := io.CopyBuffer(f, tr, nil); err != nil {
				return err
			}

			f.Close()
		}
	}
}
