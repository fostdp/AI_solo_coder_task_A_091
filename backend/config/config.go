package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	ServerPort         string
	DatabaseURL        string
	MQTTBroker         string
	MQTTClientID       string
	MQTTUsername       string
	MQTTPassword       string
	MQTTTopicPrefix    string
	SimulationInterval int
	AlertCheckInterval int
	HydraulicParams    HydraulicConfig
	AgricultureDemand  AgricultureConfig
}

type HydraulicConfig struct {
	DefaultChannel           DefaultChannelConfig  `json:"default_channel"`
	SoilPermeability         map[string]SoilConfig `json:"soil_permeability"`
	SedimentationThresholds  SedimentationConfig  `json:"sedimentation_thresholds"`
	Evaporation              EvaporationConfig    `json:"evaporation"`
	Simulation               SimConfig            `json:"simulation"`
}

type DefaultChannelConfig struct {
	Width                 float64 `json:"width"`
	Height                float64 `json:"height"`
	Slope                 float64 `json:"slope"`
	RoughnessCoeff        float64 `json:"roughness_coeff"`
	SeepageCoeff          float64 `json:"seepage_coeff"`
	DefaultSoilType       string  `json:"default_soil_type"`
	DefaultSoilCorrection float64 `json:"default_soil_correction"`
	DefaultTemperature    float64 `json:"default_temperature"`
}

type SoilConfig struct {
	BasePermeability  float64 `json:"base_permeability"`
	CorrectionFactor  float64 `json:"correction_factor"`
	Description       string  `json:"description"`
}

type SedimentationConfig struct {
	LowRiskVelocity    float64 `json:"low_risk_velocity"`
	MediumRiskVelocity float64 `json:"medium_risk_velocity"`
	HighRiskVelocity   float64 `json:"high_risk_velocity"`
	LowRiskScore       float64 `json:"low_risk_score"`
	MediumRiskScore    float64 `json:"medium_risk_score"`
	HighRiskScore      float64 `json:"high_risk_score"`
	CriticalRiskScore  float64 `json:"critical_risk_score"`
}

type EvaporationConfig struct {
	BaseRate          float64 `json:"base_rate"`
	TemperatureCoeff  float64 `json:"temperature_coeff"`
	WindCoeff         float64 `json:"wind_coeff"`
}

type SimConfig struct {
	DefaultInflowRate    float64 `json:"default_inflow_rate"`
	DownstreamInflowGain float64 `json:"downstream_inflow_gain"`
	BisectionTolerance   float64 `json:"bisection_tolerance"`
	MaxIterations        int     `json:"max_iterations"`
}

type AgricultureConfig struct {
	CropTypes           map[string]CropConfig `json:"crop_types"`
	OasisDefaults       OasisDefaultsConfig   `json:"oasis_defaults"`
	AllocationAlgorithm AllocationAlgoConfig  `json:"allocation_algorithm"`
	WaterShortageLevels WaterShortageConfig   `json:"water_shortage_levels"`
}

type CropConfig struct {
	Name                 string  `json:"name"`
	WaterRequirementM3Ha float64 `json:"water_requirement_m3_per_ha_per_day"`
	GrowingSeasonStart   string  `json:"growing_season_start"`
	GrowingSeasonEnd     string  `json:"growing_season_end"`
	Priority             int     `json:"priority"`
}

type OasisDefaultsConfig struct {
	DefaultMinAllocationRatio float64 `json:"default_min_allocation_ratio"`
	DefaultMaxAllocationRatio float64 `json:"default_max_allocation_ratio"`
	ReserveRatio              float64 `json:"reserve_ratio"`
}

type AllocationAlgoConfig struct {
	BigMPenalty        float64 `json:"big_m_penalty"`
	SimplexMaxIter     int     `json:"simplex_max_iterations"`
	Epsilon            float64 `json:"epsilon"`
	PriorityWeightBase float64 `json:"priority_weight_base"`
}

type WaterShortageConfig struct {
	WarningRatio   float64 `json:"warning_ratio"`
	CriticalRatio  float64 `json:"critical_ratio"`
	EmergencyRatio float64 `json:"emergency_ratio"`
}

func Load() *Config {
	cfg := &Config{
		ServerPort:         getEnv("SERVER_PORT", "8080"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/karez?sslmode=disable"),
		MQTTBroker:         getEnv("MQTT_BROKER", "tcp://localhost:1883"),
		MQTTClientID:       getEnv("MQTT_CLIENT_ID", "karez-backend"),
		MQTTUsername:       getEnv("MQTT_USERNAME", ""),
		MQTTPassword:       getEnv("MQTT_PASSWORD", ""),
		MQTTTopicPrefix:    getEnv("MQTT_TOPIC_PREFIX", "karez"),
		SimulationInterval: getEnvInt("SIMULATION_INTERVAL", 3600),
		AlertCheckInterval: getEnvInt("ALERT_CHECK_INTERVAL", 300),
	}

	loadJSONConfig("config/hydraulic_params.json", &cfg.HydraulicParams)
	loadJSONConfig("config/agriculture_demand.json", &cfg.AgricultureDemand)

	return cfg
}

func loadJSONConfig(relPath string, target interface{}) {
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		fullPath := filepath.Join(exeDir, relPath)
		if data, err := os.ReadFile(fullPath); err == nil {
			json.Unmarshal(data, target)
			return
		}
	}

	if data, err := os.ReadFile(relPath); err == nil {
		json.Unmarshal(data, target)
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
