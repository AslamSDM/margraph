package config

import (
	"os"
	"gopkg.in/yaml.v3"
)

type Config struct {
	App struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	} `yaml:"app"`
	Scraping struct {
		SearchDepth    int `yaml:"search_depth"`
		BranchingLimit int `yaml:"branching_limit"`
		Timeout        int `yaml:"request_timeout"`
	} `yaml:"scraping"`
	Simulation struct {
		ShockImpact    float64 `yaml:"shock_health_impact"`
		SentimentScale float64 `yaml:"sentiment_scale"`
	} `yaml:"simulation"`
	News struct {
		RSSUrl       string `yaml:"rss_url"`
		PollInterval int    `yaml:"poll_interval"`
	} `yaml:"news"`
	Market struct {
		PollInterval int `yaml:"poll_interval"`
	} `yaml:"market"`
	Server struct {
		Port string `yaml:"port"`
	} `yaml:"server"`
	Logging struct {
		Level        string `yaml:"level"`
		EnableColors bool   `yaml:"enable_colors"`
	} `yaml:"logging"`
}

var Global Config

// Load reads the config.yaml file.
func Load() error {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, &Global)
}
