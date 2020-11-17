package nimbus

import (
	"archive/zip"
	"io"
	"os"
	"path"
)

type Zipper struct {
	z *zip.Writer
}

func NewZipper(buff io.Writer) *Zipper {
	return &Zipper{
		z: zip.NewWriter(buff),
	}
}

func (z *Zipper) AddFile(filename string) error {
	f, err := os.OpenFile(filename, os.O_RDONLY, 0444)
	if err != nil {
		return err
	}
	defer f.Close()
	if writer, err := z.z.Create(path.Base(filename)); err != nil {
		return err
	} else {
		_, err := io.Copy(writer, f)
		return err
	}
}

func (z *Zipper) Close() error {
	return z.z.Close()
}
