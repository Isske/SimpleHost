package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Server struct {
	storagePath string
	files       map[string]time.Time
	mu          sync.Mutex
}

func NewServer(storagePath string) *Server {
	return &Server{
		storagePath: storagePath,
		files:       make(map[string]time.Time),
	}
}

func (s *Server) uploadPageHandler(w http.ResponseWriter) {
	html := `
		<!DOCTYPE html>
		<html lang="en">
		<head>
			<meta charset="UTF-8">
			<meta name="viewport" content="width=device-width, initial-scale=1.0">
			<title>Upload File</title>
			 <link href="https://fonts.googleapis.com/css2?family=Courier+Prime:ital,wght@0,400;0,700;1,400;1,700&display=swap"
        rel="stylesheet">
		</head>
		<style>
			body {
				font-family: 'Courier Prime', sans-serif;
				background-color: Canvas;
				color: CanvasText;
				color-scheme: light dark;
			}
		</style>
		<body>
			<h1>Upload File</h1>
			<p>Max Upload Size : 10MB
			<form enctype="multipart/form-data" action="/upload" method="post">
				<input type="file" name="file" required>
				<input type="submit" value="Upload">
			</form>
		</body>
		</html>
	`

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

func (s *Server) uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(10 << 20) // 10 MB max file size
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Unable to get the file from form", http.StatusBadRequest)
		return
	}
	defer file.Close()

	fileExtension := filepath.Ext(handler.Filename)
	newFileName := fmt.Sprintf("%s-%d%s", "simplehost", time.Now().Unix(), fileExtension)
	filePath := filepath.Join(s.storagePath, newFileName)

	out, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Unable to create the file on server", http.StatusInternalServerError)
		return
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		http.Error(w, "Unable to save the file", http.StatusInternalServerError)
		return
	}

	expiryDuration := 60 * time.Minute
	s.mu.Lock()
	s.files[newFileName] = time.Now().Add(expiryDuration)
	s.mu.Unlock()

	time.AfterFunc(expiryDuration, func() {
		s.deleteFile(newFileName)
	})

	downloadLink := fmt.Sprintf("/download?file=%s", newFileName)
	htmlResponse := fmt.Sprintf(`
		<!DOCTYPE html>
		<html lang="en">
		<head>
			<meta charset="UTF-8">
			<meta name="viewport" content="width=device-width, initial-scale=1.0">
			<title>File Uploaded</title>
			<link href="https://fonts.googleapis.com/css2?family=Courier+Prime:ital,wght@0,400;0,700;1,400;1,700&display=swap"
        rel="stylesheet">
		</head>
		<style>
			body {
				font-family: 'Courier Prime', sans-serif;
				background-color: Canvas;
				color: CanvasText;
				color-scheme: light dark;
			}
		</style>
		<body>
			<h1>File Uploaded Successfully</h1>
			<p>Your file has been uploaded and renamed to <strong>%s</strong>.</p>
			<p>File links are only valid for 1 hour.
			<p><a href="%s">Download</a></p>
		</body>
		</html>
	`, newFileName, downloadLink)

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(htmlResponse))
}

func (s *Server) downloadHandler(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if fileName == "" {
		http.Error(w, "File name is required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	expiryTime, exists := s.files[fileName]
	s.mu.Unlock()

	if !exists || time.Now().After(expiryTime) {
		http.Error(w, "File not found or expired", http.StatusNotFound)
		return
	}

	filePath := filepath.Join(s.storagePath, fileName)

	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	contentType := "application/octet-stream"
	fileStat, err := file.Stat()
	if err == nil {
		contentType = http.DetectContentType(make([]byte, fileStat.Size()))
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileStat.Size()))

	http.ServeFile(w, r, filePath)
}

func (s *Server) deleteFile(fileName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := filepath.Join(s.storagePath, fileName)
	err := os.Remove(filePath)
	if err != nil {
		fmt.Printf("Error deleting file: %s\n", err)
		return
	}

	delete(s.files, fileName)
	fmt.Printf("File deleted: %s\n", fileName)
}

func main() {
	storagePath := "./uploads"
	os.MkdirAll(storagePath, os.ModePerm)

	server := NewServer(storagePath)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		server.uploadPageHandler(w)
	})

	http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			http.Error(w, "Route Doesn't Exist :/", http.StatusNotFound)
		} else if r.Method == "POST" {
			server.uploadHandler(w, r)
		}
	})

	http.HandleFunc("/download", server.downloadHandler)

	fmt.Println("Server is running on port 8080")
	http.ListenAndServe(":8080", nil)
}
