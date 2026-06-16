package models

import "time"

type KarezSystem struct {
	ID             int       `json:"id"`
	Name           string    `json:"name"`
	Location       string    `json:"location"`
	TotalLength    float64   `json:"total_length"`
	HeadElevation  float64   `json:"head_elevation"`
	TailElevation  float64   `json:"tail_elevation"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"created_at"`
}

type AqueductSegment struct {
	ID              int       `json:"id"`
	KarezID         int       `json:"karez_id"`
	SegmentName     string    `json:"segment_name"`
	SegmentOrder    int       `json:"segment_order"`
	StartElevation  float64   `json:"start_elevation"`
	EndElevation    float64   `json:"end_elevation"`
	Length          float64   `json:"length"`
	Width           float64   `json:"width"`
	Height          float64   `json:"height"`
	Slope           float64   `json:"slope"`
	RoughnessCoeff  float64   `json:"roughness_coeff"`
	SeepageCoeff    float64   `json:"seepage_coeff"`
	IsMainChannel   bool      `json:"is_main_channel"`
	CreatedAt       time.Time `json:"created_at"`
}

type VerticalShaft struct {
	ID               int       `json:"id"`
	KarezID          int       `json:"karez_id"`
	SegmentID        int       `json:"segment_id"`
	ShaftName        string    `json:"shaft_name"`
	ShaftOrder       int       `json:"shaft_order"`
	GroundElevation  float64   `json:"ground_elevation"`
	ShaftDepth       float64   `json:"shaft_depth"`
	Diameter         float64   `json:"diameter"`
	DistanceFromHead float64   `json:"distance_from_head"`
	CreatedAt        time.Time `json:"created_at"`
}

type BranchChannel struct {
	ID                int       `json:"id"`
	KarezID           int       `json:"karez_id"`
	MainSegmentID     int       `json:"main_segment_id"`
	BranchName        string    `json:"branch_name"`
	DesignFlow        float64   `json:"design_flow"`
	MaxFlow           float64   `json:"max_flow"`
	Length            float64   `json:"length"`
	Width             float64   `json:"width"`
	Height            float64   `json:"height"`
	CurrentAllocation float64   `json:"current_allocation"`
	CreatedAt         time.Time `json:"created_at"`
}

type Oasis struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	BranchChannelID  int       `json:"branch_channel_id"`
	Area             float64   `json:"area"`
	DailyWaterDemand float64   `json:"daily_water_demand"`
	Priority         int       `json:"priority"`
	CreatedAt        time.Time `json:"created_at"`
}

type SensorData struct {
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
	Metadata        string    `json:"metadata,omitempty"`
}

type SimulationResult struct {
	Time            time.Time `json:"time"`
	KarezID         int       `json:"karez_id"`
	SegmentID       int       `json:"segment_id,omitempty"`
	SimulationType  string    `json:"simulation_type"`
	InflowRate      float64   `json:"inflow_rate"`
	OutflowRate     float64   `json:"outflow_rate"`
	SeepageLoss     float64   `json:"seepage_loss"`
	EvaporationLoss float64   `json:"evaporation_loss"`
	TotalLoss       float64   `json:"total_loss"`
	WaterDepth      float64   `json:"water_depth"`
	FlowVelocity    float64   `json:"flow_velocity"`
	ReynoldsNumber  float64   `json:"reynolds_number"`
	FroudeNumber    float64   `json:"froude_number"`
	HeadLoss        float64   `json:"head_loss"`
	Metadata        string    `json:"metadata,omitempty"`
}

type AllocationResult struct {
	Time               time.Time `json:"time"`
	KarezID            int       `json:"karez_id"`
	BranchChannelID    int       `json:"branch_channel_id,omitempty"`
	OasisID            int       `json:"oasis_id,omitempty"`
	AllocatedFlow      float64   `json:"allocated_flow"`
	AllocationRatio    float64   `json:"allocation_ratio"`
	DemandMet          float64   `json:"demand_met"`
	OptimizationMethod string    `json:"optimization_method"`
	ObjectiveValue     float64   `json:"objective_value"`
	Metadata           string    `json:"metadata,omitempty"`
}

type AlertEvent struct {
	Time            time.Time `json:"time"`
	AlertID         int       `json:"alert_id"`
	KarezID         int       `json:"karez_id,omitempty"`
	SegmentID       int       `json:"segment_id,omitempty"`
	BranchChannelID int       `json:"branch_channel_id,omitempty"`
	AlertType       string    `json:"alert_type"`
	AlertLevel      string    `json:"alert_level"`
	Message         string    `json:"message"`
	CurrentValue    float64   `json:"current_value"`
	ThresholdValue  float64   `json:"threshold_value"`
	Acknowledged    bool      `json:"acknowledged"`
	Resolved        bool      `json:"resolved"`
	Metadata        string    `json:"metadata,omitempty"`
}

type AlertRule struct {
	ID         int       `json:"id"`
	RuleName   string    `json:"rule_name"`
	RuleType   string    `json:"rule_type"`
	Threshold  float64   `json:"threshold"`
	Comparison string    `json:"comparison"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
}
