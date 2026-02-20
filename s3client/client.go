package s3client

import (
	"context"
	"strings"

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
		MaxKeys:   aws.Int32(1000),
	}

	output, err := c.s3.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, err
	}

	result := &ListResult{
		Prefix: prefix,
		Bucket: c.bucket,
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
		})
	}

	return result, nil
}
