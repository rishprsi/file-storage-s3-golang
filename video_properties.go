package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

func getVideoAspectRatio(filePath string) (string, error) {
	command := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	buffer := bytes.Buffer{}
	command.Stdout = &buffer
	err := command.Run()
	if err != nil {
		return "", err
	}

	output := struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}{}
	err = json.Unmarshal(buffer.Bytes(), &output)
	if err != nil {
		return "", err
	}
	log.Printf("The output is: $=%v", output)

	floatRatio := float64(output.Streams[0].Width) / float64(output.Streams[0].Height)
	ratio := ""
	log.Printf("My ratio is %v and the ration for horizontal %v, and for vertical %v", floatRatio, float64(16.0)/float64(9.0), float64(9.0)/float64(16.0))
	if math.Abs(floatRatio-float64(16.0)/float64(9.0)) < .1 {
		ratio = "landscape"
	} else if math.Abs(floatRatio-float64(9)/float64(16)) < .1 {
		ratio = "portrait"
	} else {
		ratio = "other"
	}
	return ratio, nil
}

func processVideoForFastStart(filePath string) (string, error) {
	outputPath := filePath + ".processing"

	command := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputPath)
	buffer := bytes.Buffer{}
	command.Stdout = &buffer
	err := command.Run()
	if err != nil {
		log.Printf("Failed to run the command ffmpeg with the error: %v", err)
		return "", err
	}
	return outputPath, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	presignedClient := s3.NewPresignClient(s3Client)
	objectInput := s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}
	object, err := presignedClient.PresignGetObject(context.Background(), &objectInput, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", fmt.Errorf("failed to create s3 key:%v", err)
	}
	return object.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	if video.VideoURL == nil {
		log.Println("No Video URL present")
		return video, nil
	}
	urlDetails := strings.Split(*video.VideoURL, ",")
	if len(urlDetails) < 2 {
		return video, nil
	}
	presignedURL, err := generatePresignedURL(cfg.s3Client, urlDetails[0], urlDetails[1], time.Hour)
	if err != nil {
		return video, err
	}

	video.VideoURL = &presignedURL
	return video, nil
}
