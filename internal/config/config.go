package config

import (
	"log"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v2"
)

type Config struct {
	SQSQueueURL        string
	SlackWebhookURL    string
	SlackBotToken      string
	SlackSigningSecret string
	ServerPort         string
	PollIntervalSec    int
	SlackChannels      map[string]string
	AlarmChannels      map[string]string
}

type AlarmChannelConfig struct {
	AlarmMappings    map[string]string `yaml:"alarm_mappings"`
	DefaultChannels  map[string]string `yaml:"default_channels"`
}

func LoadConfig() *Config {
	sqsURL := os.Getenv("SQS_QUEUE_URL")
	slackURL := os.Getenv("SLACK_WEBHOOK_URL")
	slackBotToken := os.Getenv("SLACK_BOT_TOKEN")
	slackSigningSecret := os.Getenv("SLACK_SIGNING_SECRET")
	serverPort := os.Getenv("SERVER_PORT")

	if sqsURL == "" {
		log.Fatal("Missing required env var: SQS_QUEUE_URL")
	}
	if slackBotToken == "" {
		log.Fatal("Missing required env var: SLACK_BOT_TOKEN")
	}
	if slackSigningSecret == "" {
		log.Fatal("Missing required env var: SLACK_SIGNING_SECRET")
	}
	if serverPort == "" {
		serverPort = "8088"
	}

	pollIntervalStr := os.Getenv("POLL_INTERVAL_SEC")
	pollInterval := 10
	if pollIntervalStr != "" {
		if val, err := strconv.Atoi(pollIntervalStr); err == nil {
			pollInterval = val
		}
	}

	// Configure channels for different priorities
	channels := map[string]string{
		"P0":      getEnvOrDefault("SLACK_CHANNEL_P0", "#p0-infra-alerts"),
		"P1":      getEnvOrDefault("SLACK_CHANNEL_P1", "#p1-infra-alerts"),
		"P2":      getEnvOrDefault("SLACK_CHANNEL_P2", "#p2-infra-alerts"),
		"default": getEnvOrDefault("SLACK_CHANNEL_DEFAULT", "#alerts"),
	}

	// Load alarm-to-channel mappings
	alarmChannels := loadAlarmChannelMappings()

	return &Config{
		SQSQueueURL:        sqsURL,
		SlackWebhookURL:    slackURL,
		SlackBotToken:      slackBotToken,
		SlackSigningSecret: slackSigningSecret,
		ServerPort:         serverPort,
		PollIntervalSec:    pollInterval,
		SlackChannels:      channels,
		AlarmChannels:      alarmChannels,
	}
}

func loadAlarmChannelMappings() map[string]string {
	configPath := getEnvOrDefault("CONFIG_PATH", "/etc/config")
	alarmConfigFile := filepath.Join(configPath, "alarm-channels.yaml")
	
	// Check if file exists
	if _, err := os.Stat(alarmConfigFile); os.IsNotExist(err) {
		log.Printf("Alarm channel config file not found at %s, using defaults", alarmConfigFile)
		return make(map[string]string)
	}

	// Read the YAML file
	data, err := os.ReadFile(alarmConfigFile)
	if err != nil {
		log.Printf("Failed to read alarm channel config: %v", err)
		return make(map[string]string)
	}

	// Parse YAML
	var config AlarmChannelConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Printf("Failed to parse alarm channel config: %v", err)
		return make(map[string]string)
	}

	log.Printf("Loaded %d alarm-to-channel mappings", len(config.AlarmMappings))
	return config.AlarmMappings
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
