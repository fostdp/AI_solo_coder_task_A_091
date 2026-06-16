/**
 * 坎儿井系统前端主逻辑
 * 处理数据获取、UI更新、用户交互
 */

const API_BASE = 'http://localhost:8080/api';
const KAREZ_ID = 1;

let currentSegments = [];
let currentShafts = [];
let currentBranches = [];
let currentOases = [];
let selectedSegmentId = null;
let refreshTimer = null;

document.addEventListener('DOMContentLoaded', () => {
    initKarez3D();
    loadDashboardData();
    updateCurrentTime();
    setInterval(updateCurrentTime, 1000);
    
    refreshTimer = setInterval(loadDashboardData, 10000);
});

function updateCurrentTime() {
    const now = new Date();
    document.getElementById('currentTime').textContent = now.toLocaleString('zh-CN');
}

async function fetchAPI(endpoint, options = {}) {
    try {
        const response = await fetch(`${API_BASE}${endpoint}`, {
            headers: {
                'Content-Type': 'application/json',
            },
            ...options
        });
        
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        
        return await response.json();
    } catch (error) {
        console.error(`API Error [${endpoint}]:`, error);
        return null;
    }
}

async function loadDashboardData() {
    const data = await fetchAPI(`/karez/${KAREZ_ID}/dashboard`);
    
    if (data) {
        updateDashboard(data);
        currentSegments = data.segments || [];
        currentShafts = data.shafts || [];
        currentBranches = data.branch_channels || [];
        currentOases = data.oases || [];
        
        updateSegmentDetails(currentSegments, data.latest_data || []);
        updateAllocationDisplay(data.oases || [], data.total_flow || 0);
        updateAlerts(data.active_alerts || []);
        
        const flowData = {};
        if (data.latest_data) {
            data.latest_data.forEach(d => {
                if (d.sensor_type === 'flow' && d.segment_id) {
                    flowData[d.segment_id] = d.flow_rate;
                }
            });
        }
        if (karezViewer && Object.keys(flowData).length > 0) {
            karezViewer.updateFlowData(flowData);
        }
    }
}

function updateDashboard(data) {
    document.getElementById('totalFlow').textContent = formatNumber(data.total_flow, 4);
    document.getElementById('totalDemand').textContent = formatNumber(data.total_demand, 4);
    document.getElementById('supplyRatio').textContent = formatNumber(data.supply_ratio * 100, 1) + '%';
    document.getElementById('alertCount').textContent = data.alert_count || 0;
    
    const supplyRatioEl = document.getElementById('supplyRatio');
    if (data.supply_ratio < 0.5) {
        supplyRatioEl.style.color = '#f44336';
    } else if (data.supply_ratio < 0.8) {
        supplyRatioEl.style.color = '#ff9800';
    } else {
        supplyRatioEl.style.color = '#64b5f6';
    }
}

function updateSegmentDetails(segments, latestData) {
    const container = document.getElementById('segmentDetails');
    
    if (!segments || segments.length === 0) {
        container.innerHTML = '<div class="empty-state">暂无暗渠数据</div>';
        return;
    }

    const flowMap = {};
    const levelMap = {};
    latestData.forEach(d => {
        if (d.sensor_type === 'flow' && d.segment_id) {
            if (!flowMap[d.segment_id] || new Date(d.time) > new Date(flowMap[d.segment_id].time)) {
                flowMap[d.segment_id] = d;
            }
        }
        if (d.sensor_type === 'water_level' && d.segment_id) {
            if (!levelMap[d.segment_id] || new Date(d.time) > new Date(levelMap[d.segment_id].time)) {
                levelMap[d.segment_id] = d;
            }
        }
    });

    container.innerHTML = segments.map(seg => {
        const flow = flowMap[seg.id];
        const level = levelMap[seg.id];
        const isSelected = selectedSegmentId === seg.id;
        
        return `
            <div class="segment-card ${isSelected ? 'selected' : ''}" 
                 onclick="selectSegment(${seg.id})"
                 data-segment-id="${seg.id}">
                <h4>${seg.segment_name}</h4>
                <div class="segment-stats">
                    <div>
                        <span class="stat-label">长度</span>
                        <span class="stat-val">${seg.length} m</span>
                    </div>
                    <div>
                        <span class="stat-label">坡度</span>
                        <span class="stat-val">${(seg.slope * 1000).toFixed(2)}‰</span>
                    </div>
                    <div>
                        <span class="stat-label">流量</span>
                        <span class="stat-val">${flow ? formatNumber(flow.flow_rate, 4) : '--'} m³/s</span>
                    </div>
                    <div>
                        <span class="stat-label">水位</span>
                        <span class="stat-val">${level ? formatNumber(level.water_level, 3) : '--'} m</span>
                    </div>
                </div>
            </div>
        `;
    }).join('');
}

