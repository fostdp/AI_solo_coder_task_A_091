package config

import (
	"os"
	"strconv"
)

type Config struct {
	ServerPort    string
	DatabaseURL   string
	MQTTBroker    string
	MQTTClientID  string
	MQTTUsername  string
	MQTTPassword  string
	MQTTTopicPrefix string
	SimulationInterval int
	AlertCheckInterval  int
}

func Load() *Config {
	return &Config{
		ServerPort:    getEnv("SERVER_PORT", "8080"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/karez?sslmode=disable"),
		MQTTBroker:    getEnv("MQTT_BROKER", "tcp://localhost:1883"),
		MQTTClientID:  getEnv("MQTT_CLIENT_ID", "karez-backend"),
		MQTTUsername:  getEnv("MQTT_USERNAME", ""),
		MQTTPassword:  getEnv("MQTT_PASSWORD", ""),
		MQTTTopicPrefix: getEnv("MQTT_TOPIC_PREFIX", "karez"),
		SimulationInterval: getEnvInt("SIMULATION_INTERVAL", 3600),
		AlertCheckInterval:  getEnvInt("ALERT_CHECK_INTERVAL", 300),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
