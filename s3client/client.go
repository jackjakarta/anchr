package s3client

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	bucketcfg "github.com/jackjakarta/anchr/config"
)

type Client struct {
	s3     *s3.Client
	bucket string
	prefix string
}

func NewClient(cfg bucketcfg.BucketConfig) (*Client, error) {
	var opts []func(*config.LoadOptions) error

	opts = append(opts, config.WithRegion(cfg.Region))

	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, err
	}

	var s3Opts []func(*s3.Options)
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}
	if cfg.PathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)

	return &Client{
		s3:     client,
		bucket: cfg.Bucket,
		prefix: cfg.Prefix,
	}, nil
}

func (c *Client) InitialPrefix() string {
	return c.prefix
}

func (c *Client) ListObjects(ctx context.Context, prefix string) (*ListResult, error) {
	input := &s3.ListObjectsV2Input{
		Bucket:    aws.String(c.bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
		MaxKeys:   aws.Int32(1000), // per-page size; the paginator fetches every page
	}

	result := &ListResult{
		Prefix: prefix,
		Bucket: c.bucket,
	}

	paginator := s3.NewListObjectsV2Paginator(c.s3, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, cp := range output.CommonPrefixes {
			name := strings.TrimPrefix(aws.ToString(cp.Prefix), prefix)
			result.Items = append(result.Items, S3Item{
				Key:   aws.ToString(cp.Prefix),
				Name:  name,
				IsDir: true,
			})
		}

		for _, obj := range output.Contents {
			key := aws.ToString(obj.Key)
			if key == prefix {
				continue // skip the prefix itself
			}
			name := strings.TrimPrefix(key, prefix)
			result.Items = append(result.Items, S3Item{
				Key:          key,
				Name:         name,
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified),
				ETag:         strings.Trim(aws.ToString(obj.ETag), `"`), // S3 wraps the ETag in quotes
				StorageClass: string(obj.StorageClass),
			})
		}
	}

	return result, nil
}

// DownloadObject streams the object at key to destPath on the local filesystem.
func (c *Client) DownloadObject(ctx context.Context, key, destPath string) error {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	defer out.Body.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, out.Body)
	return err
}

// PreviewObject fetches up to maxBytes of the object at key via a ranged GET.
// It returns the raw bytes and the object's Content-Type. The read is also
// capped with io.LimitReader so S3-compatible stores that ignore the Range
// header still can't stream the whole object.
func (c *Client) PreviewObject(ctx context.Context, key string, maxBytes int64) ([]byte, string, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Range:  aws.String(fmt.Sprintf("bytes=0-%d", maxBytes-1)),
	})
	if err != nil {
		return nil, "", err
	}
	defer out.Body.Close()

	body, err := io.ReadAll(io.LimitReader(out.Body, maxBytes))
	if err != nil {
		return nil, "", err
	}
	return body, aws.ToString(out.ContentType), nil
}

// PresignGetObject returns a presigned GET URL for key, valid for expiry.
func (c *Client) PresignGetObject(ctx context.Context, key string, expiry time.Duration) (string, error) {
	presign := s3.NewPresignClient(c.s3)
	req, err := presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
