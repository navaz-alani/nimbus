package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/gorilla/mux"
	"github.com/navaz-alani/nimbus"
)

func main() {
	// HTTP Form file server, with 10mb max file size & 256 byte copy buffer,
	// allowing image files with and not files without extensions.
	impl, _ := nimbus.NewHTTPFormImpl("_file_",
		nimbus.Mb10,
		nimbus.DefaultTransferBuffSize,
		"examples/formdata_upload_download/.nimbus_tmp",
		nimbus.ExtImg, false)
	// handle ctrl+c and cleanup since defered cleanup call won't be run
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			log.Println("interrupt: cleaning up...")
			impl.Cleanup()
			os.Exit(0)
		}
	}()
	defer impl.Cleanup()

	m := mux.NewRouter()
	addr := "localhost:5000"
	Configure(impl, m)
	// avoid cors and serve index.html file from same server
	m.Handle("/", http.FileServer(http.Dir("./examples/formdata_upload_download")))

	log.Printf("Attempting to bind to: %s", addr)
	if err := http.ListenAndServe(addr, m); err != nil {
		log.Printf("Server ended with error: %s", err.Error())
	}
}

func Configure(n nimbus.NimbusHTTP, m *mux.Router) {
	// configure routes as required
	m.HandleFunc("/upload", n.Upload)
	m.HandleFunc("/download", n.Download)
	m.HandleFunc("/upload-many", n.UploadMany)
	m.HandleFunc("/download-many", n.DownloadMany)
	m.HandleFunc("/delete", n.Delete)
}
