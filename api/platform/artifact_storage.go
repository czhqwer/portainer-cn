package platform

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const artifactStorageListLimit int32 = 100

// ArtifactStorageConnection 只在请求执行期间承载解密后的凭据；调用方不得将该对象持久化、记录日志或返回前端。
type ArtifactStorageConnection struct {
	Endpoint      string
	Region        string
	Bucket        string
	AccessKey     string
	SecretKey     string
	SkipTLSVerify bool
}

type ArtifactStorageObject struct {
	Key  string
	Size int64
}

// ArtifactStorageAdapter 隔离 S3 协议调用，使 handler 可以在不依赖真实 MinIO 的情况下验证授权、超时和清理边界。
type ArtifactStorageAdapter interface {
	Test(context.Context, ArtifactStorageConnection) error
	ListObjects(context.Context, ArtifactStorageConnection, string) ([]ArtifactStorageObject, error)
	HeadObject(context.Context, ArtifactStorageConnection, string) (ArtifactStorageObject, error)
	DownloadObject(context.Context, ArtifactStorageConnection, string) (io.ReadCloser, error)
}

type S3CompatibleArtifactStorageAdapter struct{}

func NewS3CompatibleArtifactStorageAdapter() *S3CompatibleArtifactStorageAdapter {
	return &S3CompatibleArtifactStorageAdapter{}
}

func (adapter *S3CompatibleArtifactStorageAdapter) Test(ctx context.Context, connection ArtifactStorageConnection) error {
	_, err := adapter.client(connection).HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(connection.Bucket)})
	return err
}

func (adapter *S3CompatibleArtifactStorageAdapter) ListObjects(ctx context.Context, connection ArtifactStorageConnection, prefix string) ([]ArtifactStorageObject, error) {
	result, err := adapter.client(connection).ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(connection.Bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(artifactStorageListLimit),
	})
	if err != nil {
		return nil, err
	}

	objects := make([]ArtifactStorageObject, 0, len(result.Contents))
	for _, object := range result.Contents {
		if object.Key == nil || object.Size == nil {
			continue
		}
		objects = append(objects, ArtifactStorageObject{Key: *object.Key, Size: *object.Size})
	}

	return objects, nil
}

func (adapter *S3CompatibleArtifactStorageAdapter) HeadObject(ctx context.Context, connection ArtifactStorageConnection, key string) (ArtifactStorageObject, error) {
	result, err := adapter.client(connection).HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(connection.Bucket), Key: aws.String(key)})
	if err != nil {
		return ArtifactStorageObject{}, err
	}
	if result.ContentLength == nil {
		return ArtifactStorageObject{}, fmt.Errorf("S3 object content length is unavailable")
	}

	return ArtifactStorageObject{Key: key, Size: *result.ContentLength}, nil
}

func (adapter *S3CompatibleArtifactStorageAdapter) DownloadObject(ctx context.Context, connection ArtifactStorageConnection, key string) (io.ReadCloser, error) {
	result, err := adapter.client(connection).GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(connection.Bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}

	return result.Body, nil
}

func (adapter *S3CompatibleArtifactStorageAdapter) client(connection ArtifactStorageConnection) *s3.Client {
	region := connection.Region
	if region == "" {
		region = "us-east-1"
	}

	config := aws.Config{
		Region:      region,
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(connection.AccessKey, connection.SecretKey, "")),
	}
	if connection.SkipTLSVerify {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		// 仅允许管理员在受管 endpoint 上显式关闭证书校验，避免把该例外扩大到其它 HTTP 客户端。
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- controlled administrator-only artifact storage setting
		config.HTTPClient = &http.Client{Transport: transport}
	}

	return s3.NewFromConfig(config, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(connection.Endpoint)
		// MinIO 与多数私有 S3 兼容实现要求 path-style，固定该选项可避免 bucket 被拼成外部域名。
		options.UsePathStyle = true
	})
}
