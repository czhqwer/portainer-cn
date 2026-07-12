package platform

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	portainer "github.com/portainer/portainer/api"
)

// DatabaseResourceProbe 将数据库驱动细节隔离在平台服务层，handler 只持有授权、密文
// 和审计边界。调用方不得记录 password，也不得把底层驱动错误返回给浏览器。
type DatabaseResourceProbe interface {
	Probe(context.Context, portainer.PlatformDatabaseResource, string) error
}

type directDatabaseResourceProbe struct{}

func NewDirectDatabaseResourceProbe() DatabaseResourceProbe {
	return directDatabaseResourceProbe{}
}

func (directDatabaseResourceProbe) Probe(ctx context.Context, resource portainer.PlatformDatabaseResource, password string) error {
	switch resource.Type {
	case portainer.PlatformDatabaseTypeMySQL, portainer.PlatformDatabaseTypeMariaDB:
		config := mysql.NewConfig()
		config.User = resource.Username
		config.Passwd = password
		config.Net = "tcp"
		config.Addr = net.JoinHostPort(resource.Host, strconv.Itoa(resource.Port))
		config.DBName = resource.Database
		config.Timeout = time.Duration(resource.ConnectionTimeoutSeconds) * time.Second
		db, err := sql.Open("mysql", config.FormatDSN())
		if err != nil {
			return err
		}
		defer db.Close()
		return db.PingContext(ctx)
	case portainer.PlatformDatabaseTypePostgres:
		connectionURL := url.URL{Scheme: "postgres", Host: net.JoinHostPort(resource.Host, strconv.Itoa(resource.Port)), Path: resource.Database}
		if resource.Username != "" {
			connectionURL.User = url.UserPassword(resource.Username, password)
		}
		query := connectionURL.Query()
		query.Set("sslmode", "disable")
		query.Set("connect_timeout", strconv.Itoa(resource.ConnectionTimeoutSeconds))
		connectionURL.RawQuery = query.Encode()
		db, err := sql.Open("postgres", connectionURL.String())
		if err != nil {
			return err
		}
		defer db.Close()
		return db.PingContext(ctx)
	case portainer.PlatformDatabaseTypeRedis:
		client := redis.NewClient(&redis.Options{Addr: net.JoinHostPort(resource.Host, strconv.Itoa(resource.Port)), Username: resource.Username, Password: password, DB: redisDatabaseIndex(resource.Database)})
		defer client.Close()
		return client.Ping(ctx).Err()
	default:
		return fmt.Errorf("unsupported database type")
	}
}

func redisDatabaseIndex(database string) int {
	value, err := strconv.Atoi(database)
	if err != nil || value < 0 {
		return 0
	}
	return value
}
