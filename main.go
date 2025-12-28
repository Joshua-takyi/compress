package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Response data structure
type CompressResponse struct {
	Success        bool    `json:"success"`
	Message        string  `json:"message"`
	OriginalSize   int64   `json:"originalSize"`
	CompressedSize int64   `json:"compressedSize"`
	Savings        float64 `json:"savings"`
	DownloadURL    string  `json:"downloadUrl"`
	Format         string  `json:"format"`
}

func main() {
	if err := loadenv(); err != nil {
		log.Fatal(err)
	}

	// Initialize directories
	for _, dir := range []string{"uploads", "compressed"} {
		os.MkdirAll(dir, 0755)
	}

	mux := http.NewServeMux()

	// API endpoint with CORS
	mux.Handle("/api/compress", enableCORS(http.HandlerFunc(handleCompress)))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CompressResponse{
			Success: true,
			Message: "Welcome to Squeeze API",
		})
	})

	// File server for downloads with CORS
	mux.Handle("/download/", enableCORS(http.StripPrefix("/download/", http.FileServer(http.Dir("compressed")))))

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 20 * time.Second,
	}

	log.Println("Squeeze API starting on :8080...")
	log.Fatal(server.ListenAndServe())
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowedOrigin := "http://localhost:4321"

		if isProduction() {
			if envOrigin := os.Getenv("ALLOWED_ORIGIN"); envOrigin != "" {
				allowedOrigin = envOrigin
			}
		}

		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Vary", "Origin")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleCompress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Parse Uploaded File
	r.ParseMultipartForm(10 << 20)
	file, header, err := r.FormFile("image")
	if err != nil {
		jsonResponse(w, CompressResponse{Success: false, Message: "Failed to upload image"}, http.StatusBadRequest)
		return
	}
	defer file.Close()

	qualityStr := r.FormValue("quality")
	quality, _ := strconv.Atoi(qualityStr)
	if quality < 1 {
		quality = 75
	}

	// 2. Decode Image
	img, format, err := image.Decode(file)
	if err != nil {
		jsonResponse(w, CompressResponse{Success: false, Message: fmt.Sprintf("Unsupported image format: %v", err)}, http.StatusBadRequest)
		return
	}

	// 3. Prepare Output Path
	outFilename := fmt.Sprintf("%d.jpg", time.Now().UnixNano())
	outPath := filepath.Join("compressed", outFilename)
	outFile, err := os.Create(outPath)
	if err != nil {
		jsonResponse(w, CompressResponse{Success: false, Message: "Failed to create output file"}, http.StatusInternalServerError)
		return
	}
	defer outFile.Close()

	// 4. Compress
	err = jpeg.Encode(outFile, img, &jpeg.Options{Quality: quality})
	if err != nil {
		jsonResponse(w, CompressResponse{Success: false, Message: "Compression failed"}, http.StatusInternalServerError)
		return
	}

	// 5. Calculate Stats
	compressedInfo, _ := os.Stat(outPath)
	compressedSize := compressedInfo.Size()
	savings := 100 - (float64(compressedSize) / float64(header.Size) * 100)

	// 6. Return JSON
	jsonResponse(w, CompressResponse{
		Success:        true,
		OriginalSize:   header.Size,
		CompressedSize: compressedSize,
		Savings:        savings,
		DownloadURL:    fmt.Sprintf("%s/download/%s", os.Getenv("ALLOWED_ORIGIN"), outFilename),
		Format:         format,
	}, http.StatusOK)
}

func jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func init() {
	image.RegisterFormat("jpeg", "\xff\xd8", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "\x89PNG\r\n\x1a\n", png.Decode, png.DecodeConfig)
}

func loadenv() error {
	_ = godotenv.Load(".env")
	return nil
}

func isProduction() bool {
	return os.Getenv("NODE_ENV") == "production"
}
