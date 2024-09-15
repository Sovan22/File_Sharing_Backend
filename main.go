package main

import (
	"context"
	"file_manage/handlers"
	"file_manage/models"
	"file_manage/utils"
	"fmt"
	"log"

	"github.com/go-redis/redis/v8"

	"time"
	"os"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	// "gorm.io/gorm/logger"

	"sync"
)

// Background worker to delete expired share URLs
func backgroundWorker(db *gorm.DB, rdc *redis.Client) {
	for {
		

		var expiredFiles []models.File
		if err := db.Where("public_url_expiry <= ? AND public_url != ?", time.Now(), "").Find(&expiredFiles).Error; err != nil {

			fmt.Println("Error fetching expired URLs:", err)
			continue
		}

		// fmt.Println(expiredFiles)

		if len(expiredFiles) > 0 {
			fmt.Printf("Found %d expired URLs to update\n", len(expiredFiles))
			var wg sync.WaitGroup
			for _, file := range expiredFiles {
				wg.Add(1)
				go func(file models.File) {
					defer wg.Done()

					
					file.PublicUrl = ""
					file.PublicUrlExpiry = time.Time{} 

					if err := db.Save(&file).Error; err != nil {
						fmt.Println("Error updating file URL and expiry:", err)
					}

					cacheKey := fmt.Sprintf("files_user_%v", file.UserID)
					if err := rdc.Del(context.Background(), cacheKey).Err(); err != nil {
						fmt.Println("Error invalidating cache for file:", file.ID, err)
					} else {
						fmt.Printf("Cache invalidated for file ID %d\n", file.ID)
					}

				}(file)
			}
			wg.Wait()
		}

		time.Sleep(time.Minute)
	}
}

func main() {

	
	// logLevel := logger.Silent

	db, err := gorm.Open(sqlite.Open("file_sharing.db"), &gorm.Config{
		// Logger: logger.Default.LogMode(logLevel)
	})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}


	db.AutoMigrate(&models.User{}, &models.File{})

	// Use this if not using Docker
	// os.Setenv("REDIS_URL", "localhost:6379")

	redisURL := os.Getenv("REDIS_URL")

	if redisURL == "" {
		log.Fatal("REDIS_URL is not set")
	}

	rdc := redis.NewClient(&redis.Options{
		Addr: redisURL, 
	})
	

	// Initialize router
	r := gin.Default()
	
	// Initialize handlers
	authHandler := handlers.NewAuthHandler(db)
	fileHandler := handlers.NewFileHandler(db)
	go backgroundWorker(db,rdc)

	
	// Routes
	r.POST("/register", authHandler.Register)
	r.POST("/login", authHandler.Login)
	r.GET("/download/:token", fileHandler.DownloadFile)

	authorized := r.Group("/")
	authorized.Use(authHandler.AuthMiddleware())
	
	authorized.Use(utils.RateLimiterMiddleware(rdc))
	{
		authorized.POST("/upload", fileHandler.Upload)
		authorized.GET("/files", fileHandler.GetFiles)
		authorized.GET("/share/:fileID", fileHandler.ShareFile)
		authorized.GET("/delete/:fileID", fileHandler.DeleteFile)
		authorized.GET("/search", fileHandler.SearchFiles)
	}
	

	r.Run(":8080")
}
