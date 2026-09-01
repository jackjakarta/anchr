package s3client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	bucketcfg "github.com/jackjakarta/anchr/config"
)

// testClient points a Client at a local HTTP server standing in for S3. The
// handler only has to answer GetObject; requests are signed but never verified.
func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c, err := NewClient(bucketcfg.BucketConfig{
		Bucket:    "test-bucket",
		Region:    "us-east-1",
		Endpoint:  srv.URL,
		PathStyle: true,
		AccessKey: "AKIAEXAMPLE",
		SecretKey: "secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %s", err)
	}
	return c
}

func TestDownloadObjectProgressAndRename(t *testing.T) {
	body := make([]byte, 300*1024) // a few io.Copy chunks
	for i := range body {
		body[i] = byte(i)
	}
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})

	dest := filepath.Join(t.TempDir(), "obj.bin")

	var mu sync.Mutex
	var samples [][2]int64
	err := c.DownloadObject(context.Background(), "obj.bin", dest, func(written, total int64) {
		mu.Lock()
		defer mu.Unlock()
		samples = append(samples, [2]int64{written, total})
	})
	if err != nil {
		t.Fatalf("DownloadObject: %s", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading destination: %s", err)
	}
	if len(got) != len(body) {
		t.Errorf("downloaded %d bytes, want %d", len(got), len(body))
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Errorf("%s.part survived a successful download", dest)
	}

	if len(samples) < 2 {
		t.Fatalf("got %d progress samples, want the initial one plus per-write updates", len(samples))
	}
	if samples[0] != [2]int64{0, int64(len(body))} {
		t.Errorf("first sample = %v, want {0, %d} before any byte is written", samples[0], len(body))
	}
	var prev int64
	for i, s := range samples {
		if s[0] < prev {
			t.Fatalf("sample %d went backwards: %d after %d", i, s[0], prev)
		}
		prev = s[0]
		if s[1] != int64(len(body)) {
			t.Errorf("sample %d total = %d, want %d", i, s[1], len(body))
		}
	}
	if prev != int64(len(body)) {
		t.Errorf("final sample = %d bytes, want %d", prev, len(body))
	}
}

func TestDownloadObjectErrorLeavesNoFiles(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	})

	dest := filepath.Join(t.TempDir(), "obj.bin")
	if err := c.DownloadObject(context.Background(), "obj.bin", dest, nil); err == nil {
		t.Fatal("DownloadObject on a 404 returned no error")
	}
	for _, p := range []string{dest, dest + ".part"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s exists after a failed download", p)
		}
	}
}

func TestDownloadObjectCancelRemovesPartial(t *testing.T) {
	release := make(chan struct{})
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(1<<20))
		w.WriteHeader(http.StatusOK)
		w.Write(make([]byte, 64*1024))
		w.(http.Flusher).Flush()
		<-release // stall mid-body until the test cancels
	})
	t.Cleanup(func() { close(release) })

	ctx, cancel := context.WithCancel(context.Background())
	dest := filepath.Join(t.TempDir(), "obj.bin")

	errCh := make(chan error, 1)
	go func() { errCh <- c.DownloadObject(ctx, "obj.bin", dest, nil) }()

	time.Sleep(100 * time.Millisecond) // let the body start streaming
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("cancelled DownloadObject returned no error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Logf("cancellation surfaced as %v", err) // wrapped differently per transport
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled DownloadObject never returned")
	}

	for _, p := range []string{dest, dest + ".part"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s exists after a cancelled download", p)
		}
	}
}
