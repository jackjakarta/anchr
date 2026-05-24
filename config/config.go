package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type BucketConfig struct {
	Name      string `yaml:"name"`
	Bucket    string `yaml:"bucket"`
	Region    string `yaml:"region"`
	Endpoint  string `yaml:"endpoint,omitempty"`
	AccessKey string `yaml:"access_key,omitempty"`
	SecretKey string `yaml:"secret_key,omitempty"`
	PathStyle bool   `yaml:"path_style,omitempty"`
	Prefix    string `yaml:"prefix,omitempty"`
}

type Config struct {
	Buckets []BucketConfig `yaml:"buckets"`
}

const exampleConfig = `# S3 TUI Configuration
buckets:
  - name: "my-bucket"
    bucket: "my-bucket"
    region: "us-east-1"
    # endpoint: "https://minio.internal:9000"  # omit for AWS
    # access_key: "..."                         # omit for default credentials
    # secret_key: "..."
    # path_style: true                          # required for MinIO
    # prefix: "images/"                         # optional starting prefix
`

func Load(path string) (*Config, error) {
	if path == "" {
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("cannot determine home directory: %w", err)
			}
			configDir = filepath.Join(home, ".config")
		}
		path = filepath.Join(configDir, "anchr", "config.yaml")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s\n\nCreate it with:\n\nmkdir -p %s\ncat > %s << 'EOF'\n%sEOF",
				path, filepath.Dir(path), path, exampleConfig)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if len(cfg.Buckets) == 0 {
		return nil, fmt.Errorf("no buckets configured in %s", path)
	}

	for i, b := range cfg.Buckets {
		if b.Bucket == "" {
			return nil, fmt.Errorf("bucket[%d]: 'bucket' field is required", i)
		}
		if b.Name == "" {
			cfg.Buckets[i].Name = b.Bucket
		}
		if b.Region == "" {
			cfg.Buckets[i].Region = "us-east-1"
		}
	}

	return &cfg, nil
}
