package mqtt

import (
	"encoding/json"
	"fmt"
	"karez-system/config"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Client struct {
	client      mqtt.Client
	topicPrefix string
}

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

type AlertMessage struct {
	Time       time.Time `json:"time"`
	AlertID    int       `json:"alert_id"`
	KarezID    int       `json:"karez_id"`
	AlertType  string    `json:"alert_type"`
	AlertLevel string    `json:"alert_level"`
	Message    string    `json:"message"`
	Value      float64   `json:"value"`
	Threshold  float64   `json:"threshold"`
}

type SensorDataHandler func(msg *SensorMessage)

func New(cfg *config.Config) (*Client, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTTBroker)
	opts.SetClientID(cfg.MQTTClientID)
	if cfg.MQTTUsername != "" {
		opts.SetUsername(cfg.MQTTUsername)
		opts.SetPassword(cfg.MQTTPassword)
	}
	opts.SetAutoReconnect(true)
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v", err)
	})
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("MQTT connected")
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}

	return &Client{
		client:      client,
		topicPrefix: cfg.MQTTTopicPrefix,
	}, nil
}

func (m *Client) Close() {
	m.client.Disconnect(250)
}

func (m *Client) SubscribeSensorData(handler SensorDataHandler) error {
	topic := fmt.Sprintf("%s/sensor/+/+", m.topicPrefix)
	token := m.client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
		var sensorMsg SensorMessage
		if err := json.Unmarshal(msg.Payload(), &sensorMsg); err != nil {
			log.Printf("Failed to parse sensor message: %v", err)
			return
		}
		if sensorMsg.Time.IsZero() {
			sensorMsg.Time = time.Now()
		}
		handler(&sensorMsg)
	})
	token.Wait()
	if token.Error() != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", topic, token.Error())
	}
	log.Printf("Subscribed to MQTT topic: %s", topic)
	return nil
}

func (m *Client) PublishAlert(alert *AlertMessage) error {
	topic := fmt.Sprintf("%s/alert/%d", m.topicPrefix, alert.KarezID)
	payload, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	token := m.client.Publish(topic, 1, false, payload)
	token.Wait()
	if token.Error() != nil {
		return fmt.Errorf("failed to publish alert: %w", token.Error())
	}
	log.Printf("Alert published to %s: %s - %s", topic, alert.AlertType, alert.Message)
	return nil
}

func (m *Client) PublishAllocation(karezID int, data interface{}) error {
	topic := fmt.Sprintf("%s/allocation/%d", m.topicPrefix, karezID)
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	token := m.client.Publish(topic, 1, false, payload)
	token.Wait()
	return token.Error()
}
