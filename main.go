package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ctx         = context.Background()
	redisClient *redis.Client
	port        = "8080"
)

type ViewIncrementRequest struct {
	VideoID string `json:"videoId"`
	UserID  string `json:"userId"`
}

type FeedCacheRequest struct {
	Videos []map[string]interface{} `json:"videos"`
}

type APIResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func initRedis() {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}

	redisClient = redis.NewClient(&redis.Options{
		Addr:         redisURL,
		Password:     "",
		DB:           0,
		PoolSize:     100, // Handle high concurrency
		MinIdleConns: 10,
	})

	_, err := redisClient.Ping(ctx).Result()
	if err != nil {
		log.Printf("⚠️ Redis connection failed: %v", err)
	} else {
		log.Println("✅ Go Video Service connected to Redis")
	}
}

// 1. Atomically increment video view counter
func handleIncrementView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ViewIncrementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.VideoID == "" {
		http.Error(w, "Invalid request: videoId is required", http.StatusBadRequest)
		return
	}

	// Atomic operations in Redis
	pipe := redisClient.Pipeline()
	viewKey := fmt.Sprintf("video:views:%s", req.VideoID)
	pipe.Incr(ctx, viewKey)
	pipe.ZIncrBy(ctx, "videos:trending_views", 1, req.VideoID)

	// HyperLogLog for unique viewers
	if req.UserID != "" {
		uniqueKey := fmt.Sprintf("video:unique_views:%s", req.VideoID)
		pipe.PFAdd(ctx, uniqueKey, req.UserID)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		http.Error(w, "Failed to record view", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{Status: "ok", Message: "View incremented"})
}

// 2. Fetch instant view count
func handleGetViews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Path: /api/v1/views/{id}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 || parts[4] == "" {
		http.Error(w, "Missing video ID", http.StatusBadRequest)
		return
	}
	videoID := parts[4]

	viewKey := fmt.Sprintf("video:views:%s", videoID)
	val, err := redisClient.Get(ctx, viewKey).Result()
	if err == redis.Nil {
		val = "0"
	} else if err != nil {
		http.Error(w, "Failed to read views", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"videoId": videoID,
		"views":   val,
	})
}

// 3. Serve pre-cached high-speed video feed
func handleGetFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cachedFeed, err := redisClient.Get(ctx, "cache:video_feed:hot").Result()
	if err != nil || cachedFeed == "" {
		// Empty or cache miss
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "HIT")
	w.Write([]byte(cachedFeed))
}

// 4. Update the cached feed (called by Node.js or background sync)
func handleCacheFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req FeedCacheRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	bytes, _ := json.Marshal(req.Videos)
	// Cache for 60 seconds
	redisClient.Set(ctx, "cache:video_feed:hot", string(bytes), 60*time.Second)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{Status: "ok", Message: "Feed cached successfully"})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "z-video-service-go",
	})
}

func main() {
	initRedis()

	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/api/v1/views/increment", handleIncrementView)
	http.HandleFunc("/api/v1/views/", handleGetViews)
	http.HandleFunc("/api/v1/feed", handleGetFeed)
	http.HandleFunc("/api/v1/feed/cache", handleCacheFeed)

	log.Printf("⚡ Go Video Service running on http://localhost:%s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
