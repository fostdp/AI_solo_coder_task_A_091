package alarmmqtt

import (
	"context"
	"fmt"
	"karez-system/config"
	"karez-system/db"
	"karez-system/models"
	"karez-system/mqtt"
	"log"
	"math"
	"time"
)

type AlarmCheckRequest struct {
	KarezID  int
	ForceRun bool
}

type AlarmManager struct {
	cfg          *config.Config
	database     *db.Database
	mqttClient   *mqtt.Client
	inputChan    chan AlarmCheckRequest
	simInputChan <-chan interface{}
	cooldown     map[string]time.Time
	cooldownDur  time.Duration
}

func New(cfg *config.Config, database *db.Database, mqttClient *mqtt.Client,
	inputChan chan AlarmCheckRequest) *AlarmManager {
	return &AlarmManager{
		cfg:         cfg,
		database:    database,
		mqttClient:  mqttClient,
		inputChan:   inputChan,
		cooldown:    make(map[string]time.Time),
		cooldownDur: 30 * time.Minute,
	}
}

func (am *AlarmManager) Start(ctx context.Context) {
	go am.run(ctx)
	log.Println("Alarm MQTT: started")
}

func (am *AlarmManager) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Alarm MQTT: stopped")
			return
		case req := <-am.inputChan:
			if err := am.CheckAndAlert(ctx, req.KarezID); err != nil {
				log.Printf("Alarm MQTT: check failed for karez %d: %v", req.KarezID, err)
			}
		}
	}
}

func (am *AlarmManager) CheckAndAlert(ctx context.Context, karezID int) error {
	segments, err := am.database.GetAqueductSegments(ctx, karezID)
	if err != nil {
		return fmt.Errorf("failed to get segments: %w", err)
	}

	for _, segment := range segments {
		if err := am.checkSegmentAlerts(ctx, karezID, segment); err != nil {
			log.Printf("Alarm MQTT: error checking segment %d alerts: %v", segment.ID, err)
		}
	}

	if err := am.checkWaterShortage(ctx, karezID); err != nil {
		log.Printf("Alarm MQTT: error checking water shortage: %v", err)
	}

	return nil
}

func (am *AlarmManager) checkSegmentAlerts(ctx context.Context, karezID int, segment models.AqueductSegment) error {
	flowRate, err := am.database.GetLatestFlowRate(ctx, karezID, segment.ID)
	if err != nil {
		return err
	}

	thresh := am.cfg.HydraulicParams.SedimentationThresholds
	defaults := am.cfg.HydraulicParams.DefaultChannel

	params := struct {
		Width          float64
		Height         float64
		Slope          float64
		RoughnessCoeff float64
		SeepageCoeff   float64
		Length         float64
		Temperature    float64
	}{
		Width:          segment.Width,
		Height:         segment.Height,
		Slope:          segment.Slope,
		RoughnessCoeff: segment.RoughnessCoeff,
		SeepageCoeff:   segment.SeepageCoeff,
		Length:         segment.Length,
		Temperature:    defaults.DefaultTemperature,
	}

	velocity := am.estimateVelocity(params, flowRate)

	alertKey := fmt.Sprintf("low_flow_%d_%d", karezID, segment.ID)
	lowFlowThreshold := 0.02
	if flowRate > 0 && flowRate < lowFlowThreshold {
		am.triggerAlert(ctx, &models.AlertEvent{
			Time:           time.Now(),
			KarezID:        karezID,
			SegmentID:      segment.ID,
			AlertType:      "low_flow",
			AlertLevel:     "warning",
			Message:        fmt.Sprintf("暗渠段 %s 流量过低: %.4f m³/s", segment.SegmentName, flowRate),
			CurrentValue:   flowRate,
			ThresholdValue: lowFlowThreshold,
		}, alertKey)
	}

	alertKey = fmt.Sprintf("sedimentation_%d_%d", karezID, segment.ID)
	sedimentationRisk := am.estimateSedimentationRisk(velocity)
	if sedimentationRisk >= thresh.HighRiskScore {
		level := "warning"
		if sedimentationRisk >= thresh.CriticalRiskScore {
			level = "critical"
		}
		am.triggerAlert(ctx, &models.AlertEvent{
			Time:           time.Now(),
			KarezID:        karezID,
			SegmentID:      segment.ID,
			AlertType:      "sedimentation",
			AlertLevel:     level,
			Message:        fmt.Sprintf("暗渠段 %s 淤塞风险高: 流速 %.4f m/s", segment.SegmentName, velocity),
			CurrentValue:   velocity,
			ThresholdValue: thresh.HighRiskVelocity,
		}, alertKey)
	}

	return nil
}

