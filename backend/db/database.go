package db

import (
	"context"
	"fmt"
	"karez-system/config"
	"karez-system/models"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Database struct {
	pool *pgxpool.Pool
}

func New(cfg *config.Config) (*Database, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Database{pool: pool}, nil
}

func (d *Database) Close() {
	d.pool.Close()
}

func (d *Database) GetPool() *pgxpool.Pool {
	return d.pool
}

func (d *Database) InsertSensorData(ctx context.Context, data *models.SensorData) error {
	query := `INSERT INTO sensor_data 
		(time, karez_id, segment_id, shaft_id, sensor_type, sensor_id, 
		 flow_rate, water_level, shaft_water_level, evaporation, temperature, turbidity, velocity)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	segID := nullableInt(data.SegmentID)
	shaftID := nullableInt(data.ShaftID)

	_, err := d.pool.Exec(ctx, query,
		data.Time, data.KarezID, segID, shaftID,
		data.SensorType, data.SensorID,
		nullableFloat(data.FlowRate), nullableFloat(data.WaterLevel), nullableFloat(data.ShaftWaterLevel),
		nullableFloat(data.Evaporation), nullableFloat(data.Temperature), nullableFloat(data.Turbidity), nullableFloat(data.Velocity))
	return err
}

func nullableInt(v int) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

func nullableFloat(v float64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

func (d *Database) InsertSimulationResult(ctx context.Context, result *models.SimulationResult) error {
	query := `INSERT INTO simulation_results 
		(time, karez_id, segment_id, simulation_type, 
		 inflow_rate, outflow_rate, seepage_loss, evaporation_loss, total_loss,
		 water_depth, flow_velocity, reynolds_number, froude_number, head_loss)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

	segID := nullableInt(result.SegmentID)

	_, err := d.pool.Exec(ctx, query,
		result.Time, result.KarezID, segID, result.SimulationType,
		result.InflowRate, result.OutflowRate, result.SeepageLoss, result.EvaporationLoss, result.TotalLoss,
		result.WaterDepth, result.FlowVelocity, result.ReynoldsNumber, result.FroudeNumber, result.HeadLoss)
	return err
}

func (d *Database) InsertAllocationResult(ctx context.Context, result *models.AllocationResult) error {
	query := `INSERT INTO allocation_results 
		(time, karez_id, branch_channel_id, oasis_id, 
		 allocated_flow, allocation_ratio, demand_met, optimization_method, objective_value)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	branchID := nullableInt(result.BranchChannelID)
	oasisID := nullableInt(result.OasisID)

	_, err := d.pool.Exec(ctx, query,
		result.Time, result.KarezID, branchID, oasisID,
		result.AllocatedFlow, result.AllocationRatio, result.DemandMet,
		result.OptimizationMethod, result.ObjectiveValue)
	return err
}

func (d *Database) InsertAlertEvent(ctx context.Context, alert *models.AlertEvent) error {
	query := `INSERT INTO alert_events 
		(time, karez_id, segment_id, branch_channel_id, 
		 alert_type, alert_level, message, current_value, threshold_value)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING alert_id`

	karezID := nullableInt(alert.KarezID)
	segID := nullableInt(alert.SegmentID)
	branchID := nullableInt(alert.BranchChannelID)

	err := d.pool.QueryRow(ctx, query,
		alert.Time, karezID, segID, branchID,
		alert.AlertType, alert.AlertLevel, alert.Message,
		alert.CurrentValue, alert.ThresholdValue).Scan(&alert.AlertID)
	return err
}

func (d *Database) GetKarezSystems(ctx context.Context) ([]models.KarezSystem, error) {
	rows, err := d.pool.Query(ctx, "SELECT id, name, location, total_length, head_elevation, tail_elevation, description, created_at FROM karez_systems ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var systems []models.KarezSystem
	for rows.Next() {
		var s models.KarezSystem
		err := rows.Scan(&s.ID, &s.Name, &s.Location, &s.TotalLength,
			&s.HeadElevation, &s.TailElevation, &s.Description, &s.CreatedAt)
		if err != nil {
			return nil, err
		}
		systems = append(systems, s)
	}
	return systems, rows.Err()
}

func (d *Database) GetAqueductSegments(ctx context.Context, karezID int) ([]models.AqueductSegment, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, karez_id, segment_name, segment_order, start_elevation, end_elevation, 
		 length, width, height, slope, roughness_coeff, seepage_coeff, is_main_channel, created_at 
		 FROM aqueduct_segments WHERE karez_id = $1 ORDER BY segment_order`, karezID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var segments []models.AqueductSegment
	for rows.Next() {
		var s models.AqueductSegment
		err := rows.Scan(&s.ID, &s.KarezID, &s.SegmentName, &s.SegmentOrder,
			&s.StartElevation, &s.EndElevation, &s.Length, &s.Width, &s.Height,
			&s.Slope, &s.RoughnessCoeff, &s.SeepageCoeff, &s.IsMainChannel, &s.CreatedAt)
		if err != nil {
			return nil, err
		}
		segments = append(segments, s)
	}
	return segments, rows.Err()
}

func (d *Database) GetVerticalShafts(ctx context.Context, karezID int) ([]models.VerticalShaft, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, karez_id, segment_id, shaft_name, shaft_order, ground_elevation, 
		 shaft_depth, diameter, distance_from_head, created_at 
		 FROM vertical_shafts WHERE karez_id = $1 ORDER BY shaft_order`, karezID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shafts []models.VerticalShaft
	for rows.Next() {
		var s models.VerticalShaft
		var segID *int
		err := rows.Scan(&s.ID, &s.KarezID, &segID, &s.ShaftName,
			&s.ShaftOrder, &s.GroundElevation, &s.ShaftDepth, &s.Diameter,
			&s.DistanceFromHead, &s.CreatedAt)
		if err != nil {
			return nil, err
		}
		if segID != nil {
			s.SegmentID = *segID
		}
		shafts = append(shafts, s)
	}
	return shafts, rows.Err()
}

func (d *Database) GetBranchChannels(ctx context.Context, karezID int) ([]models.BranchChannel, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, karez_id, main_segment_id, branch_name, design_flow, max_flow, 
		 length, width, height, current_allocation, created_at 
		 FROM branch_channels WHERE karez_id = $1 ORDER BY id`, karezID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []models.BranchChannel
	for rows.Next() {
		var c models.BranchChannel
		var mainSegID *int
		err := rows.Scan(&c.ID, &c.KarezID, &mainSegID, &c.BranchName,
			&c.DesignFlow, &c.MaxFlow, &c.Length, &c.Width, &c.Height,
			&c.CurrentAllocation, &c.CreatedAt)
		if err != nil {
			return nil, err
		}
		if mainSegID != nil {
			c.MainSegmentID = *mainSegID
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}

func (d *Database) GetOases(ctx context.Context, karezID int) ([]models.Oasis, error) {
	query := `SELECT o.id, o.name, o.branch_channel_id, o.area, o.daily_water_demand, o.priority, o.created_at 
		FROM oases o 
		JOIN branch_channels bc ON o.branch_channel_id = bc.id 
		WHERE bc.karez_id = $1 ORDER BY o.priority`
	rows, err := d.pool.Query(ctx, query, karezID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var oases []models.Oasis
	for rows.Next() {
		var o models.Oasis
		err := rows.Scan(&o.ID, &o.Name, &o.BranchChannelID, &o.Area,
			&o.DailyWaterDemand, &o.Priority, &o.CreatedAt)
		if err != nil {
			return nil, err
		}
		oases = append(oases, o)
	}
	return oases, rows.Err()
}

func (d *Database) GetLatestSensorData(ctx context.Context, karezID int, limit int) ([]models.SensorData, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT time, karez_id, segment_id, shaft_id, sensor_type, sensor_id,
		 flow_rate, water_level, shaft_water_level, evaporation, temperature, turbidity, velocity
		 FROM sensor_data WHERE karez_id = $1 
		 ORDER BY time DESC LIMIT $2`, karezID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []models.SensorData
	for rows.Next() {
		var sd models.SensorData
		var segID, shaftID *int
		var flowRate, waterLevel, shaftWaterLevel, evaporation, temperature, turbidity, velocity *float64

		err := rows.Scan(&sd.Time, &sd.KarezID, &segID, &shaftID,
			&sd.SensorType, &sd.SensorID,
			&flowRate, &waterLevel, &shaftWaterLevel, &evaporation,
			&temperature, &turbidity, &velocity)
		if err != nil {
			return nil, err
		}

		if segID != nil {
			sd.SegmentID = *segID
		}
		if shaftID != nil {
			sd.ShaftID = *shaftID
		}
		if flowRate != nil {
			sd.FlowRate = *flowRate
		}
		if waterLevel != nil {
			sd.WaterLevel = *waterLevel
		}
		if shaftWaterLevel != nil {
			sd.ShaftWaterLevel = *shaftWaterLevel
		}
		if evaporation != nil {
			sd.Evaporation = *evaporation
		}
		if temperature != nil {
			sd.Temperature = *temperature
		}
		if turbidity != nil {
			sd.Turbidity = *turbidity
		}
		if velocity != nil {
			sd.Velocity = *velocity
		}

		data = append(data, sd)
	}
	return data, rows.Err()
}

func (d *Database) GetSensorDataByRange(ctx context.Context, karezID int, startTime, endTime time.Time) ([]models.SensorData, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT time, karez_id, segment_id, shaft_id, sensor_type, sensor_id,
		 flow_rate, water_level, shaft_water_level, evaporation, temperature, turbidity, velocity
		 FROM sensor_data WHERE karez_id = $1 AND time BETWEEN $2 AND $3
		 ORDER BY time ASC`, karezID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []models.SensorData
	for rows.Next() {
		var sd models.SensorData
		var segID, shaftID *int
		var flowRate, waterLevel, shaftWaterLevel, evaporation, temperature, turbidity, velocity *float64

		err := rows.Scan(&sd.Time, &sd.KarezID, &segID, &shaftID,
			&sd.SensorType, &sd.SensorID,
			&flowRate, &waterLevel, &shaftWaterLevel, &evaporation,
			&temperature, &turbidity, &velocity)
		if err != nil {
			return nil, err
		}

		if segID != nil {
			sd.SegmentID = *segID
		}
		if shaftID != nil {
			sd.ShaftID = *shaftID
		}
		if flowRate != nil {
			sd.FlowRate = *flowRate
		}
		if waterLevel != nil {
			sd.WaterLevel = *waterLevel
		}
		if shaftWaterLevel != nil {
			sd.ShaftWaterLevel = *shaftWaterLevel
		}
		if evaporation != nil {
			sd.Evaporation = *evaporation
		}
		if temperature != nil {
			sd.Temperature = *temperature
		}
		if turbidity != nil {
			sd.Turbidity = *turbidity
		}
		if velocity != nil {
			sd.Velocity = *velocity
		}

		data = append(data, sd)
	}
	return data, rows.Err()
}

func (d *Database) GetActiveAlerts(ctx context.Context, karezID int) ([]models.AlertEvent, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT time, alert_id, karez_id, segment_id, branch_channel_id,
		 alert_type, alert_level, message, current_value, threshold_value,
		 acknowledged, resolved
		 FROM alert_events WHERE karez_id = $1 AND resolved = false
		 ORDER BY time DESC`, karezID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []models.AlertEvent
	for rows.Next() {
		var a models.AlertEvent
		var karezIDVal, segID, branchID *int

		err := rows.Scan(&a.Time, &a.AlertID, &karezIDVal, &segID, &branchID,
			&a.AlertType, &a.AlertLevel, &a.Message, &a.CurrentValue, &a.ThresholdValue,
			&a.Acknowledged, &a.Resolved)
		if err != nil {
			return nil, err
		}

		if karezIDVal != nil {
			a.KarezID = *karezIDVal
		}
		if segID != nil {
			a.SegmentID = *segID
		}
		if branchID != nil {
			a.BranchChannelID = *branchID
		}

		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (d *Database) GetAlertRules(ctx context.Context) ([]models.AlertRule, error) {
	rows, err := d.pool.Query(ctx, "SELECT id, rule_name, rule_type, threshold, comparison, enabled, created_at FROM alert_rules WHERE enabled = true ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.AlertRule
	for rows.Next() {
		var r models.AlertRule
		err := rows.Scan(&r.ID, &r.RuleName, &r.RuleType, &r.Threshold, &r.Comparison, &r.Enabled, &r.CreatedAt)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (d *Database) GetLatestFlowRate(ctx context.Context, karezID int, segmentID int) (float64, error) {
	var flowRate float64
	err := d.pool.QueryRow(ctx,
		`SELECT flow_rate FROM sensor_data 
		 WHERE karez_id = $1 AND segment_id = $2 AND sensor_type = 'flow'
		 ORDER BY time DESC LIMIT 1`, karezID, segmentID).Scan(&flowRate)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0.05, nil
		}
		return 0, err
	}
	return flowRate, nil
}
