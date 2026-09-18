package search

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// CacheService handles search result caching
type CacheService struct {
	client *redis.Client
	logger *zap.Logger
	prefix string
	ttl    time.Duration
}

// NewCacheService creates a new search cache service
func NewCacheService(client *redis.Client, logger *zap.Logger) *CacheService {
	return &CacheService{
		client: client,
		logger: logger,
		prefix: "donelist:search:",
		ttl:    10 * time.Minute, // Default TTL for search results
	}
}

// GetSearchResults retrieves cached search results
func (cs *CacheService) GetSearchResults(ctx context.Context, userID uuid.UUID, filters SearchFilters) (*SearchResponse, error) {
	key := cs.makeSearchKey(userID, filters)

	val, err := cs.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			cs.logger.Debug("Search cache miss", zap.String("key", key))
			return nil, nil // Cache miss
		}
		cs.logger.Error("Search cache get error", zap.String("key", key), zap.Error(err))
		return nil, err
	}

	var response SearchResponse
	if err := json.Unmarshal([]byte(val), &response); err != nil {
		cs.logger.Error("Search cache unmarshal error", zap.Error(err))
		return nil, err
	}

	cs.logger.Debug("Search cache hit", zap.String("key", key))
	return &response, nil
}

// SetSearchResults caches search results
func (cs *CacheService) SetSearchResults(ctx context.Context, userID uuid.UUID, filters SearchFilters, response *SearchResponse) error {
	key := cs.makeSearchKey(userID, filters)

	data, err := json.Marshal(response)
	if err != nil {
		cs.logger.Error("Search cache marshal error", zap.Error(err))
		return err
	}

	if err := cs.client.Set(ctx, key, data, cs.ttl).Err(); err != nil {
		cs.logger.Error("Search cache set error", zap.String("key", key), zap.Error(err))
		return err
	}

	cs.logger.Debug("Search cached", zap.String("key", key), zap.Duration("ttl", cs.ttl))
	return nil
}

// InvalidateUserSearches invalidates all search caches for a user
func (cs *CacheService) InvalidateUserSearches(ctx context.Context, userID uuid.UUID) error {
	pattern := cs.prefix + userID.String() + ":*"

	iter := cs.client.Scan(ctx, 0, pattern, 0).Iterator()
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}

	if err := iter.Err(); err != nil {
		cs.logger.Error("Search cache scan error", zap.String("pattern", pattern), zap.Error(err))
		return err
	}

	if len(keys) > 0 {
		if err := cs.client.Del(ctx, keys...).Err(); err != nil {
			cs.logger.Error("Search cache delete error", zap.Int("count", len(keys)), zap.Error(err))
			return err
		}
		cs.logger.Info("Invalidated user search caches", zap.String("user_id", userID.String()), zap.Int("count", len(keys)))
	}

	return nil
}

// makeSearchKey creates a consistent cache key from user ID and filters
func (cs *CacheService) makeSearchKey(userID uuid.UUID, filters SearchFilters) string {
	// Create a hash of the filters to ensure consistent key generation
	data, _ := json.Marshal(filters)
	hash := md5.Sum(data)
	return fmt.Sprintf("%s%s:%x", cs.prefix, userID.String(), hash)
}

// GetFacets retrieves cached facets
func (cs *CacheService) GetFacets(ctx context.Context, userID uuid.UUID, filters SearchFilters) (*SearchFacets, error) {
	key := cs.makeFacetsKey(userID, filters)

	val, err := cs.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Cache miss
		}
		return nil, err
	}

	var facets SearchFacets
	if err := json.Unmarshal([]byte(val), &facets); err != nil {
		return nil, err
	}

	return &facets, nil
}

// SetFacets caches facets
func (cs *CacheService) SetFacets(ctx context.Context, userID uuid.UUID, filters SearchFilters, facets *SearchFacets) error {
	key := cs.makeFacetsKey(userID, filters)

	data, err := json.Marshal(facets)
	if err != nil {
		return err
	}

	return cs.client.Set(ctx, key, data, cs.ttl).Err()
}

// makeFacetsKey creates a cache key for facets
func (cs *CacheService) makeFacetsKey(userID uuid.UUID, filters SearchFilters) string {
	// Facets depend on filters but not pagination
	filtersWithoutPagination := filters
	filtersWithoutPagination.Limit = 0
	filtersWithoutPagination.Offset = 0

	data, _ := json.Marshal(filtersWithoutPagination)
	hash := md5.Sum(data)
	return fmt.Sprintf("%sfacets:%s:%x", cs.prefix, userID.String(), hash)
}
