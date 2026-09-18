package signing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestMiddleware(t *testing.T, config SignatureMiddlewareConfig) (*gin.Engine, *KeyManager, *NonceTracker, *Signer) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	logger, _ := zap.NewDevelopment()
	if config.Logger == nil {
		config.Logger = logger
	}

	router.Use(SignatureMiddleware(config))
	router.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	return router, config.KeyManager, config.NonceTracker, config.Signer
}

func createSignedRequest(t *testing.T, method, path string, body []byte, signingKey string, keyID uuid.UUID) *http.Request {
	timestamp := time.Now().Unix()
	nonce, err := GenerateNonce()
	require.NoError(t, err)

	signer := NewSigner(AlgorithmHMACSHA256)
	signature, err := signer.SignRequest(signingKey, method, path, body, timestamp, nonce)
	require.NoError(t, err)

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set(HeaderSignature, signature)
	req.Header.Set(HeaderTimestamp, fmt.Sprintf("%d", timestamp))
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderKeyID, keyID.String())
	req.Header.Set("Content-Type", "application/json")

	return req
}

func TestSignatureMiddleware_MissingHeaders(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	redisClient, mr := setupTestRedis(t)
	defer mr.Close()
	defer redisClient.Close()

	logger, _ := zap.NewDevelopment()
	config := SignatureMiddlewareConfig{
		KeyManager:   NewKeyManager(db),
		NonceTracker: NewNonceTracker(redisClient, 5*time.Minute),
		Logger:       logger,
	}

	router, _, _, _ := setupTestMiddleware(t, config)

	tests := []struct {
		name           string
		setupRequest   func() *http.Request
		expectedStatus int
		expectedError  string
	}{
		{
			name: "missing signature",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("POST", "/test", nil)
				req.Header.Set(HeaderTimestamp, fmt.Sprintf("%d", time.Now().Unix()))
				req.Header.Set(HeaderNonce, "test-nonce")
				req.Header.Set(HeaderKeyID, uuid.New().String())
				return req
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "missing_signature",
		},
		{
			name: "missing timestamp",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("POST", "/test", nil)
				req.Header.Set(HeaderSignature, "test-signature")
				req.Header.Set(HeaderNonce, "test-nonce")
				req.Header.Set(HeaderKeyID, uuid.New().String())
				return req
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "missing_timestamp",
		},
		{
			name: "missing nonce",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("POST", "/test", nil)
				req.Header.Set(HeaderSignature, "test-signature")
				req.Header.Set(HeaderTimestamp, fmt.Sprintf("%d", time.Now().Unix()))
				req.Header.Set(HeaderKeyID, uuid.New().String())
				return req
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "missing_nonce",
		},
		{
			name: "missing key ID",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("POST", "/test", nil)
				req.Header.Set(HeaderSignature, "test-signature")
				req.Header.Set(HeaderTimestamp, fmt.Sprintf("%d", time.Now().Unix()))
				req.Header.Set(HeaderNonce, "test-nonce")
				return req
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "missing_key_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedError, response["error"])
		})
	}
}

