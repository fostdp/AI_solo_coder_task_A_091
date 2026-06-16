package dtureceiver

import (
	"context"
	"fmt"
	"karez-system/config"
	"karez-system/db"
	"karez-system/models"
	"karez-system/mqtt"
	"log"
	"time"
)

type SensorMessage struct {
	Time            time.Time `json:"time"`
	KarezID         int       `json:"karez_id"`
	SegmentID       int       `json:"segment_id,omitempty"`
	ShaftID         int       `json:"shaft_id,omitempty"`
	SensorType      string    `json:"sensor_type"`
	SensorID        string    `json:"sensor_id"`
	FlowRate        float64   `json:"flow_rate,omitempty"`
	WaterLevel      float64   `json:"water_level,omitempty"`
	ShaftWaterLevel float64   `json:"shaft_water_level,omitempty"`
	Evaporation     float64   `json:"evaporation,omitempty"`
	Temperature     float64   `json:"temperature,omitempty"`
	Turbidity       float64   `json:"turbidity,omitempty"`
	Velocity        float64   `json:"velocity,omitempty"`
}

type DtuReceiver struct {
	cfg        *config.Config
	database   *db.Database
	mqttClient *mqtt.Client
	outputChan chan<- *models.SensorData
	validator  *DataValidator
}

type DataValidator struct {
	flowRateMin    float64
	flowRateMax    float64
	waterLevelMin  float64
	waterLevelMax  float64
	temperatureMin float64
	temperatureMax float64
}

func New(cfg *config.Config, database *db.Database, mqttClient *mqtt.Client, outputChan chan<- *models.SensorData) *DtuReceiver {
	return &DtuReceiver{
		cfg:        cfg,
		database:   database,
		mqttClient: mqttClient,
		outputChan: outputChan,
		validator: &DataValidator{
			flowRateMin:    0,
			flowRateMax:    10.0,
			waterLevelMin:  0,
			waterLevelMax:  10.0,
			temperatureMin: -20,
			temperatureMax: 60,
		},
	}
}

func (r *DtuReceiver) Start(ctx context.Context) error {
	if r.mqttClient != nil {
		err := r.mqttClient.SubscribeSensorData(func(msg *mqtt.SensorMessage) {
			sensorData := &models.SensorData{
				Time:            msg.Time,
				KarezID:         msg.KarezID,
				SegmentID:       msg.SegmentID,
				ShaftID:         msg.ShaftID,
				SensorType:      msg.SensorType,
				SensorID:        msg.SensorID,
				FlowRate:        msg.FlowRate,
				WaterLevel:      msg.WaterLevel,
				ShaftWaterLevel: msg.ShaftWaterLevel,
				Evaporation:     msg.Evaporation,
				Temperature:     msg.Temperature,
				Turbidity:       msg.Turbidity,
				Velocity:        msg.Velocity,
			}
			r.processSensorData(ctx, sensorData)
		})
		if err != nil {
			log.Printf("DTU Receiver: MQTT subscription failed: %v", err)
		} else {
			log.Println("DTU Receiver: MQTT subscription started")
		}
	}

	log.Println("DTU Receiver: started")
	return nil
}

func (r *DtuReceiver) processSensorData(ctx context.Context, data *models.SensorData) {
	if data.Time.IsZero() {
		data.Time = time.Now()
	}

	if err := r.validator.Validate(data); err != nil {
		log.Printf("DTU Receiver: validation failed for sensor %s: %v", data.SensorID, err)
		return
	}

	if err := r.database.InsertSensorData(ctx, data); err != nil {
		log.Printf("DTU Receiver: failed to insert sensor data: %v", err)
	}

	select {
	case r.outputChan <- data:
	default:
		log.Printf("DTU Receiver: output channel full, dropping data from %s", data.SensorID)
	}
}

func (r *DtuReceiver) ReceiveHTTP(data *models.SensorData) error {
	if data.Time.IsZero() {
		data.Time = time.Now()
	}

	if err := r.validator.Validate(data); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	ctx := context.Background()
	if err := r.database.InsertSensorData(ctx, data); err != nil {
		return fmt.Errorf("database insert failed: %w", err)
	}

	select {
	case r.outputChan <- data:
	default:
		log.Printf("DTU Receiver: output channel full, dropping HTTP data from %s", data.SensorID)
	}

	return nil
}

func (v *DataValidator) Validate(data *models.SensorData) error {
	if data.KarezID <= 0 {
		return fmt.Errorf("invalid karez_id: %d", data.KarezID)
	}
	if data.SensorType == "" {
		return fmt.Errorf("sensor_type is required")
	}
	if data.SensorID == "" {
		return fmt.Errorf("sensor_id is required")
	}

	switch data.SensorType {
	case "flow":
		if data.FlowRate < v.flowRateMin || data.FlowRate > v.flowRateMax {
			return fmt.Errorf("flow_rate %.4f out of range [%.2f, %.2f]",
				data.FlowRate, v.flowRateMin, v.flowRateMax)
		}
	case "water_level":
		if data.WaterLevel < v.waterLevelMin || data.WaterLevel > v.waterLevelMax {
			return fmt.Errorf("water_level %.3f out of range [%.2f, %.2f]",
				data.WaterLevel, v.waterLevelMin, v.waterLevelMax)
		}
	case "shaft_water_level":
		if data.ShaftWaterLevel < v.waterLevelMin || data.ShaftWaterLevel > v.waterLevelMax {
			return fmt.Errorf("shaft_water_level %.3f out of range", data.ShaftWaterLevel)
		}
	case "temperature":
		if data.Temperature < v.temperatureMin || data.Temperature > v.temperatureMax {
			return fmt.Errorf("temperature %.1f out of range [%.1f, %.1f]",
				data.Temperature, v.temperatureMin, v.temperatureMax)
		}
	}

	return nil
}
