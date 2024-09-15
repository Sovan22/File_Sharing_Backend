package handlers

import (
	"context"
	"encoding/json"
	"file_manage/models"
	"file_manage/utils"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type FileHandler struct {
	DB *gorm.DB
	SDB *gorm.DB
	Redis *redis.Client
}

func NewFileHandler(db *gorm.DB) *FileHandler {
	sdb, err := gorm.Open(sqlite.Open("shared_files.db"), &gorm.Config{})
	if err != nil {
		fmt.Printf("failed to connect to database: %s", err)
	}

	//Form sharedfiles table
	if err := sdb.AutoMigrate(&SharedFile{}); err != nil {
		fmt.Printf("failed to auto migrate: %s", err)
	}

	redisURL := os.Getenv("REDIS_URL")
	// redisURL = "localhost:6379"
	if redisURL == "" {
		log.Fatal("REDIS_URL is not set")
	}
	rdb := redis.NewClient(&redis.Options{
        Addr:    redisURL,
        Password: "",               
        DB:       0,                
    })
	db.AutoMigrate(&SharedFile{})
	return &FileHandler{
		DB: db,
		SDB: sdb,
		Redis : rdb,
	}
}

func (h *FileHandler) Upload(c *gin.Context) {
	userID, _ := c.Get("userID")

	// Extract all files from the request
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse multipart form"})
		return
	}

	files := form.File["file"]

	uploadDir := "uploads"
	if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
		err = os.MkdirAll(uploadDir, 0755)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create uploads directory"})
			return
		}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var uploadErrors []string 

	for _, file := range files {
		wg.Add(1)
		go func(file *multipart.FileHeader) {
			defer wg.Done()

			src, err := file.Open()
			if err != nil {
				mu.Lock()
				uploadErrors = append(uploadErrors, fmt.Sprintf("Failed to open file %s: %v", file.Filename, err))
				mu.Unlock()
				return
			}
			defer src.Close()

			fileData, err := io.ReadAll(src)
			if err != nil {
				mu.Lock()
				uploadErrors = append(uploadErrors, fmt.Sprintf("Failed to read file %s: %v", file.Filename, err))
				mu.Unlock()
				return
			}

			// Generate unique filename
			filename := uuid.New().String() + filepath.Ext(file.Filename)
			uploadPath := filepath.Join("uploads", filename)

			// Save the file to the uploads folder
			if err := os.WriteFile(uploadPath, fileData, 0644); err != nil {
				mu.Lock()
				uploadErrors = append(uploadErrors, fmt.Sprintf("Failed to save file %s: %v", file.Filename, err))
				mu.Unlock()
				return
			}

			// Save metadata in the database
			fileRecord := models.File{
				Name:   file.Filename,
				Size:   file.Size,
				URL:    uploadPath,
				UserID: userID.(uint),
				Type: utils.ExtractType(file.Filename),
			}
			if err := h.DB.Create(&fileRecord).Error; err != nil {
				mu.Lock()
				uploadErrors = append(uploadErrors, fmt.Sprintf("Failed to save metadata for file %s: %v", file.Filename, err))
				mu.Unlock()
				return
			}
		}(file)
	}

	wg.Wait()

	// Check if there were any errors and return them
	if len(uploadErrors) > 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"errors": uploadErrors})
		return
	}

	cacheKey := fmt.Sprintf("files_user_%v", userID)
    h.Redis.Del(context.Background(), cacheKey)

	c.JSON(http.StatusOK, gin.H{"message": "Files uploaded successfully"})
}


func (h *FileHandler) Upload2(c *gin.Context) {
	userID, _ := c.Get("userID")

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to get file from request"})
		return
	}

	filename := uuid.New().String() + filepath.Ext(file.Filename)
	uploadPath := filepath.Join("uploads", filename)

	fileSaveCh := make(chan error)
	dbSaveCh := make(chan error)

	go func() {
		if err := c.SaveUploadedFile(file, uploadPath); err != nil {
			fileSaveCh <- err
		} else {
			fileSaveCh <- nil
		}
	}()

	// Goroutine to handle database record creation
	go func() {
		fileRecord := models.File{
			Name:   file.Filename,
			Size:   file.Size,
			URL:    uploadPath,
			UserID: userID.(uint),
			Type: utils.ExtractType(file.Filename),
		}
		if err := h.DB.Create(&fileRecord).Error; err != nil {
			dbSaveCh <- err
		} else {
			dbSaveCh <- nil
		}
	}()

	
	fileSaveErr := <-fileSaveCh
	dbSaveErr := <-dbSaveCh

	
	if fileSaveErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}
	if dbSaveErr != nil {
		os.Remove(uploadPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file metadata"})
		return
	}

	cacheKey := fmt.Sprintf("files_user_%v", userID)
    h.Redis.Del(context.Background(), cacheKey)

	c.JSON(http.StatusOK, gin.H{"message": "File uploaded successfully"})
}


