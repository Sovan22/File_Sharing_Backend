package utils

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

func RateLimiterMiddleware(redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID from context after authentication
		userID, exists := c.Get("userID")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			c.Abort()
			return
		}

		ctx := context.Background()
		limitKey := fmt.Sprintf("rate_limit_%v", userID)
		limit, err := redisClient.Get(ctx, limitKey).Int()

		if err == redis.Nil {
			// Key does not exist, initialize it with 1 request and set expiration for 1 minute
			err = redisClient.Set(ctx, limitKey, 1, time.Minute).Err()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set rate limit"})
				c.Abort()
				return
			}
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check rate limit"})
			c.Abort()
			return
		} else {
			// Key exists, check if the limit has been reached
			if limit >= 100 {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": "Rate limit exceeded. Try again in a minute.",
				})
				c.Abort()
				return
			}

			// Increment the count of requests
			err = redisClient.Incr(ctx, limitKey).Err()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to increment rate limit"})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}