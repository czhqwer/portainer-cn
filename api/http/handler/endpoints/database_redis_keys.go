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

func scanRedisKeys(r *http.Request, connection portainer.DatabaseConnection, dbIndex int, cursor uint64, pattern string, count int64) (*databaseRedisKeyScanResponse, error) {
	client := newRedisClient(connection, dbIndex)
	defer client.Close()

	// 单次接口内最多收集 100 个 Key，避免前端翻页累加导致 keyspace 过载。
	result := &databaseRedisKeyScanResponse{
		Cursor: "0",
		Keys:   make([]databaseRedisKey, 0, maxRedisPreviewItems),
	}
	scanCursor := cursor
	scanCount := count
	if scanCount > maxRedisPreviewItems {
		scanCount = maxRedisPreviewItems
	}

	for len(result.Keys) < int(maxRedisPreviewItems) {
		keys, nextCursor, err := client.Scan(r.Context(), scanCursor, pattern, scanCount).Result()
		if err != nil {
			return nil, err
		}

		for _, key := range keys {
			if len(result.Keys) >= int(maxRedisPreviewItems) {
				break
			}
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

		scanCursor = nextCursor
		if nextCursor == 0 || len(result.Keys) >= int(maxRedisPreviewItems) {
			break
		}
	}

	// 达到上限后即使 SCAN 未结束也返回 0，前端不再提供继续翻页入口。
	result.Cursor = "0"
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
