# Nimbus

This is a simple fileserver which facilitates uploading and downloading of
files, written in Go. For examples of how this software works, check out the
`examples` directory in the project root.

## Implementation Details

There is an interface type `NimbusHTTP` which defines the HTTP interface of the
file server. There is currently one implementation of this:

* `NimbusHTTPFormImpl` is the implementation which attempts to download a file
  from the user and save it to the server, through a request with type
  `multipart/form-data`. The server returns a string representing the server
  name for the uploaded file. Using this name as a query parameter (with the key
  as a configurable value), the user can then download the uploaded files.
