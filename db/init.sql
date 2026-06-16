-- 古代坎儿井暗渠水力仿真与水量分配系统
-- TimescaleDB 初始化脚本

-- 创建扩展
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- ============================================
-- 维度表
-- ============================================

-- 坎儿井主表
CREATE TABLE IF NOT EXISTS karez_systems (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    location VARCHAR(200),
    total_length NUMERIC(10,2),
    head_elevation NUMERIC(10,2),
    tail_elevation NUMERIC(10,2),
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 暗渠段表
CREATE TABLE IF NOT EXISTS aqueduct_segments (
    id SERIAL PRIMARY KEY,
    karez_id INTEGER REFERENCES karez_systems(id),
    segment_name VARCHAR(100),
    segment_order INTEGER,
    start_elevation NUMERIC(10,2),
    end_elevation NUMERIC(10,2),
    length NUMERIC(10,2),
    width NUMERIC(8,2),
    height NUMERIC(8,2),
    slope NUMERIC(8,6),
    roughness_coeff NUMERIC(8,4) DEFAULT 0.013,
    seepage_coeff NUMERIC(10,6) DEFAULT 0.0001,
    is_main_channel BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 竖井筒表
CREATE TABLE IF NOT EXISTS vertical_shafts (
    id SERIAL PRIMARY KEY,
    karez_id INTEGER REFERENCES karez_systems(id),
    segment_id INTEGER REFERENCES aqueduct_segments(id),
    shaft_name VARCHAR(100),
    shaft_order INTEGER,
    ground_elevation NUMERIC(10,2),
    shaft_depth NUMERIC(10,2),
    diameter NUMERIC(8,2),
    distance_from_head NUMERIC(10,2),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 支渠表
CREATE TABLE IF NOT EXISTS branch_channels (
    id SERIAL PRIMARY KEY,
    karez_id INTEGER REFERENCES karez_systems(id),
    main_segment_id INTEGER REFERENCES aqueduct_segments(id),
    branch_name VARCHAR(100),
    design_flow NUMERIC(10,4),
    max_flow NUMERIC(10,4),
    length NUMERIC(10,2),
    width NUMERIC(8,2),
    height NUMERIC(8,2),
    current_allocation NUMERIC(6,3) DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 下游绿洲表
CREATE TABLE IF NOT EXISTS oases (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    branch_channel_id INTEGER REFERENCES branch_channels(id),
    area NUMERIC(12,2),
    daily_water_demand NUMERIC(12,4),
    priority INTEGER DEFAULT 5,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 告警规则配置表
CREATE TABLE IF NOT EXISTS alert_rules (
    id SERIAL PRIMARY KEY,
    rule_name VARCHAR(100),
    rule_type VARCHAR(50),
    threshold NUMERIC(12,4),
    comparison VARCHAR(10),
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- 时序数据表（使用 TimescaleDB 超表）
-- ============================================

-- 传感器数据表（超表）
CREATE TABLE IF NOT EXISTS sensor_data (
    time TIMESTAMPTZ NOT NULL,
    karez_id INTEGER NOT NULL,
    segment_id INTEGER,
    shaft_id INTEGER,
    sensor_type VARCHAR(50) NOT NULL,
    sensor_id VARCHAR(100) NOT NULL,
    flow_rate NUMERIC(12,4),
    water_level NUMERIC(10,4),
    shaft_water_level NUMERIC(10,4),
    evaporation NUMERIC(10,4),
    temperature NUMERIC(8,2),
    turbidity NUMERIC(10,2),
    velocity NUMERIC(10,4),
    metadata JSONB DEFAULT '{}'::jsonb
);

-- 创建超表
SELECT create_hypertable('sensor_data', 'time', if_not_exists => TRUE);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_sensor_data_karez ON sensor_data(karez_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_sensor_data_segment ON sensor_data(segment_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_sensor_data_type ON sensor_data(sensor_type, time DESC);
CREATE INDEX IF NOT EXISTS idx_sensor_data_sensor_id ON sensor_data(sensor_id, time DESC);

-- 水力仿真结果表
CREATE TABLE IF NOT EXISTS simulation_results (
    time TIMESTAMPTZ NOT NULL,
    karez_id INTEGER NOT NULL,
    segment_id INTEGER,
    simulation_type VARCHAR(50),
    inflow_rate NUMERIC(12,4),
    outflow_rate NUMERIC(12,4),
    seepage_loss NUMERIC(12,4),
    evaporation_loss NUMERIC(12,4),
    total_loss NUMERIC(12,4),
    water_depth NUMERIC(10,4),
    flow_velocity NUMERIC(10,4),
    reynolds_number NUMERIC(12,2),
    froude_number NUMERIC(8,4),
    head_loss NUMERIC(10,4),
    metadata JSONB DEFAULT '{}'::jsonb
);

SELECT create_hypertable('simulation_results', 'time', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_sim_results_karez ON simulation_results(karez_id, time DESC);

-- 水量分配结果表
CREATE TABLE IF NOT EXISTS allocation_results (
    time TIMESTAMPTZ NOT NULL,
    karez_id INTEGER NOT NULL,
    branch_channel_id INTEGER,
    oasis_id INTEGER,
    allocated_flow NUMERIC(12,4),
    allocation_ratio NUMERIC(6,3),
    demand_met NUMERIC(6,3),
    optimization_method VARCHAR(50),
    objective_value NUMERIC(12,4),
    metadata JSONB DEFAULT '{}'::jsonb
);

SELECT create_hypertable('allocation_results', 'time', if_not_exists => TRUE);

-- 告警事件表
CREATE TABLE IF NOT EXISTS alert_events (
    time TIMESTAMPTZ NOT NULL,
    alert_id SERIAL,
    karez_id INTEGER,
    segment_id INTEGER,
    branch_channel_id INTEGER,
    alert_type VARCHAR(50),
    alert_level VARCHAR(20),
    message TEXT,
    current_value NUMERIC(12,4),
    threshold_value NUMERIC(12,4),
    acknowledged BOOLEAN DEFAULT false,
    resolved BOOLEAN DEFAULT false,
    metadata JSONB DEFAULT '{}'::jsonb
);

SELECT create_hypertable('alert_events', 'time', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_alert_events_karez ON alert_events(karez_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_alert_events_unresolved ON alert_events(resolved, time DESC);

-- ============================================
-- 连续聚合视图
-- ============================================

-- 每小时流量汇总
CREATE MATERIALIZED VIEW IF NOT EXISTS hourly_flow_summary
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', time) AS bucket,
    karez_id,
    segment_id,
    AVG(flow_rate) AS avg_flow_rate,
    MAX(flow_rate) AS max_flow_rate,
    MIN(flow_rate) AS min_flow_rate,
    SUM(flow_rate / 3600) AS total_volume
FROM sensor_data
WHERE sensor_type = 'flow'
GROUP BY bucket, karez_id, segment_id
WITH NO DATA;

-- 每日损失汇总
CREATE MATERIALIZED VIEW IF NOT EXISTS daily_loss_summary
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 day', time) AS bucket,
    karez_id,
    segment_id,
    SUM(seepage_loss) AS total_seepage_loss,
    SUM(evaporation_loss) AS total_evaporation_loss,
    SUM(total_loss) AS total_loss
FROM simulation_results
GROUP BY bucket, karez_id, segment_id
WITH NO DATA;

-- ============================================
-- 初始数据
-- ============================================

-- 插入示例坎儿井系统（吐鲁番坎儿井）
INSERT INTO karez_systems (name, location, total_length, head_elevation, tail_elevation, description)
VALUES 
('吐鲁番坎儿井-木纳尔', '新疆吐鲁番市', 5200.0, 85.0, -5.0, '吐鲁番著名坎儿井，清代修建，全长约5.2公里')
ON CONFLICT DO NOTHING;

-- 插入暗渠段
INSERT INTO aqueduct_segments (karez_id, segment_name, segment_order, start_elevation, end_elevation, length, width, height, slope, is_main_channel)
VALUES 
(1, '首部暗渠段', 1, 85.0, 80.0, 800.0, 0.8, 1.2, 0.00625, true),
(1, '中部暗渠段', 2, 80.0, 70.0, 1800.0, 0.8, 1.2, 0.00556, true),
(1, '尾部暗渠段', 3, 70.0, 55.0, 1600.0, 0.8, 1.2, 0.00938, true),
(1, '龙口段', 4, 55.0, -5.0, 1000.0, 1.0, 1.5, 0.06000, true)
ON CONFLICT DO NOTHING;

-- 插入竖井（生成20个竖井）
INSERT INTO vertical_shafts (karez_id, segment_id, shaft_name, shaft_order, ground_elevation, shaft_depth, diameter, distance_from_head)
SELECT 1, 
       (SELECT id FROM aqueduct_segments WHERE karez_id = 1 ORDER BY segment_order LIMIT 1 OFFSET (n-1) % 4),
       '竖井-' || n,
       n,
       200.0 - n * 2.5,
       120.0 + n * 1.5,
       0.8,
       n * 250.0
FROM generate_series(1, 20) as n
ON CONFLICT DO NOTHING;

-- 插入支渠
INSERT INTO branch_channels (karez_id, main_segment_id, branch_name, design_flow, max_flow, length, width, height, current_allocation)
VALUES 
(1, 4, '东支渠', 0.05, 0.08, 1500.0, 0.5, 0.6, 0.4),
(1, 4, '西支渠', 0.05, 0.08, 1200.0, 0.5, 0.6, 0.35),
(1, 4, '南支渠', 0.03, 0.05, 800.0, 0.4, 0.5, 0.25)
ON CONFLICT DO NOTHING;

-- 插入绿洲
INSERT INTO oases (name, branch_channel_id, area, daily_water_demand, priority)
VALUES 
('东绿洲', 1, 120.0, 1200.0, 1),
('西绿洲', 2, 100.0, 900.0, 2),
('南绿洲', 3, 60.0, 500.0, 3)
ON CONFLICT DO NOTHING;

-- 插入告警规则
INSERT INTO alert_rules (rule_name, rule_type, threshold, comparison, enabled)
VALUES 
('低流量告警', 'low_flow', 0.02, '<=', true),
('高流量告警', 'high_flow', 0.15, '>=', true),
('低水位告警', 'low_water_level', 0.3, '<=', true),
('淤塞告警-流速', 'sedimentation', 0.3, '<=', true),
('高蒸发告警', 'high_evaporation', 5.0, '>=', true),
('供水不足告警', 'water_shortage', 0.8, '<=', true)
ON CONFLICT DO NOTHING;