function selectSegment(segmentId) {
    selectedSegmentId = segmentId;
    
    const cards = document.querySelectorAll('.segment-card');
    cards.forEach(card => {
        if (parseInt(card.dataset.segmentId) === segmentId) {
            card.classList.add('selected');
        } else {
            card.classList.remove('selected');
        }
    });
    
    if (karezViewer) {
        karezViewer.highlightSegment(segmentId);
    }
    
    const seg = currentSegments.find(s => s.id === segmentId);
    if (seg) {
        showSegmentDetailModal(seg);
    }
}

function showSegmentDetailModal(segment) {
    console.log('Selected segment:', segment);
}

function updateAllocationDisplay(oases, totalFlow) {
    if (window.AllocationPanel) {
        AllocationPanel.updateOases(oases, totalFlow);
    }
}

function updateAlerts(alerts) {
    const container = document.getElementById('alertList');
    
    if (!alerts || alerts.length === 0) {
        container.innerHTML = '<div class="empty-state">暂无告警</div>';
        return;
    }

    container.innerHTML = alerts.map(alert => `
        <div class="alert-item ${alert.alert_level}">
            <div class="alert-type">${getAlertTypeName(alert.alert_type)}</div>
            <div class="alert-message">${alert.message}</div>
            <div style="font-size: 11px; color: #78909c; margin-top: 4px;">
                ${new Date(alert.time).toLocaleString('zh-CN')}
            </div>
        </div>
    `).join('');
}

function getAlertTypeName(type) {
    const names = {
        'low_flow': '⚠️ 低流量',
        'high_flow': '📈 高流量',
        'low_water_level': '📉 低水位',
        'sedimentation': '🏜️ 淤塞风险',
        'high_evaporation': '☀️ 高蒸发',
        'water_shortage': '💧 供水不足'
    };
    return names[type] || type;
}

function runOptimization() {
    if (window.AllocationPanel) {
        AllocationPanel.runOptimization();
    }
}

async function runHydraulicSim() {
    const btn = event.target;
    btn.disabled = true;
    btn.textContent = '仿真中...';
    
    try {
        const result = await fetchAPI('/simulate', {
            method: 'POST',
            body: JSON.stringify({
                karez_id: KAREZ_ID
            })
        });
        
        if (result && result.status === 'success') {
            console.log('水力仿真完成');
            loadDashboardData();
        }
    } catch (error) {
        console.error('仿真失败:', error);
    } finally {
        btn.disabled = false;
        btn.textContent = '运行水力仿真';
    }
}

function formatNumber(num, decimals = 2) {
    if (num === null || num === undefined || isNaN(num)) {
        return '--';
    }
    return num.toFixed(decimals);
}

async function loadHistoricalData(startDate, endDate) {
    const start = startDate ? new Date(startDate).toISOString() : '';
    const end = endDate ? new Date(endDate).toISOString() : '';
    
    const data = await fetchAPI(
        `/sensor/${KAREZ_ID}/range?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`
    );
    
    return data || [];
}

function connectMQTTAlerts() {
    console.log('MQTT alert subscription would go here');
}

window.runOptimization = runOptimization;
window.runHydraulicSim = runHydraulicSim;
window.selectSegment = selectSegment;
