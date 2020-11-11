package nimbus

import (
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
)

const (
	DefaultTransferBuffSize = 256
	Mb10                    = (10 << 20)
)

// NimbusHTTPFormImpl is a NimbusHTTP implementation which handles file uploads
// performed using HTML forms or the FormData web API.
type NimbusHTTPFormImpl struct {
  mu        sync.RWMutex
	maxSize   int64
	tBuffSize int64
	dfk       string
	mimeCache map[string][]string
	tmpDir    string
}

// NNewHTTPFormImpl creates and returns the form implementation of NimbusHTTP.
// The `dfk` argument is the "default file key" which is a string indicating
// the name of the file field in requests to be received. `maxSize` specifies
// the maximum supported file size and `buffSize` indicates the copy buffer
// size. `tmpDir` is the directory in which the uploaded files will be stored
// as temporary files.
func NewHTTPFormImpl(dfk string, maxSize int64, buffSize int64, tmpDir string) (NimbusHTTP, error) {
	// create tmpdir if it doesn't already exist
	_ = os.Mkdir(tmpDir, 0755)
	return &NimbusHTTPFormImpl{
		maxSize:   maxSize,
		tBuffSize: buffSize,
		dfk:       dfk,
		mimeCache: make(map[string][]string),
		tmpDir:    tmpDir,
	}, nil
}

func (n *NimbusHTTPFormImpl) Cleanup() {
	// delete tmpdir (and all contents) created during initialization
	_ = os.RemoveAll(n.tmpDir)
}

// write is a helper which writes the contents of the file `f` to the writer `w`
// in chunks of `buffSize`.
func write(f multipart.File, w io.Writer, buffSize int64) error {
	buff := make([]byte, buffSize)
	for {
		n, err := f.Read(buff)
		if err == io.EOF && n == 0 {
			break
		} else if err != nil {
			return err
		}
		w.Write(buff)
	}
	return nil
}

// Upload defines the endpoint which performs the download to the server.
// A single file is expected, with the form
// It is assumed that files will contain extensions and that the extension is
// the part of the filename from the first character after the last '.' to the
// end of the filename. This substring is required to be non-empty.
func (n *NimbusHTTPFormImpl) Upload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(n.maxSize)
	uploaded, hdr, err := r.FormFile(n.dfk)
	if err != nil {
		http.Error(w, "failed to obtain file from request", http.StatusBadRequest)
		return
	}
	defer uploaded.Close()

	var fExt string
	{
		lastDotIdx := strings.LastIndex(hdr.Filename, ".")
		if lastDotIdx == -1 || lastDotIdx == len(hdr.Filename) {
			http.Error(w, "cannot determine filename extension", http.StatusBadRequest)
			return
		}
		fExt = hdr.Filename[lastDotIdx:]
	}

	tempFile, err := ioutil.TempFile(".nimbus_tmp", fmt.Sprintf("*%s", fExt))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tempFile.Close()

	// read file into transfer buffer and write in chunks to avoid reading the
	// whole file at once
	if err := write(uploaded, tempFile, n.tBuffSize); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// cache hdr for this file so that it can be downloaded with the same hdr
  n.mu.Lock()
	n.mimeCache[tempFile.Name()] = hdr.Header["Content-Type"]
  n.mu.Unlock()
	w.Write([]byte(path.Base(tempFile.Name())))
}

// Download defines the endpoint which writes the first requested file from the
// request queries under the specified "default file key" to the user.
func (n *NimbusHTTPFormImpl) Download(w http.ResponseWriter, r *http.Request) {
	files := r.URL.Query()[n.dfk]
	if len(files) == 0 {
		http.Error(w, "expected file name", http.StatusBadRequest)
		return
	}
	f, err := os.Open(fmt.Sprintf(".nimbus_tmp/%s", files[0]))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// set headers as they were when the file was uploaded (obtain mu for reading)
  n.mu.RLock()
	for _, t := range n.mimeCache[files[0]] {
		r.Header.Add("Content-Type", t)
	}
  n.mu.RUnlock()
	if err := write(f, w, n.tBuffSize); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (n *NimbusHTTPFormImpl) UploadMany(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (n *NimbusHTTPFormImpl) DownloadMany(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