func (h *FileHandler) GetFiles(c *gin.Context) {
    userID, _ := c.Get("userID")
    cacheKey := fmt.Sprintf("files_user_%v", userID)
    ctx := context.Background()

    var wg sync.WaitGroup
    var cachedData string
    var dbFiles []models.File
    var cacheErr error
    var dbErr error

    wg.Add(2)

    // Fetch from cache concurrently
    go func() {
        defer wg.Done()
        cachedData, cacheErr = h.Redis.Get(ctx, cacheKey).Result()
    }()

    // Fetch from DB concurrently
    go func() {
        defer wg.Done()
        dbErr = h.DB.Where("user_id = ?", userID).Find(&dbFiles).Error
    }()

    wg.Wait()

    if cacheErr == redis.Nil {
        fmt.Println( "Cache miss - Using DB")
        if dbErr != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve files"})
            return
        }
        jsonData, err := json.Marshal(dbFiles)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize files"})
            return
        }
        h.Redis.Set(ctx, cacheKey, jsonData, 5*time.Minute)
        c.JSON(http.StatusOK, dbFiles)
    } else if cacheErr != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Cache error"})
    } else {
		fmt.Println("Cache hit")
        var files []models.File
        json.Unmarshal([]byte(cachedData), &files)
        c.JSON(http.StatusOK, files)
    }
}


func (h *FileHandler) GetFiles2(c *gin.Context) {
    userID, _ := c.Get("userID")

    // Check cache first
    cacheKey := fmt.Sprintf("files_user_%v", userID)
    ctx := context.Background()
    cachedData, err := h.Redis.Get(ctx, cacheKey).Result()

    if err == redis.Nil {
        fmt.Println("Cache miss, fetch from database")
        var files []models.File
        if err := h.DB.Where("user_id = ?", userID).Find(&files).Error; err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve files"})
            return
        }

        // Serialize the files into JSON and cache the result
        jsonData, err := json.Marshal(files)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize files"})
            return
        }

        h.Redis.Set(ctx, cacheKey, jsonData, 5*time.Minute) // Cache for 5 minutes
        c.JSON(http.StatusOK, files)
    } else if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Cache error"})
    } else {
        fmt.Println("Cache hit, return cached data")
        var files []models.File
        if err := json.Unmarshal([]byte(cachedData), &files); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to deserialize cached files"})
            return
        }
        c.JSON(http.StatusOK, files)
    }
}

func (h *FileHandler) GetFilesOnlySQL(c *gin.Context) {
	userID, _ := c.Get("userID")

	var files []models.File
	if err := h.DB.Where("user_id = ?", userID).Find(&files).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve files"})
		return
	}

	c.JSON(http.StatusOK, files)
}

type SharedFile struct{
	Token string `gorm:"uniqueIndex"`
	FilePath string
	FileName string
	Expires time.Time
}

var sharedFiles = make(map[string]SharedFile) 
var mu sync.Mutex

