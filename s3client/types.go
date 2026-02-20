package s3client

import "time"

type S3Item struct {
	Key          string
	Name         string
	IsDir        bool
	Size         int64
	LastModified time.Time
}

type ListResult struct {
	Items  []S3Item
	Prefix string
	Bucket string
}