func (am *AlarmManager) estimateVelocity(params struct {
	Width          float64
	Height         float64
	Slope          float64
	RoughnessCoeff float64
	SeepageCoeff   float64
	Length         float64
	Temperature    float64
}, flowRate float64) float64 {
	if flowRate <= 0 {
		return 0
	}
	depth := flowRate / (params.Width * 0.5)
	if depth <= 0 {
		return 0
	}
	if depth > params.Height {
		depth = params.Height
	}
	area := params.Width * depth
	wettedPerimeter := params.Width + 2*depth
	hydraulicRadius := area / wettedPerimeter
	return (1.0 / params.RoughnessCoeff) * math.Pow(hydraulicRadius, 2.0/3.0) * math.Sqrt(params.Slope)
}

func (am *AlarmManager) estimateSedimentationRisk(velocity float64) float64 {
	thresh := am.cfg.HydraulicParams.SedimentationThresholds
	if velocity >= thresh.LowRiskVelocity {
		return thresh.LowRiskScore
	} else if velocity >= thresh.MediumRiskVelocity {
		return thresh.MediumRiskScore
	} else if velocity >= thresh.HighRiskVelocity {
		return thresh.HighRiskScore
	} else {
		return thresh.CriticalRiskScore
	}
}

func (am *AlarmManager) checkWaterShortage(ctx context.Context, karezID int) error {
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

	shortageCfg := am.cfg.AgricultureDemand.WaterShortageLevels
	alertKey := fmt.Sprintf("water_shortage_%d", karezID)
	if supplyRatio < shortageCfg.WarningRatio {
		level := "warning"
		if supplyRatio < shortageCfg.CriticalRatio {
			level = "critical"
		}
		am.triggerAlert(ctx, &models.AlertEvent{
			Time:           time.Now(),
			KarezID:        karezID,
			AlertType:      "water_shortage",
			AlertLevel:     level,
			Message: fmt.Sprintf("水量不足: 供水 %.4f m³/s，需求 %.4f m³/s，满足率 %.1f%%",
				outflow, totalDemand, supplyRatio*100),
			CurrentValue:   supplyRatio,
			ThresholdValue: shortageCfg.WarningRatio,
		}, alertKey)
	}

	return nil
}

func (am *AlarmManager) triggerAlert(ctx context.Context, alert *models.AlertEvent, alertKey string) {
	if lastAlert, exists := am.cooldown[alertKey]; exists {
		if time.Since(lastAlert) < am.cooldownDur {
			return
		}
	}

	if err := am.database.InsertAlertEvent(ctx, alert); err != nil {
		log.Printf("Alarm MQTT: failed to insert alert event: %v", err)
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
			log.Printf("Alarm MQTT: failed to publish alert via MQTT: %v", err)
		}
	}

	am.cooldown[alertKey] = time.Now()
	log.Printf("Alarm MQTT: alert triggered: %s - %s", alert.AlertType, alert.Message)
}

func (am *AlarmManager) CheckAllKarez(ctx context.Context) error {
	systems, err := am.database.GetKarezSystems(ctx)
	if err != nil {
		return err
	}

	for _, sys := range systems {
		if err := am.CheckAndAlert(ctx, sys.ID); err != nil {
			log.Printf("Alarm MQTT: error checking alerts for karez %d: %v", sys.ID, err)
		}
	}

	return nil
}

func (am *AlarmManager) AcknowledgeAlert(ctx context.Context, alertID int) error {
	_, err := am.database.GetPool().Exec(ctx,
		"UPDATE alert_events SET acknowledged = true WHERE alert_id = $1", alertID)
	return err
}

func (am *AlarmManager) ResolveAlert(ctx context.Context, alertID int) error {
	_, err := am.database.GetPool().Exec(ctx,
		"UPDATE alert_events SET resolved = true WHERE alert_id = $1", alertID)
	return err
}

func (am *AlarmManager) RequestCheck(karezID int) error {
	select {
	case am.inputChan <- AlarmCheckRequest{KarezID: karezID, ForceRun: true}:
		return nil
	default:
		return fmt.Errorf("alarm check channel full")
	}
}