func (h *FileHandler) ShareFile(c *gin.Context) {
	userID, _ := c.Get("userID")
	
	fileID, err := strconv.Atoi(c.Param("fileID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file ID"})
		return
	}

	expiry := c.Query("expiry")
    if expiry == "" {
        expiry = "24h"
    }

    exp, err := time.ParseDuration(expiry)
	if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiry format"})
        return
    }

	var file models.File
	if err := h.DB.Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}
	if file.PublicUrl != "" {
		c.JSON(http.StatusOK, gin.H{
			"shareURL": "http://" + file.PublicUrl,
			"expires_at": file.PublicUrlExpiry,
		})
		return 
	}

	// shareURL :=  wd + fmt.Sprintf("/download/%s", filepath.Base(file.URL))

	token := uuid.New().String()
	workingDir, err := os.Getwd() 
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	filePath := filepath.Join(workingDir, file.URL)
	expiration:= time.Now().Add(exp)
	// expiration := time.Now().Add(time.Duration(1) * time.Minute)


	sharedFile := SharedFile{
		Token:            token,
		FilePath:         filePath,
		FileName: 		  file.Name,
		Expires:          expiration,
	}

	// Save to database
	var wg sync.WaitGroup
	wg.Add(2)
	errCh := make(chan error, 2)

	// Concurrent database save
	go func() {
		defer wg.Done()
		if err := h.SDB.Create(&sharedFile).Error; err != nil {
			errCh <- fmt.Errorf("failed to create shared file in DB: %w", err)
		}
	}()

	// Concurrent Redis save
	go func() {
		defer wg.Done()
		ctx := context.Background()
		pipe := h.Redis.Pipeline()
		pipe.HSet(ctx, fmt.Sprintf("shared_file:%s", token),
			"file_path", filePath,
			"original_file_name", file.Name,
			"expires", expiration.Unix(),
		)
		pipe.Expire(ctx, fmt.Sprintf("shared_file:%s", token), time.Until(expiration))
		_, err := pipe.Exec(ctx)
		if err != nil {
			errCh <- fmt.Errorf("failed to save to Redis: %w", err)
		}
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		fmt.Printf("Error in ShareFile: %v\n", err)
	}


	// mu.Lock()
	// sharedFiles[token] = SharedFile{
	// 	FilePath: filePath,
	// 	FileName: file.Name,
	// 	Expires:  expiration,
	// }
	// mu.Unlock()

	shareURL := fmt.Sprintf("%s/download/%s",c.Request.Host,token)
	cacheKey := fmt.Sprintf("files_user_%v", userID)
	h.Redis.Del(context.Background(), cacheKey)

	c.JSON(http.StatusOK, gin.H{
		"shareURL": "http://" + shareURL,
		"expires_at": expiration.Format(time.RFC3339),
	})


	go func() {
        file.PublicUrl = shareURL
        file.PublicUrlExpiry = expiration

        if err := h.DB.Save(&file).Error; err != nil {
            fmt.Printf("Error saving file share URL: %v", err)
        }
    }()
}

func (h *FileHandler) DownloadFile(c *gin.Context) {
	token := c.Param("token")

	ctx := context.Background()
	var sharedFile map[string]string
	var dbSharedFile SharedFile
	var wg sync.WaitGroup
	wg.Add(2)

	// Concurrent Redis fetch
	go func() {
		defer wg.Done()
		result, err := h.Redis.HGetAll(ctx, fmt.Sprintf("shared_file:%s", token)).Result()
		if err != nil && err != redis.Nil {
			fmt.Printf("Redis error: %v\n", err)
			return
		}
		if len(result) > 0 {
			sharedFile = result
		}
	}()

	// Concurrent DB fetch
	go func() {
		defer wg.Done()
		if err := h.SDB.Where("token = ?", token).First(&dbSharedFile).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				fmt.Printf("DB error: %v\n", err)
			}
		}
	}()

	wg.Wait()

	// Use DB result if Redis is empty
	if len(sharedFile) == 0 && dbSharedFile != (SharedFile{}) {
		sharedFile = map[string]string{
			"file_path":          dbSharedFile.FilePath,
			"original_file_name": dbSharedFile.FileName,
			"expires":            fmt.Sprintf("%d", dbSharedFile.Expires.Unix()),
		}
		
		// Update Redis asynchronously
		go func() {
			h.Redis.HSet(ctx, fmt.Sprintf("shared_file:%s", token),
				"file_path", dbSharedFile.FilePath,
				"original_file_name", dbSharedFile.FileName,
				"expires", dbSharedFile.Expires.Unix(),
			)
			h.Redis.Expire(ctx, fmt.Sprintf("shared_file:%s", token), time.Until(dbSharedFile.Expires))
		}()
	}

	if len(sharedFile) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Link not found"})
		return
	}

	expiresUnix, _ := strconv.ParseInt(sharedFile["expires"], 10, 64)
	if time.Now().After(time.Unix(expiresUnix, 0)) {
		c.JSON(http.StatusGone, gin.H{"error": "Link has expired"})
		return
	}

	c.FileAttachment(sharedFile["file_path"], sharedFile["original_file_name"])
}

func (h *FileHandler) DownloadFile2(c *gin.Context) {
	token := c.Param("token")


	mu.Lock()
	sharedFile, exists := sharedFiles[token]
	mu.Unlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Link not found"})
		return
	}

	// Check if the link has expired
	if time.Now().After(sharedFile.Expires) {
		c.JSON(http.StatusGone, gin.H{"error": "Link has expired"})
		return
	}

	c.FileAttachment(sharedFile.FilePath, sharedFile.FileName)
}

