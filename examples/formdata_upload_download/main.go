package main

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/navaz-alani/nimbus"
)

func main() {
	// HTTP Form file server, with 10mb max file size & 256 byte copy buffer
	impl, _ := nimbus.NewHTTPFormImpl("_file_",
		nimbus.Mb10,
		nimbus.DefaultTransferBuffSize,
		".nimbus_tmp")
	defer impl.Cleanup()

	m := mux.NewRouter()
	addr := "localhost:5000"
	Configure(impl, m)
	// avoid cors and serve index.html file from same server
	m.Handle("/", http.FileServer(http.Dir("./examples/formdata_upload_download")))

	log.Printf("Attempting to bind to: %s", addr)
	log.Fatalln(http.ListenAndServe(addr, m))
}

func Configure(n nimbus.NimbusHTTP, m *mux.Router) {
	// configure routes as required
	m.HandleFunc("/upload", n.Upload)
	m.HandleFunc("/download", n.Download)
	m.HandleFunc("/upload-many", n.UploadMany)
	m.HandleFunc("/download-many", n.DownloadMany)
	m.HandleFunc("/delete", n.Delete)
}
