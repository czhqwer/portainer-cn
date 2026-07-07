package endpoints

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/response"
	"github.com/redis/go-redis/v9"
)

const (
	defaultRedisScanCount = int64(50)
	maxRedisPreviewItems  = int64(100)
)

type databaseRedisKeyScanResponse struct {
	Cursor string             `json:"Cursor"`
	Keys   []databaseRedisKey `json:"Keys"`
}

type databaseRedisKey struct {
	Name string `json:"Name"`
	Type string `json:"Type"`
	TTL  string `json:"TTL"`
}

type databaseRedisKeyDetailsResponse struct {
	Name  string              `json:"Name"`
	Type  string              `json:"Type"`
	TTL   string              `json:"TTL"`
	Rows  []map[string]string `json:"Rows"`
	Value string              `json:"Value,omitempty"`
}

func (handler *Handler) databaseConnectionRedisKeys(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	endpointID, httpErr := handler.databaseEndpointIDFromRequest(r)
	if httpErr != nil {
		return httpErr
	}

	connection, httpErr := handler.databaseConnectionFromRequest(r, endpointID)
	if httpErr != nil {
		return httpErr
	}
	if connection.Type != "redis" {
		return httperror.BadRequest("Redis key browsing is only supported for Redis connections", errors.New("unsupported database type"))
	}

	cursor, _ := strconv.ParseUint(r.URL.Query().Get("cursor"), 10, 64)
	count := normalizedRedisScanCount(r.URL.Query().Get("count"))
	pattern := strings.TrimSpace(r.URL.Query().Get("pattern"))
	if pattern == "" {
		pattern = "*"
	}

	dbIndex, err := redisDatabaseIndex(r.URL.Query().Get("database"), connection.Database)
	if err != nil {
		return httperror.BadRequest("Invalid Redis database", err)
	}

	result, err := scanRedisKeys(r, *connection, dbIndex, cursor, pattern, count)
	if err != nil {
		return writeDatabaseError(w, "Unable to scan Redis keys", err)
	}

	return response.JSON(w, result)
}

func (handler *Handler) databaseConnectionRedisKeyDetails(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	endpointID, httpErr := handler.databaseEndpointIDFromRequest(r)
	if httpErr != nil {
		return httpErr
	}

	connection, httpErr := handler.databaseConnectionFromRequest(r, endpointID)
	if httpErr != nil {
		return httpErr
	}
	if connection.Type != "redis" {
		return httperror.BadRequest("Redis key details are only supported for Redis connections", errors.New("unsupported database type"))
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		return httperror.BadRequest("Invalid Redis key request", errors.New("key is required"))
	}

	dbIndex, err := redisDatabaseIndex(r.URL.Query().Get("database"), connection.Database)
	if err != nil {
		return httperror.BadRequest("Invalid Redis database", err)
	}

	result, err := redisKeyDetails(r, *connection, dbIndex, key)
	if err != nil {
		return writeDatabaseError(w, "Unable to retrieve Redis key details", err)
	}

	return response.JSON(w, result)
}

func scanRedisKeys(r *http.Request, connection portainer.DatabaseConnection, dbIndex int, cursor uint64, pattern string, count int64) (*databaseRedisKeyScanResponse, error) {
	client := newRedisClient(connection, dbIndex)
	defer client.Close()

	keys, nextCursor, err := client.Scan(r.Context(), cursor, pattern, count).Result()
	if err != nil {
		return nil, err
	}

	result := &databaseRedisKeyScanResponse{
		Cursor: strconv.FormatUint(nextCursor, 10),
		Keys:   make([]databaseRedisKey, 0, len(keys)),
	}

	for _, key := range keys {
		keyType, err := client.Type(r.Context(), key).Result()
		if err != nil {
			return nil, err
		}
		ttl, err := client.TTL(r.Context(), key).Result()
		if err != nil {
			return nil, err
		}
		result.Keys = append(result.Keys, databaseRedisKey{
			Name: key,
			Type: keyType,
			TTL:  redisTTLLabel(ttl),
		})
	}

	return result, nil
}

// Redis Key 浏览必须使用 SCAN/分页读取；详情预览也限制数量，避免自用场景误触发全量 keyspace 扫描。
func redisKeyDetails(r *http.Request, connection portainer.DatabaseConnection, dbIndex int, key string) (*databaseRedisKeyDetailsResponse, error) {
	client := newRedisClient(connection, dbIndex)
	defer client.Close()

	keyType, err := client.Type(r.Context(), key).Result()
	if err != nil {
		return nil, err
	}
	ttl, err := client.TTL(r.Context(), key).Result()
	if err != nil {
		return nil, err
	}

	result := &databaseRedisKeyDetailsResponse{
		Name: key,
		Type: keyType,
		TTL:  redisTTLLabel(ttl),
		Rows: []map[string]string{},
	}

	switch keyType {
	case "string":
		value, err := client.Get(r.Context(), key).Result()
		if err != nil {
			return nil, err
		}
		result.Value = value
		result.Rows = append(result.Rows, map[string]string{"Field": "value", "Value": value})
	case "list":
		values, err := client.LRange(r.Context(), key, 0, maxRedisPreviewItems-1).Result()
		if err != nil {
			return nil, err
		}
		for index, value := range values {
			result.Rows = append(result.Rows, map[string]string{"Index": strconv.Itoa(index), "Value": value})
		}
	case "hash":
		values, err := client.HGetAll(r.Context(), key).Result()
		if err != nil {
			return nil, err
		}
		count := int64(0)
		for field, value := range values {
			if count >= maxRedisPreviewItems {
				break
			}
			result.Rows = append(result.Rows, map[string]string{"Field": field, "Value": value})
			count++
		}
	case "set":
		values, _, err := client.SScan(r.Context(), key, 0, "*", maxRedisPreviewItems).Result()
		if err != nil {
			return nil, err
		}
		for index, value := range values {
			result.Rows = append(result.Rows, map[string]string{"Index": strconv.Itoa(index), "Value": value})
		}
	case "zset":
		values, err := client.ZRangeWithScores(r.Context(), key, 0, maxRedisPreviewItems-1).Result()
		if err != nil {
			return nil, err
		}
		for index, value := range values {
			result.Rows = append(result.Rows, map[string]string{
				"Index": strconv.Itoa(index),
				"Score": databaseValueToString(value.Score),
				"Value": databaseValueToString(value.Member),
			})
		}
	}

	return result, nil
}

func newRedisClient(connection portainer.DatabaseConnection, dbIndex int) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(connection.Host, strconv.Itoa(connection.Port)),
		Username: connection.Username,
		Password: connection.Password,
		DB:       dbIndex,
	})
}

func redisDatabaseIndex(requestDatabase string, connectionDatabase string) (int, error) {
	database := strings.TrimSpace(requestDatabase)
	if database == "" {
		database = strings.TrimSpace(connectionDatabase)
	}
	if database == "" {
		return 0, nil
	}

	return strconv.Atoi(database)
}

func normalizedRedisScanCount(rawCount string) int64 {
	count, err := strconv.ParseInt(rawCount, 10, 64)
	if err != nil || count <= 0 {
		return defaultRedisScanCount
	}
	if count > maxRedisPreviewItems {
		return maxRedisPreviewItems
	}

	return count
}

func redisTTLLabel(ttl time.Duration) string {
	switch ttl {
	case -2 * time.Nanosecond:
		return "Missing"
	case -1 * time.Nanosecond:
		return "No expiration"
	default:
		if ttl < 0 {
			return ttl.String()
		}
		return ttl.String()
	}
}