func TestSignatureMiddleware_InvalidTimestamp(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	redisClient, mr := setupTestRedis(t)
	defer mr.Close()
	defer redisClient.Close()

	logger, _ := zap.NewDevelopment()
	config := SignatureMiddlewareConfig{
		KeyManager:    NewKeyManager(db),
		NonceTracker:  NewNonceTracker(redisClient, 5*time.Minute),
		MaxAgeSeconds: 300,
		Logger:        logger,
	}

	router, _, _, _ := setupTestMiddleware(t, config)

	t.Run("invalid timestamp format", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", nil)
		req.Header.Set(HeaderSignature, "test-signature")
		req.Header.Set(HeaderTimestamp, "invalid")
		req.Header.Set(HeaderNonce, "test-nonce")
		req.Header.Set(HeaderKeyID, uuid.New().String())

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "invalid_timestamp", response["error"])
	})

	t.Run("timestamp too old", func(t *testing.T) {
		oldTimestamp := time.Now().Unix() - 400 // 400 seconds ago

		req := httptest.NewRequest("POST", "/test", nil)
		req.Header.Set(HeaderSignature, "test-signature")
		req.Header.Set(HeaderTimestamp, fmt.Sprintf("%d", oldTimestamp))
		req.Header.Set(HeaderNonce, "test-nonce")
		req.Header.Set(HeaderKeyID, uuid.New().String())

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "invalid_timestamp", response["error"])
	})

	t.Run("timestamp in future", func(t *testing.T) {
		futureTimestamp := time.Now().Unix() + 60 // 60 seconds in future

		req := httptest.NewRequest("POST", "/test", nil)
		req.Header.Set(HeaderSignature, "test-signature")
		req.Header.Set(HeaderTimestamp, fmt.Sprintf("%d", futureTimestamp))
		req.Header.Set(HeaderNonce, "test-nonce")
		req.Header.Set(HeaderKeyID, uuid.New().String())

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestSignatureMiddleware_SkipPaths(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	redisClient, mr := setupTestRedis(t)
	defer mr.Close()
	defer redisClient.Close()

	logger, _ := zap.NewDevelopment()
	config := SignatureMiddlewareConfig{
		KeyManager:   NewKeyManager(db),
		NonceTracker: NewNonceTracker(redisClient, 5*time.Minute),
		Logger:       logger,
		SkipPaths:    []string{"/health", "/public"},
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SignatureMiddleware(config))

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/public/info", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"info": "public"})
	})
	router.POST("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	t.Run("skip health endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("skip public endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/public/info", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("protected endpoint requires signature", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/protected", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestSignatureMiddleware_ContextValues(t *testing.T) {
	db, mock := setupTestDB(t)
	defer db.Close()

	redisClient, mr := setupTestRedis(t)
	defer mr.Close()
	defer redisClient.Close()

	logger, _ := zap.NewDevelopment()
	keyManager := NewKeyManager(db)
	config := SignatureMiddlewareConfig{
		KeyManager:   keyManager,
		NonceTracker: NewNonceTracker(redisClient, 5*time.Minute),
		Logger:       logger,
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SignatureMiddleware(config))

	var capturedKeyID uuid.UUID
	var capturedUserID uuid.UUID

	router.POST("/test", func(c *gin.Context) {
		keyID, exists := c.Get("signing_key_id")
		if exists {
			capturedKeyID = keyID.(uuid.UUID)
		}

		userID, exists := c.Get("signing_key_user_id")
		if exists {
			capturedUserID = userID.(uuid.UUID)
		}

		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	keyID := uuid.New()
	userID := uuid.New()
	keyValue, err := GenerateSigningKey()
	require.NoError(t, err)
	keyHash := hashKey(keyValue)

	// Mock key lookup
	mock.ExpectQuery("SELECT (.+) FROM signing_keys").
		WithArgs(keyID).
		WillReturnRows(
			sqlmock.NewRows([]string{"id", "user_id", "key_hash", "name", "algorithm", "is_active", "last_used", "created_at", "expires_at"}).
				AddRow(keyID, userID, keyHash, "Test Key", AlgorithmHMACSHA256, true, nil, time.Now(), nil),
		)

	// Mock last used update
	mock.ExpectExec("UPDATE signing_keys").
		WithArgs(keyID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := []byte(`{"test":"data"}`)
	timestamp := time.Now().Unix()
	nonce, err := GenerateNonce()
	require.NoError(t, err)

	signer := NewSigner(AlgorithmHMACSHA256)
	signature, err := signer.SignRequest(keyHash, "POST", "/test", body, timestamp, nonce)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))
	req.Header.Set(HeaderSignature, signature)
	req.Header.Set(HeaderTimestamp, fmt.Sprintf("%d", timestamp))
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderKeyID, keyID.String())

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, keyID, capturedKeyID)
	assert.Equal(t, userID, capturedUserID)
}
