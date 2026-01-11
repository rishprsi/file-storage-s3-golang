package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	const maxMemory = 10 << 20
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, 500, "Failed to parse Multipart data", err)
	}

	file, fileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse from file", err)
		return
	}
	defer file.Close()

	metadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Video metadata not found", err)
		return
	}
	if metadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Video not owned by the user", err)
		return
	}

	contentType := fileHeader.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusUnsupportedMediaType, "Unsopported media type", err)
		return
	}
	contentType = strings.Split(contentType, "/")[1]
	fileKey, err := createThumbnailName()
	if err != nil {
		fileKey = videoIDString
	}
	fileName := fmt.Sprintf("%s.%s", fileKey, contentType)

	dataURL := filepath.Join(cfg.assetsRoot, fileName)
	storedFile, err := os.Create(dataURL)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create file for thumbnail", err)
		return
	}
	_, err = io.Copy(storedFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create file for thumbnail", err)
		return
	}
	dataURL = "http://localhost:" + cfg.port + "/" + dataURL
	metadata.ThumbnailURL = &dataURL
	fmt.Printf("URL being stored is: %v, and data url is %v", metadata.ThumbnailURL, dataURL)

	err = cfg.db.UpdateVideo(metadata)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to update thumbnail", err)
		return
	}

	respondWithJSON(w, http.StatusOK, metadata)
}

func createThumbnailName() (string, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		return "", err
	}

	stringKey := base64.RawURLEncoding.EncodeToString(key)
	return stringKey, nil
}
