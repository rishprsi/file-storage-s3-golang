package main

import (
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const uploadLimit = 1 << 30
	http.MaxBytesReader(w, r.Body, uploadLimit)

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	bearerToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(bearerToken, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find the user", err)
		return
	}

	metadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't find the video", err)
		return
	}
	if metadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Video doesn't belong to the current user", err)
		return
	}

	file, fileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't load file from the request", err)
		return
	}
	defer file.Close()

	contentType := fileHeader.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		log.Printf("Failed to convert contentType %v", err)
	}
	if mediaType != "video/mp4" {
		log.Printf("Contenttype is : %s", contentType)
		respondWithError(w, http.StatusUnsupportedMediaType, "Media file not of type mp4", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusUnsupportedMediaType, "Couldn't save tempfile", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't copy file contents to file", err)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to move file pointer to start", err)
	}

	processedFilePath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error processing video", err)
		return
	}
	defer os.Remove(processedFilePath)

	processedFile, err := os.Open(processedFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not open processed file", err)
		return
	}
	defer processedFile.Close()
	videoKey, err := createThumbnailName()
	if err != nil {
		videoKey = videoIDString
	}

	videoKey += ".mp4"
	orientation, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		log.Printf("Failed to get aspect ratio of the video %s: %v", tempFile.Name(), err)
	} else {
		videoKey = orientation + "/" + videoKey
	}

	input := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &videoKey,
		Body:        processedFile,
		ContentType: &mediaType,
	}

	output, err := cfg.s3Client.PutObject(r.Context(), &input)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed sending file to s3", err)
		return
	}
	log.Printf("Saved file %v", output)

	videoURL := fmt.Sprintf("https://%s/%s", cfg.s3CfDistribution, videoKey)
	fmt.Printf("Adding video URL: %s", videoURL)

	metadata.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(metadata)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to save video output", err)
		return
	}

	respondWithJSON(w, http.StatusOK, metadata)
}
