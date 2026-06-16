/**
 * 水量分配面板模块
 * 负责绿洲需水展示、分配优化、分配结果可视化
 */

const AllocationPanel = (function() {
    const API_BASE = 'http://localhost:8080/api';
    const KAREZ_ID = 1;

    let currentOases = [];
    let currentTotalFlow = 0;
    let currentAllocations = null;

    function init() {
        console.log('Allocation Panel: initialized');
    }

    function updateOases(oases, totalFlow) {
        currentOases = oases || [];
        currentTotalFlow = totalFlow || 0;
        renderAllocationList();
    }

    function renderAllocationList() {
        const container = document.getElementById('allocationList');
        if (!container) return;

        if (!currentOases || currentOases.length === 0) {
            container.innerHTML = '<div class="empty-state">暂无绿洲数据</div>';
            return;
        }

        const totalDemand = currentOases.reduce((sum, o) => sum + o.daily_water_demand / 86400, 0);

        container.innerHTML = currentOases.map(oasis => {
            const demand = oasis.daily_water_demand / 86400;
            const allocRatio = totalDemand > 0 ? demand / totalDemand : 0;

            let actualAlloc = currentAllocations && currentAllocations[oasis.id] !== undefined
                ? currentAllocations[oasis.id]
                : currentTotalFlow * allocRatio;

            const demandMet = demand > 0 ? Math.min(1, actualAlloc / demand) : 0;

            const barColor = demandMet >= 0.9 ? '#4caf50' :
                             demandMet >= 0.7 ? '#ff9800' : '#f44336';

            return `
                <div class="allocation-item" data-oasis-id="${oasis.id}">
                    <div class="alloc-header">
                        <span class="alloc-name">${oasis.name}</span>
                        <span class="alloc-value">${(allocRatio * 100).toFixed(1)}%</span>
                    </div>
                    <div class="allocation-bar">
                        <div class="allocation-bar-fill" 
                             style="width: ${demandMet * 100}%; background: ${barColor}"></div>
                    </div>
                    <div style="font-size: 11px; color: #78909c; margin-top: 4px;">
                        需求: ${formatNumber(demand, 4)} m³/s | 优先级: ${oasis.priority}
                    </div>
                    <div style="font-size: 11px; color: #90a4ae; margin-top: 2px;">
                        分配: ${formatNumber(actualAlloc, 4)} m³/s | 满足率: ${(demandMet * 100).toFixed(1)}%
                    </div>
                </div>
            `;
        }).join('');
    }

    async function runOptimization() {
        const btn = event.target;
        btn.disabled = true;
        btn.textContent = '优化中...';

        try {
            const totalFlow = currentTotalFlow > 0 ? currentTotalFlow : 0.08;

            const response = await fetch(`${API_BASE}/allocate`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    karez_id: KAREZ_ID,
                    total_available_flow: totalFlow
                })
            });

            const result = await response.json();

            if (result && result.status === 'success') {
                console.log('优化结果:', result);
                currentAllocations = result.allocations;
                renderAllocationList();

                if (typeof onAllocationComplete === 'function') {
                    onAllocationComplete(result);
                }
            }
        } catch (error) {
            console.error('优化失败:', error);
        } finally {
            btn.disabled = false;
            btn.textContent = '运行优化分配';
        }
    }

    function getCurrentAllocations() {
        return currentAllocations;
    }

    function formatNumber(num, decimals = 2) {
        if (num === null || num === undefined || isNaN(num)) {
            return '--';
        }
        return num.toFixed(decimals);
    }

    return {
        init: init,
        updateOases: updateOases,
        runOptimization: runOptimization,
        getCurrentAllocations: getCurrentAllocations,
        formatNumber: formatNumber
    };
})();

if (typeof window !== 'undefined') {
    window.AllocationPanel = AllocationPanel;
}
