package ui

import "github.com/jackjakarta/anchr/s3client"

type BucketSelectedMsg struct {
	Index int
}

type ObjectsLoadedMsg struct {
	Result *s3client.ListResult
	Err    error
}

type NavigateMsg struct {
	Prefix string
}