func (h *FileHandler) DeleteFile(c *gin.Context) {
	userID, _ := c.Get("userID")
	fileID, err := strconv.Atoi(c.Param("fileID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file ID"})
		return
	}

	var file models.File
	err = h.DB.Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found or you don't have permission to delete it"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch file information"})
		}
		return
	}

	// Channels to handle the results of deleting file and DB record
	fileDeleteCh := make(chan error, 1)
	dbDeleteCh := make(chan error, 1)

	// Goroutine to delete the DB record
	go func() {
		dbDeleteCh <- h.DB.Delete(&file).Error
	}()

	// Goroutine to delete the actual file
	go func() {
		workingDir, err := os.Getwd()
		if err != nil {
			fileDeleteCh <- fmt.Errorf("failed to get working directory: %w", err)
			return
		}

		fullPath := filepath.Join(workingDir, file.URL)
		if err := os.Remove(fullPath); err != nil {
			if os.IsNotExist(err) {
				fileDeleteCh <- fmt.Errorf("file does not exist on filesystem: %w", err)
			} else {
				fileDeleteCh <- fmt.Errorf("failed to delete file from filesystem: %w", err)
			}
		} else {
			fileDeleteCh <- nil
		}
	}()

	
	dbErr := <-dbDeleteCh
	fileErr := <-fileDeleteCh

	// Handle errors
	if dbErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file metadata", "details": dbErr.Error()})
		return
	}

	if fileErr != nil {
		
		if strings.Contains(fileErr.Error(), "file does not exist on filesystem") {
			log.Printf("Warning: %v", fileErr)
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file from filesystem", "details": fileErr.Error()})
			return
		}
	}

	// Clear the cache
	cacheKey := fmt.Sprintf("files_user_%v", userID)
	if err := h.Redis.Del(context.Background(), cacheKey).Err(); err != nil {
		log.Printf("Failed to clear cache: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "File deleted successfully"})
}


func (h *FileHandler) DeleteFile2(c *gin.Context) {
	userID, _ := c.Get("userID")
	fileID, err := strconv.Atoi(c.Param("fileID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file ID"})
		return
	}

	// Channels to handle the results of deleting file and DB record
	fileDeleteCh := make(chan error)
	dbDeleteCh := make(chan error)

	// Goroutine to fetch the file from the database
	var file models.File
	
	go func() {
		if err := h.DB.Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error; err != nil {
			dbDeleteCh <- err
			return
		}

		// If file exists in DB, delete the record
		if err := h.DB.Delete(&file).Error; err != nil {
			dbDeleteCh <- err
		} else {
			dbDeleteCh <- nil
		}
	}()
	
	go func() {
		
		workingDir, err := os.Getwd()
		if err != nil {
			fileDeleteCh <- err
			return
		}

		fullPath := filepath.Join(workingDir, file.URL) // Construct the full file path
		fmt.Printf("%s",fullPath)
		if err := os.Remove(fullPath); err != nil {
			fileDeleteCh <- err
		} else {
			fileDeleteCh <- nil
		}
	}()
	
	dbErr := <-dbDeleteCh
	fileErr := <-fileDeleteCh

	// Handle errors
	if dbErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file metadata"})
		return
	}

	if fileErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file from filesystem"})
		return
	}

	cacheKey := fmt.Sprintf("files_user_%v", userID)
    h.Redis.Del(context.Background(), cacheKey)

	// Respond with success
	c.JSON(http.StatusOK, gin.H{"message": "File deleted successfully"})

}

func (h *FileHandler) SearchFiles(c *gin.Context) {
	userID, _ := c.Get("userID")
	fileName := c.Query("name")               // e.g., ?name=report
	fileType := c.Query("type")               // e.g., ?type=pdf
	uploadedDate := c.Query("date")  		  // e.g., ?uploaded_date=2023-09-14
	limitStr := c.Query("limit")              // Limit the number of results
	offsetStr := c.Query("offset")            // Offset for pagination
	
	// Default pagination values
	limit := 10
	offset := 0

	// Parse pagination parameters if provided
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			limit = parsedLimit
		}
	}
	if offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil {
			offset = parsedOffset
		}
	}

	
	query := h.DB.Model(&models.File{})

	// Add filters to the query based on the parameters provided
	if fileName != "" {
		query = query.Where("name LIKE ? AND user_id = ?", "%"+fileName+"%",userID)
	}
	if fileType != "" {
		query = query.Where("type = ? AND user_id = ?", fileType,userID)
	}
	if uploadedDate != "" {
        // Check for a valid date format
        parsedDate, err := time.Parse("2006-01-02", uploadedDate)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format. Expected YYYY-MM-DD"})
            return
        }

        query = query.Where("DATE(created_at) = ? AND user_id = ?", parsedDate.Format("2006-01-02"), userID)
    }

	// Apply pagination
	query = query.Limit(limit).Offset(offset)

	// Execute the query and fetch the results
	var files []models.File
	if err := query.Find(&files).Error; err != nil {
		fmt.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search files"})
		return
	}

	c.JSON(http.StatusOK, files)
}


