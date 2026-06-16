package alert

import (
	"context"
	"fmt"
	"karez-system/db"
	"karez-system/models"
	"karez-system/mqtt"
	"karez-system/optimization"
	"karez-system/simulation"
	"log"
	"time"
)

type AlertManager struct {
	database   *db.Database
	mqttClient *mqtt.Client
	simulator  *simulation.HydraulicSimulator
	allocator  *optimization.WaterAllocator
	cooldown   map[string]time.Time
}

type AlertConfig struct {
	LowFlowThreshold     float64
	HighFlowThreshold    float64
	LowWaterLevel        float64
	SedimentationSpeed   float64
	HighEvaporation      float64
	WaterShortageRatio   float64
}

func New(database *db.Database, mqttClient *mqtt.Client,
	simulator *simulation.HydraulicSimulator, allocator *optimization.WaterAllocator) *AlertManager {
	return &AlertManager{
		database:   database,
		mqttClient: mqttClient,
		simulator:  simulator,
		allocator:  allocator,
		cooldown:   make(map[string]time.Time),
	}
}

func (am *AlertManager) CheckAndAlert(ctx context.Context, karezID int) error {
	segments, err := am.database.GetAqueductSegments(ctx, karezID)
	if err != nil {
		return fmt.Errorf("failed to get segments: %w", err)
	}

	for _, segment := range segments {
		if err := am.checkSegmentAlerts(ctx, karezID, segment); err != nil {
			log.Printf("Error checking segment %d alerts: %v", segment.ID, err)
		}
	}

	if err := am.checkWaterShortage(ctx, karezID); err != nil {
		log.Printf("Error checking water shortage: %v", err)
	}

	return nil
}

func (am *AlertManager) checkSegmentAlerts(ctx context.Context, karezID int, segment models.AqueductSegment) error {
	flowRate, err := am.database.GetLatestFlowRate(ctx, karezID, segment.ID)
	if err != nil {
		return err
	}

	params := simulation.ChannelParams{
		Width:          segment.Width,
		Height:         segment.Height,
		Slope:          segment.Slope,
		RoughnessCoeff: segment.RoughnessCoeff,
		SeepageCoeff:   segment.SeepageCoeff,
		Length:         segment.Length,
		Temperature:    25.0,
	}

	simResult := am.simulator.SimulateSegment(params, flowRate)

	alertKey := fmt.Sprintf("low_flow_%d_%d", karezID, segment.ID)
	if flowRate > 0 && flowRate < 0.02 {
		am.triggerAlert(ctx, &models.AlertEvent{
			Time:           time.Now(),
			KarezID:        karezID,
			SegmentID:      segment.ID,
			AlertType:      "low_flow",
			AlertLevel:     "warning",
			Message:        fmt.Sprintf("暗渠段 %s 流量过低: %.4f m³/s", segment.SegmentName, flowRate),
			CurrentValue:   flowRate,
			ThresholdValue: 0.02,
		}, alertKey)
	}

	alertKey = fmt.Sprintf("sedimentation_%d_%d", karezID, segment.ID)
	sedimentationRisk := am.simulator.EstimateSedimentationRisk(simResult.FlowVelocity)
	if sedimentationRisk >= 0.7 {
		level := "warning"
		if sedimentationRisk >= 0.9 {
			level = "critical"
		}
		am.triggerAlert(ctx, &models.AlertEvent{
			Time:           time.Now(),
			KarezID:        karezID,
			SegmentID:      segment.ID,
			AlertType:      "sedimentation",
			AlertLevel:     level,
			Message:        fmt.Sprintf("暗渠段 %s 淤塞风险高: 流速 %.4f m/s", segment.SegmentName, simResult.FlowVelocity),
			CurrentValue:   simResult.FlowVelocity,
			ThresholdValue: 0.3,
		}, alertKey)
	}

	return nil
}

func (am *AlertManager) checkWaterShortage(ctx context.Context, karezID int) error {
	oases, err := am.database.GetOases(ctx, karezID)
	if err != nil {
		return err
	}

	totalDemand := 0.0
	for _, o := range oases {
		totalDemand += o.DailyWaterDemand / 86400.0
	}

	if totalDemand <= 0 {
		return nil
	}

	segments, err := am.database.GetAqueductSegments(ctx, karezID)
	if err != nil {
		return err
	}

	if len(segments) == 0 {
		return nil
	}

	lastSegment := segments[len(segments)-1]
	outflow, err := am.database.GetLatestFlowRate(ctx, karezID, lastSegment.ID)
	if err != nil {
		return err
	}

	if outflow <= 0 {
		return nil
	}

	supplyRatio := outflow / totalDemand

	alertKey := fmt.Sprintf("water_shortage_%d", karezID)
	if supplyRatio < 0.8 {
		level := "warning"
		if supplyRatio < 0.5 {
			level = "critical"
		}
		am.triggerAlert(ctx, &models.AlertEvent{
			Time:           time.Now(),
			KarezID:        karezID,
			AlertType:      "water_shortage",
			AlertLevel:     level,
			Message:        fmt.Sprintf("水量不足: 供水 %.4f m³/s，需求 %.4f m³/s，满足率 %.1f%%",
				outflow, totalDemand, supplyRatio*100),
			CurrentValue:   supplyRatio,
			ThresholdValue: 0.8,
		}, alertKey)
	}

	return nil
}

func (am *AlertManager) triggerAlert(ctx context.Context, alert *models.AlertEvent, alertKey string) {
	if lastAlert, exists := am.cooldown[alertKey]; exists {
		if time.Since(lastAlert) < 30*time.Minute {
			return
		}
	}

	if err := am.database.InsertAlertEvent(ctx, alert); err != nil {
		log.Printf("Failed to insert alert event: %v", err)
		return
	}

	if am.mqttClient != nil {
		alertMsg := &mqtt.AlertMessage{
			Time:       alert.Time,
			AlertID:    alert.AlertID,
			KarezID:    alert.KarezID,
			AlertType:  alert.AlertType,
			AlertLevel: alert.AlertLevel,
			Message:    alert.Message,
			Value:      alert.CurrentValue,
			Threshold:  alert.ThresholdValue,
		}
		if err := am.mqttClient.PublishAlert(alertMsg); err != nil {
			log.Printf("Failed to publish alert via MQTT: %v", err)
		}
	}

	am.cooldown[alertKey] = time.Now()
	log.Printf("Alert triggered: %s - %s", alert.AlertType, alert.Message)
}

func (am *AlertManager) CheckAllKarez(ctx context.Context) error {
	systems, err := am.database.GetKarezSystems(ctx)
	if err != nil {
		return err
	}

	for _, sys := range systems {
		if err := am.CheckAndAlert(ctx, sys.ID); err != nil {
			log.Printf("Error checking alerts for karez %d: %v", sys.ID, err)
		}
	}

	return nil
}

func (am *AlertManager) AcknowledgeAlert(ctx context.Context, alertID int) error {
	_, err := am.database.GetPool().Exec(ctx,
		"UPDATE alert_events SET acknowledged = true WHERE alert_id = $1", alertID)
	return err
}

func (am *AlertManager) ResolveAlert(ctx context.Context, alertID int) error {
	_, err := am.database.GetPool().Exec(ctx,
		"UPDATE alert_events SET resolved = true WHERE alert_id = $1", alertID)
	return err
}
